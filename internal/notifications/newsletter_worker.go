package notifications

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/Silo-Server/silo-server/internal/mail"
	"github.com/Silo-Server/silo-server/internal/newsletter"
	"github.com/jackc/pgx/v5"
)

// runNewsletters shares the server's lifecycle. Every recipient is locked
// across the send, so two workers cannot send the same scheduled window.
func (s *System) runNewsletters(ctx context.Context) {
	tick := time.NewTicker(time.Minute)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
			if err := s.sweepNewsletters(ctx); err != nil && ctx.Err() == nil {
				s.logger.ErrorContext(ctx, "newsletter sweep failed", "error", err)
			}
		}
	}
}

func (s *System) sweepNewsletters(ctx context.Context) error {
	if !s.NewsletterTransportAvailable(ctx) {
		return nil
	}
	settings, err := s.NewsletterSettings(ctx)
	if err != nil {
		return err
	}
	if !settings.Enabled || settings.Content.Delivery.Paused {
		return nil
	}
	rows, err := s.pool.Query(ctx, `SELECT p.user_id,p.profile_id FROM notification_delivery_preferences p
 JOIN users u ON u.id=p.user_id AND u.enabled=true
 JOIN notification_email_prefs e ON e.user_id=p.user_id AND e.profile_id=p.profile_id AND e.custom_email<>''
 WHERE (p.preferences->>'alerts'<>'off' OR p.preferences->>'newsletter'<>'off')
 AND (p.failures<=0 OR p.attempted_at IS NULL OR p.attempted_at +
 LEAST($1::double precision*power(2,LEAST(p.failures-1,30)),$2::double precision)*interval '1 millisecond'<=now())
 ORDER BY p.attempted_at NULLS FIRST LIMIT 200`, channelFailureBackoffBase.Milliseconds(), channelFailureBackoffMax.Milliseconds())
	if err != nil {
		return err
	}
	var keys []profileKey
	for rows.Next() {
		var k profileKey
		if err = rows.Scan(&k.userID, &k.profileID); err != nil {
			rows.Close()
			return err
		}
		keys = append(keys, k)
	}
	rows.Close()
	if err = rows.Err(); err != nil {
		return err
	}
	for i, k := range keys {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if i > 0 && settings.Content.Delivery.SendDelaySeconds > 0 {
			timer := time.NewTimer(time.Duration(settings.Content.Delivery.SendDelaySeconds) * time.Second)
			select {
			case <-ctx.Done():
				timer.Stop()
				return ctx.Err()
			case <-timer.C:
			}
		}
		runCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
		err = s.sendNewsletter(runCtx, k, settings)
		cancel()
		if err != nil && ctx.Err() == nil {
			s.logger.WarnContext(ctx, "newsletter recipient failed", "error", err)
		}
	}
	_, err = s.pool.Exec(ctx, `DELETE FROM notification_newsletter_history WHERE sent_at<now()-interval '90 days'`)
	return err
}

func (s *System) sendNewsletter(ctx context.Context, k profileKey, c NewsletterSettings) (result error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	var raw []byte
	var alertsSince, newsSince time.Time
	var alertsLast, newsLast, attempt *time.Time
	var failures int
	claimed := false
	defer func() {
		persist, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
		defer cancel()
		_ = tx.Rollback(persist)
		if result != nil && claimed {
			// Also back off composition/catalog failures. Compare the original
			// attempt so a newer worker's committed progress is never overwritten.
			_, saveErr := s.pool.Exec(persist, `UPDATE notification_delivery_preferences SET attempted_at=now(),failures=failures+1
 WHERE user_id=$1 AND profile_id=$2 AND attempted_at IS NOT DISTINCT FROM $3::timestamptz`, k.userID, k.profileID, attempt)
			if saveErr != nil {
				result = errors.Join(result, saveErr)
			}
		}
	}()
	// Hold the current configuration through dispatch. Once disabling returns,
	// no worker may still start a send with the previous configuration.
	err = tx.QueryRow(ctx, `SELECT enabled,settings FROM notification_newsletter_settings WHERE singleton FOR SHARE`).Scan(&c.Enabled, &raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return tx.Commit(ctx)
	}
	if err != nil {
		return err
	}
	if err = json.Unmarshal(raw, &c.Content); err != nil {
		return err
	}
	c.Content.Normalize()
	if !c.Enabled || c.Content.Delivery.Paused {
		return tx.Commit(ctx)
	}
	base := s.newsletterLinkBase(ctx)
	if base == "" {
		return tx.Commit(ctx)
	}
	err = tx.QueryRow(ctx, `SELECT preferences,alerts_since,newsletter_since,alerts_sent_at,newsletter_sent_at,attempted_at,failures
 FROM notification_delivery_preferences WHERE user_id=$1 AND profile_id=$2 FOR UPDATE SKIP LOCKED`, k.userID, k.profileID).
		Scan(&raw, &alertsSince, &newsSince, &alertsLast, &newsLast, &attempt, &failures)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	claimed = true
	p := defaultDeliveryPreferences()
	if err = json.Unmarshal(raw, &p); err != nil {
		return err
	}
	now := time.Now()
	if !channelRetryEligible(now, attempt, failures) {
		return nil
	}
	if _, err = tx.Exec(ctx, `UPDATE notification_delivery_preferences SET attempted_at=$3 WHERE user_id=$1 AND profile_id=$2`, k.userID, k.profileID, now); err != nil {
		return err
	}
	if s.profileIsChild(ctx, k.userID, k.profileID) {
		return tx.Commit(ctx)
	}
	channel, mode, since := "", "", time.Time{}
	if p.Newsletter != "off" && newsletterDue(now, newsLast, p.Newsletter, c.Content.Schedule) {
		channel, mode, since = "newsletter", p.Newsletter, newsSince
	} else if p.Alerts == "instant" && (alertsLast == nil || now.Sub(*alertsLast) >= time.Duration(c.Content.Delivery.InstantGapMinutes)*time.Minute) {
		channel, mode, since = "alerts", "instant", alertsSince
	} else if p.Alerts == "daily" && newsletterDue(now, alertsLast, "daily", c.Content.Schedule) {
		channel, mode, since = "alerts", "daily", alertsSince
	}
	if channel == "" {
		return tx.Commit(ctx)
	}
	if mode == "instant" {
		if newsletterQuiet(now, c.Content) {
			return tx.Commit(ctx)
		}
		if c.Content.Delivery.InstantDailyCap > 0 {
			loc := newsletterLocation(c.Content.Schedule)
			local := now.In(loc)
			start := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, loc)
			var count int
			if err = tx.QueryRow(ctx, `SELECT count(*) FROM notification_newsletter_history WHERE user_id=$1 AND profile_id=$2 AND channel='alerts' AND sent_at>=$3 AND sent_at<$4`, k.userID, k.profileID, start, start.AddDate(0, 0, 1)).Scan(&count); err != nil {
				return err
			}
			if count >= c.Content.Delivery.InstantDailyCap {
				return tx.Commit(ctx)
			}
		}
	}
	// Lock the verified destination too: clearing it waits for this send,
	// and an unsubscribe acquires these locks in the same order.
	var address, token string
	err = tx.QueryRow(ctx, `SELECT e.custom_email,e.unsubscribe_token FROM notification_email_prefs e JOIN users u ON u.id=e.user_id AND u.enabled=true
 WHERE e.user_id=$1 AND e.profile_id=$2 FOR UPDATE OF e`, k.userID, k.profileID).Scan(&address, &token)
	if errors.Is(err, pgx.ErrNoRows) {
		return tx.Commit(ctx)
	}
	if err != nil {
		return err
	}
	if address == "" {
		return tx.Commit(ctx)
	}
	if token == "" {
		token, _, err = newEmailToken()
		if err != nil {
			return err
		}
		if err = s.EmailPrefs.setUnsubscribeToken(ctx, tx, k, token); err != nil {
			return err
		}
	}
	// Leave a settlement window behind the watermark for catalog commits.
	until := now.Add(-max(2*time.Minute, time.Duration(c.Content.Delivery.SettleMinutes)*time.Minute))
	oldest := now.Add(-time.Duration(c.Content.Delivery.MaxAgeHours) * time.Hour)
	if channel == "newsletter" {
		oldest = now.AddDate(0, 0, -c.Content.Schedule.WeeklyDays)
		if mode == "daily" {
			oldest = now.AddDate(0, 0, -1)
		}
	}
	if until.Before(since) {
		return tx.Commit(ctx)
	}
	if since.Before(oldest) {
		since = oldest
	}
	entries, err := s.newsletterEntries(ctx, tx, k, p, since, until)
	if err != nil {
		return err
	}
	compact := c.Content.Delivery.BurstThreshold > 0 && len(entries) > c.Content.Delivery.BurstThreshold
	more := 0
	if !compact {
		entries, more = newsletter.LimitEntries(entries, c.Content.Delivery)
	}
	env := newsletter.RenderEnv{Compact: compact, MoreCount: more, Settings: c.Content, ServerName: s.emailBrand.Load(ctx).Name, PublicURL: base,
		Recipient: newsletter.Recipient{Name: s.lookupProfileName(ctx, k.userID, k.profileID), Email: address}, Mode: mode, Now: now,
		ManageURL: base + "/settings/notifications", UnsubscribeURL: emailUnsubscribeURL(base, token)}
	if channel == "newsletter" {
		env.Recap, err = s.newsletterRecap(ctx, tx, k, p, c.Content, since, until)
		if err != nil {
			return err
		}
	}
	if len(entries) > 0 || channel == "newsletter" && c.Content.Recap.WeeklyWhenEmpty && !env.Recap.Empty() {
		env.Settings.Theme.LogoURL = s.absoluteArtworkURL(ctx, env.Settings.Theme.LogoURL)
		rendered, renderErr := newsletter.Render(ctx, env, entries, newsletter.ArtworkFetcher(entries, base, env.Settings.Theme.LogoURL))
		if renderErr != nil {
			return renderErr
		}
		msg := mail.Message{To: []string{address}, Subject: rendered.Subject, HTMLBody: rendered.HTML, TextBody: rendered.Text}
		for _, img := range rendered.Images {
			msg.Inline = append(msg.Inline, mail.InlineImage{ContentID: img.CID, Filename: img.CID + ".jpg", ContentType: img.ContentType, Data: img.Data})
		}
		if env.UnsubscribeURL != "" {
			msg.Headers = map[string]string{"List-Unsubscribe": "<" + env.UnsubscribeURL + ">", "List-Unsubscribe-Post": "List-Unsubscribe=One-Click"}
		}
		if err = s.mailSender.Send(ctx, msg); err != nil {
			// Keep the window for a later attempt. SMTP acceptance may be uncertain;
			// this has the same at-least-once semantics as notification email.
			persist, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
			defer cancel()
			if _, saveErr := tx.Exec(persist, `UPDATE notification_delivery_preferences SET failures=failures+1 WHERE user_id=$1 AND profile_id=$2`, k.userID, k.profileID); saveErr != nil {
				return saveErr
			}
			if saveErr := tx.Commit(persist); saveErr != nil {
				return saveErr
			}
			return fmt.Errorf("newsletter transport: %w", err)
		}
		persist, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
		defer cancel()
		ctx = persist
		if _, err = tx.Exec(ctx, `INSERT INTO notification_newsletter_history(user_id,profile_id,channel,item_count) VALUES($1,$2,$3,$4)`, k.userID, k.profileID, channel, len(entries)); err != nil {
			return err
		}
	}
	// Column names come only from the fixed channel choices above.
	if channel == "newsletter" {
		_, err = tx.Exec(ctx, `UPDATE notification_delivery_preferences SET newsletter_since=$3,newsletter_sent_at=$4,failures=0 WHERE user_id=$1 AND profile_id=$2`, k.userID, k.profileID, until, now)
	} else {
		_, err = tx.Exec(ctx, `UPDATE notification_delivery_preferences SET alerts_since=$3,alerts_sent_at=$4,failures=0 WHERE user_id=$1 AND profile_id=$2`, k.userID, k.profileID, until, now)
	}
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}
