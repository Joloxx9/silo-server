package notifications

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	netmail "net/mail"
	"time"

	"github.com/Silo-Server/silo-server/internal/newsletter"
	"github.com/jackc/pgx/v5"
)

// NewsletterRecipient is one profile on an enabled account, its email
// address if it has one, and what it has chosen to receive.
type NewsletterRecipient struct {
	UserID      int
	Account     string
	ProfileID   string
	ProfileName string
	IsChild     bool
	IsPrimary   bool
	// AccountEmail is the login account's address, a suggestion for the
	// primary profile; it is never used for delivery.
	AccountEmail string
	Email        string // verified address, or "" when the profile has none
	Newsletter   string // off, daily, weekly
	Alerts       string // off, instant, daily
	LastSentAt   *time.Time
}

// NewsletterRecipients lists every profile of every enabled account, with or
// without an address, so the admin can give household profiles an address.
func (s *System) NewsletterRecipients(ctx context.Context) ([]NewsletterRecipient, error) {
	type account struct {
		id          int
		name, email string
	}
	rows, err := s.pool.Query(ctx, `SELECT id,COALESCE(NULLIF(username,''),email,''),COALESCE(email,'') FROM users WHERE enabled=true ORDER BY id`)
	if err != nil {
		return nil, err
	}
	var accounts []account
	for rows.Next() {
		var a account
		if err = rows.Scan(&a.id, &a.name, &a.email); err != nil {
			rows.Close()
			return nil, err
		}
		accounts = append(accounts, a)
	}
	rows.Close()
	if err = rows.Err(); err != nil {
		return nil, err
	}
	type state struct {
		email string
		prefs []byte
		last  *time.Time
	}
	states := map[profileKey]state{}
	rows, err = s.pool.Query(ctx, `SELECT k.user_id,k.profile_id,COALESCE(e.custom_email,''),p.preferences,
 (SELECT max(h.sent_at) FROM notification_newsletter_history h WHERE h.user_id=k.user_id AND h.profile_id=k.profile_id)
 FROM (SELECT user_id,profile_id FROM notification_email_prefs UNION SELECT user_id,profile_id FROM notification_delivery_preferences) k
 LEFT JOIN notification_email_prefs e ON e.user_id=k.user_id AND e.profile_id=k.profile_id
 LEFT JOIN notification_delivery_preferences p ON p.user_id=k.user_id AND p.profile_id=k.profile_id`)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var k profileKey
		var st state
		if err = rows.Scan(&k.userID, &k.profileID, &st.email, &st.prefs, &st.last); err != nil {
			rows.Close()
			return nil, err
		}
		states[k] = st
	}
	rows.Close()
	if err = rows.Err(); err != nil {
		return nil, err
	}
	out := []NewsletterRecipient{}
	for _, a := range accounts {
		if s.stores == nil {
			break
		}
		store, err := s.stores.ForUser(ctx, a.id)
		if err != nil {
			return nil, err
		}
		profiles, err := store.ListProfiles(ctx)
		if err != nil {
			return nil, err
		}
		for _, profile := range profiles {
			st := states[profileKey{userID: a.id, profileID: profile.ID}]
			p := defaultDeliveryPreferences()
			if len(st.prefs) > 0 {
				if err = json.Unmarshal(st.prefs, &p); err != nil {
					return nil, err
				}
			}
			p.weeklyNewsletter()
			out = append(out, NewsletterRecipient{UserID: a.id, Account: a.name, ProfileID: profile.ID, ProfileName: profile.Name,
				IsChild: profile.IsChild, IsPrimary: profile.IsPrimary, AccountEmail: a.email, Email: st.email, Newsletter: p.Newsletter, Alerts: p.Alerts, LastSentAt: st.last})
		}
	}
	return out, nil
}

// SetNewsletterRecipient lets the admin give a profile an address and choose
// what it receives, for household profiles that never sign in on their own.
// The admin vouches for the address, so it is stored as verified. An empty
// address removes it and stops that profile's email.
func (s *System) SetNewsletterRecipient(ctx context.Context, userID int, profileID, address, newsletterMode, alerts string) error {
	if s.lookupProfile(ctx, userID, profileID) == nil {
		return fmt.Errorf("%w: profile", ErrNewsletterTest)
	}
	if address == "" {
		return s.EmailPrefs.ClearCustomAddress(ctx, userID, profileID)
	}
	parsed, err := netmail.ParseAddress(address)
	if err != nil {
		return fmt.Errorf("%w: address", ErrNewsletterTest)
	}
	address = parsed.Address
	if (newsletterMode != "off" || alerts != "off") && s.profileIsChild(ctx, userID, profileID) {
		return ErrEmailChildProfile
	}
	inUse, err := addressInUse(ctx, s.pool, address, profileID, userID)
	if err != nil {
		return err
	}
	if inUse {
		return fmt.Errorf("%w: this address already belongs to another profile or account", ErrNewsletterTest)
	}
	if _, err = s.pool.Exec(ctx, `INSERT INTO notification_email_prefs(profile_id,user_id,mode,custom_email) VALUES($1,$2,'off',$3)
 ON CONFLICT(user_id,profile_id) DO UPDATE SET custom_email=EXCLUDED.custom_email,pending_email='',pending_token_hash='',pending_expires_at=NULL,updated_at=now()`,
		profileID, userID, address); err != nil {
		if isUniqueViolation(err) {
			return fmt.Errorf("%w: this address already belongs to another profile", ErrNewsletterTest)
		}
		return err
	}
	p, err := s.DeliveryPreferences(ctx, userID, profileID)
	if err != nil {
		return err
	}
	p.Newsletter, p.Alerts = newsletterMode, alerts
	return s.SetDeliveryPreferences(ctx, userID, profileID, p)
}

// NewsletterTest asks for one recipient's real newsletter, built from the
// catalog and that profile's history as the scheduled send would build it.
type NewsletterTest struct {
	Settings  newsletter.Settings // the admin's draft, saved or not
	UserID    int
	ProfileID string
	Mode      string // daily or weekly; it sets the window and the recap
	To        string // address to send to; empty renders without sending
}

type NewsletterTestResult struct {
	Rendered  *newsletter.Rendered // nil when the window has nothing to show
	ItemCount int
	Sent      bool
}

var ErrNewsletterTest = errors.New("invalid newsletter test")

// NewsletterTestSend renders the profile's newsletter for the last day or
// week and, with an address, sends it there. It changes no delivery state:
// the recipient's schedule and watermarks stay as they are.
func (s *System) NewsletterTestSend(ctx context.Context, t NewsletterTest) (NewsletterTestResult, error) {
	var result NewsletterTestResult
	// The newsletter is weekly; a test shows the same week a send would.
	t.Mode = "weekly"
	send := t.To != ""
	if send {
		addr, err := netmail.ParseAddress(t.To)
		if err != nil {
			return result, fmt.Errorf("%w: address", ErrNewsletterTest)
		}
		t.To = addr.Address
		if s.mailSender == nil || !s.mailSender.Enabled(ctx) {
			return result, fmt.Errorf("%w: outgoing mail is not configured", ErrNewsletterTest)
		}
	}
	base := s.newsletterLinkBase(ctx)
	if base == "" {
		return result, fmt.Errorf("%w: the server has no public URL", ErrNewsletterTest)
	}
	if s.lookupProfile(ctx, t.UserID, t.ProfileID) == nil {
		return result, fmt.Errorf("%w: profile", ErrNewsletterTest)
	}
	p, err := s.DeliveryPreferences(ctx, t.UserID, t.ProfileID)
	if err != nil {
		return result, err
	}
	t.Settings.Normalize()
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{AccessMode: pgx.ReadOnly})
	if err != nil {
		return result, err
	}
	defer func() { _ = tx.Rollback(context.WithoutCancel(ctx)) }()
	now := time.Now()
	since := now.AddDate(0, 0, -t.Settings.Schedule.WeeklyDays)
	k := profileKey{userID: t.UserID, profileID: t.ProfileID}
	env := newsletter.RenderEnv{Settings: t.Settings, ServerName: s.emailBrand.Load(ctx).Name, PublicURL: base,
		Recipient: newsletter.Recipient{Name: s.lookupProfileName(ctx, t.UserID, t.ProfileID), Email: t.To}, Mode: t.Mode, Now: now,
		ManageURL: base + "/settings/notifications", Preview: !send}
	rendered, entries, err := s.composeNewsletter(ctx, tx, k, p, env, true, since, now)
	if err != nil {
		return result, err
	}
	result.Rendered, result.ItemCount = rendered, len(entries)
	if rendered == nil || !send {
		return result, nil
	}
	rendered.Subject = "[Test] " + rendered.Subject
	if err = s.mailSender.Send(ctx, newsletterMessage(t.To, rendered, "")); err != nil {
		return result, fmt.Errorf("newsletter transport: %w", err)
	}
	result.Sent = true
	return result, nil
}
