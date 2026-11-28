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

// NewsletterRecipient is one profile with a verified email address, and what
// it has chosen to receive.
type NewsletterRecipient struct {
	UserID      int
	Account     string
	ProfileID   string
	ProfileName string
	Email       string
	Newsletter  string // off, daily, weekly
	Alerts      string // off, instant, daily
	LastSentAt  *time.Time
}

// NewsletterRecipients lists every profile with a verified address on an
// enabled account, including those that receive nothing, so the admin sees
// who could be sent mail and who is.
func (s *System) NewsletterRecipients(ctx context.Context) ([]NewsletterRecipient, error) {
	rows, err := s.pool.Query(ctx, `SELECT e.user_id,COALESCE(u.username,u.email,''),e.profile_id,e.custom_email,p.preferences,
 (SELECT max(h.sent_at) FROM notification_newsletter_history h WHERE h.user_id=e.user_id AND h.profile_id=e.profile_id)
 FROM notification_email_prefs e JOIN users u ON u.id=e.user_id AND u.enabled=true
 LEFT JOIN notification_delivery_preferences p ON p.user_id=e.user_id AND p.profile_id=e.profile_id
 WHERE e.custom_email<>'' ORDER BY e.user_id,e.profile_id`)
	if err != nil {
		return nil, err
	}
	var out []NewsletterRecipient
	for rows.Next() {
		var r NewsletterRecipient
		var raw []byte
		if err = rows.Scan(&r.UserID, &r.Account, &r.ProfileID, &r.Email, &raw, &r.LastSentAt); err != nil {
			rows.Close()
			return nil, err
		}
		p := defaultDeliveryPreferences()
		if len(raw) > 0 {
			if err = json.Unmarshal(raw, &p); err != nil {
				rows.Close()
				return nil, err
			}
		}
		r.Newsletter, r.Alerts = p.Newsletter, p.Alerts
		out = append(out, r)
	}
	rows.Close()
	if err = rows.Err(); err != nil {
		return nil, err
	}
	for i := range out {
		out[i].ProfileName = s.lookupProfileName(ctx, out[i].UserID, out[i].ProfileID)
	}
	return out, nil
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
	if t.Mode != "daily" {
		t.Mode = "weekly"
	}
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
	if t.Mode == "daily" {
		since = now.AddDate(0, 0, -1)
	}
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
