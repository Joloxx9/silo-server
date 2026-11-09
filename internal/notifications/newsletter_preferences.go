package notifications

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/Silo-Server/silo-server/internal/newsletter"
	"github.com/jackc/pgx/v5"
)

// DeliveryPreferences separates device push from opt-in email broadcasts.
// The verified email destination remains owned by EmailPrefs.
type DeliveryPreferences struct {
	Push         bool   `json:"push"`
	PushEpisodes bool   `json:"push_episodes"`
	PushRequests bool   `json:"push_requests"`
	PushOther    bool   `json:"push_other"`
	Alerts       string `json:"alerts" enum:"off,instant,daily"`
	Newsletter   string `json:"newsletter" enum:"off,daily,weekly"`
	Movies       bool   `json:"movies"`
	Episodes     bool   `json:"episodes"`
	Personal     bool   `json:"personal"`
	Popular      bool   `json:"popular"`
}

func defaultDeliveryPreferences() DeliveryPreferences {
	return DeliveryPreferences{Push: true, PushEpisodes: true, PushRequests: true, PushOther: true,
		Alerts: "off", Newsletter: "off", Movies: true, Episodes: true, Personal: true, Popular: true}
}

var ErrDeliveryPreferences = errors.New("invalid delivery preferences")

func (p DeliveryPreferences) valid() bool {
	return (p.Alerts == "off" || p.Alerts == "instant" || p.Alerts == "daily") &&
		(p.Newsletter == "off" || p.Newsletter == "daily" || p.Newsletter == "weekly")
}

func (p DeliveryPreferences) allowsPush(kind string) bool {
	if !p.Push {
		return false
	}
	switch kind {
	case DeliveryTypeEpisodeAvailable:
		return p.PushEpisodes
	case DeliveryTypeRequestApproved, DeliveryTypeRequestDeclined, DeliveryTypeRequestFulfilled:
		return p.PushRequests
	default:
		return p.PushOther
	}
}

func (s *System) DeliveryPreferences(ctx context.Context, user int, profile string) (DeliveryPreferences, error) {
	p := defaultDeliveryPreferences()
	var raw []byte
	err := s.pool.QueryRow(ctx, `SELECT preferences FROM notification_delivery_preferences WHERE user_id=$1 AND profile_id=$2`, user, profile).Scan(&raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return p, nil
	}
	if err != nil {
		return p, err
	}
	err = json.Unmarshal(raw, &p)
	return p, err
}

func (s *System) SetDeliveryPreferences(ctx context.Context, user int, profile string, p DeliveryPreferences) error {
	if !p.valid() {
		return ErrDeliveryPreferences
	}
	raw, err := json.Marshal(p)
	if err != nil {
		return err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(context.WithoutCancel(ctx)) }()
	_, err = tx.Exec(ctx, `INSERT INTO notification_delivery_preferences(user_id,profile_id,preferences)
 VALUES($1,$2,$3) ON CONFLICT(user_id,profile_id) DO UPDATE SET
 alerts_since=CASE WHEN notification_delivery_preferences.preferences->>'alerts'='off' AND $3::jsonb->>'alerts'<>'off' THEN now() ELSE notification_delivery_preferences.alerts_since END,
 newsletter_since=CASE WHEN notification_delivery_preferences.preferences->>'newsletter'='off' AND $3::jsonb->>'newsletter'<>'off' THEN now() ELSE notification_delivery_preferences.newsletter_since END,
 preferences=EXCLUDED.preferences`, user, profile, raw)
	if err != nil {
		return err
	}
	if p.Alerts != "off" || p.Newsletter != "off" {
		if s.profileIsChild(ctx, user, profile) {
			return ErrEmailChildProfile
		}
		// Match dispatch/unsubscribe lock order. Clearing an address cannot
		// race an opt-in and leave mail armed for a future destination.
		var address string
		err = tx.QueryRow(ctx, `SELECT custom_email FROM notification_email_prefs WHERE user_id=$1 AND profile_id=$2 FOR UPDATE`, user, profile).Scan(&address)
		if errors.Is(err, pgx.ErrNoRows) || err == nil && address == "" {
			return ErrEmailNoAddress
		}
		if err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func (s *System) allowNotificationPush(ctx context.Context, row DeliveryRow) (bool, error) {
	p, err := s.DeliveryPreferences(ctx, row.UserID, row.ProfileID)
	return err == nil && p.allowsPush(row.Type), err
}

type NewsletterSettings struct {
	Enabled bool                `json:"enabled"`
	Content newsletter.Settings `json:"content"`
}

func (s *System) NewsletterTransportAvailable(ctx context.Context) bool {
	return s.mailSender != nil && s.mailSender.Enabled(ctx) && s.newsletterLinkBase(ctx) != ""
}

func (s *System) NewsletterEmailAllowed(ctx context.Context, user int, profile string) bool {
	return !s.profileIsChild(ctx, user, profile)
}

func (s *System) newsletterLinkBase(ctx context.Context) string {
	base := strings.TrimRight(s.emailLinkBase(ctx), "/")
	u, err := url.Parse(base)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return ""
	}
	return base
}

func (s *System) NewsletterSettings(ctx context.Context) (NewsletterSettings, error) {
	c := NewsletterSettings{Content: newsletter.DefaultSettings("en")}
	var raw []byte
	err := s.pool.QueryRow(ctx, `SELECT enabled,settings FROM notification_newsletter_settings WHERE singleton`).Scan(&c.Enabled, &raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return c, nil
	}
	if err != nil {
		return c, err
	}
	if err = json.Unmarshal(raw, &c.Content); err != nil {
		return c, err
	}
	c.Content.Normalize()
	return c, nil
}

func (s *System) SetNewsletterSettings(ctx context.Context, c NewsletterSettings) error {
	c.Content.Normalize()
	if _, err := time.LoadLocation(c.Content.Schedule.Timezone); c.Content.Schedule.Timezone != "" && err != nil {
		return fmt.Errorf("%w: timezone", ErrDeliveryPreferences)
	}
	if err := c.Content.Validate(); err != nil {
		return fmt.Errorf("%w: template: %v", ErrDeliveryPreferences, err)
	}
	raw, err := json.Marshal(c.Content)
	if err != nil {
		return err
	}
	_, err = s.pool.Exec(ctx, `INSERT INTO notification_newsletter_settings(singleton,enabled,settings) VALUES(true,$1,$2)
 ON CONFLICT(singleton) DO UPDATE SET enabled=EXCLUDED.enabled,settings=EXCLUDED.settings,updated_at=now()`, c.Enabled, raw)
	return err
}

// newsletterDue uses calendar boundaries so daylight-saving transitions do
// not drift the configured local send time. An opt-in never sends old mail.
func newsletterDue(now time.Time, last *time.Time, mode string, s newsletter.Schedule) bool {
	loc := newsletterLocation(s)
	now = now.In(loc)
	hour, minute := s.DailyHour, s.DailyMinute
	if mode == "weekly" {
		hour, minute = s.WeeklyHour, s.WeeklyMinute
	}
	due := time.Date(now.Year(), now.Month(), now.Day(), hour, minute, 0, 0, loc)
	if mode == "weekly" {
		due = due.AddDate(0, 0, -(int(now.Weekday())-s.WeeklyDay+7)%7)
	}
	return !now.Before(due) && (last == nil || last.Before(due))
}

func newsletterLocation(s newsletter.Schedule) *time.Location {
	if s.Timezone != "" {
		if l, err := time.LoadLocation(s.Timezone); err == nil {
			return l
		}
	}
	return time.UTC
}

func newsletterQuiet(now time.Time, c newsletter.Settings) bool {
	d := c.Delivery
	if !d.QuietEnabled || d.QuietStart == d.QuietEnd {
		return false
	}
	hour := now.In(newsletterLocation(c.Schedule)).Hour()
	if d.QuietStart > d.QuietEnd {
		return hour >= d.QuietStart || hour < d.QuietEnd
	}
	return hour >= d.QuietStart && hour < d.QuietEnd
}
