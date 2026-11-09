package notifications

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/Silo-Server/silo-server/internal/catalog"
	"github.com/Silo-Server/silo-server/internal/mail"
	"github.com/Silo-Server/silo-server/internal/newsletter"
	"github.com/Silo-Server/silo-server/internal/plugins"
	"github.com/Silo-Server/silo-server/internal/secret"
	"github.com/jackc/pgx/v5"
)

var ErrNewsletterImport = errors.New("newsletter import requires a disabled Email Alerts plugin and disabled native delivery")

type NewsletterImportSource struct {
	InstallationID int                     `json:"installation_id"`
	Version        string                  `json:"version"`
	Enabled        bool                    `json:"enabled"`
	Imported       bool                    `json:"imported"`
	Result         *NewsletterImportResult `json:"result,omitempty"`
}

type NewsletterImportResult struct {
	InstallationID int  `json:"installation_id"`
	Recipients     int  `json:"recipients"`
	Subscriptions  int  `json:"subscriptions"`
	NeedsReview    int  `json:"needs_review"`
	SettingsCopied bool `json:"settings_copied"`
	SMTPCopied     bool `json:"smtp_copied"`
}

type newsletterSnapshot struct {
	State   map[string]json.RawMessage `json:"state"`
	Configs map[string]map[string]any  `json:"configs"`
}

type NewsletterArchivedRecipient struct {
	Name      string   `json:"name"`
	Email     string   `json:"email"`
	UserID    string   `json:"user_id"`
	Enabled   bool     `json:"enabled"`
	Verified  bool     `json:"verified"`
	Mode      string   `json:"mode"`
	Types     []string `json:"types"`
	Libraries []string `json:"libraries"`
}
type NewsletterArchivedSend struct {
	Time      time.Time `json:"time"`
	Recipient string    `json:"recipient"`
	Mode      string    `json:"mode"`
	Items     int       `json:"items"`
	Subject   string    `json:"subject"`
	Error     string    `json:"error,omitempty"`
}
type NewsletterImportArchive struct {
	Recipients []NewsletterArchivedRecipient `json:"recipients"`
	History    []NewsletterArchivedSend      `json:"history"`
}

// NewsletterArchive exposes only subscriber choices and historical sends.
// SMTP credentials, API keys and unsubscribe/verification tokens stay sealed.
func (s *System) NewsletterArchive(ctx context.Context, installation int) (NewsletterImportArchive, error) {
	out := NewsletterImportArchive{Recipients: []NewsletterArchivedRecipient{}, History: []NewsletterArchivedSend{}}
	if s.newsletterCipher == nil {
		return out, ErrNewsletterImport
	}
	var sealed string
	err := s.pool.QueryRow(ctx, `SELECT snapshot FROM notification_newsletter_imports WHERE installation_id=$1`, installation).Scan(&sealed)
	if err != nil {
		return out, err
	}
	plain, err := s.newsletterCipher.Decrypt(sealed, secret.RowAAD("notification_newsletter_imports", "snapshot", strconv.Itoa(installation)))
	if err != nil {
		return out, err
	}
	var snapshot newsletterSnapshot
	if err = json.Unmarshal([]byte(plain), &snapshot); err != nil {
		return out, err
	}
	var recipients []newsletter.Recipient
	if raw := snapshot.State["recipients"]; len(raw) > 0 {
		if err = json.Unmarshal(raw, &recipients); err != nil {
			return out, err
		}
	}
	for _, r := range recipients {
		types, libraries := r.Types, r.Libraries
		if types == nil {
			types = []string{}
		}
		if libraries == nil {
			libraries = []string{}
		}
		out.Recipients = append(out.Recipients, NewsletterArchivedRecipient{Name: r.Name, Email: r.Email, UserID: r.UserID, Enabled: r.Enabled, Verified: r.Verified, Mode: r.Mode, Types: types, Libraries: libraries})
	}
	if raw := snapshot.State["log"]; len(raw) > 0 {
		if err = json.Unmarshal(raw, &out.History); err != nil {
			return out, err
		}
		if out.History == nil {
			out.History = []NewsletterArchivedSend{}
		}
	}
	return out, nil
}

func (s *System) NewsletterImportSources(ctx context.Context) ([]NewsletterImportSource, error) {
	rows, err := s.pool.Query(ctx, `SELECT COALESCE(p.id,i.installation_id),COALESCE(p.version,''),COALESCE(p.enabled,false),i.installation_id IS NOT NULL,i.result FROM plugin_installations p
 FULL JOIN notification_newsletter_imports i ON i.installation_id=p.id WHERE p.plugin_id='community.email-alerts' OR p.id IS NULL ORDER BY 1`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []NewsletterImportSource{}
	for rows.Next() {
		var source NewsletterImportSource
		var raw []byte
		if err := rows.Scan(&source.InstallationID, &source.Version, &source.Enabled, &source.Imported, &raw); err != nil {
			return nil, err
		}
		if len(raw) > 0 {
			source.Result = &NewsletterImportResult{}
			if err := json.Unmarshal(raw, source.Result); err != nil {
				return nil, err
			}
		}
		out = append(out, source)
	}
	return out, rows.Err()
}

// ImportNewsletter retains the original settings, subscriber records, history,
// watermarks and SMTP configuration under a new encryption binding. Import is
// repeatable after a lost response and never re-arms an existing native opt-out.
func (s *System) ImportNewsletter(ctx context.Context, installation int) (NewsletterImportResult, error) {
	result := NewsletterImportResult{InstallationID: installation}
	if s.newsletterCipher == nil {
		return result, ErrNewsletterImport
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return result, err
	}
	defer func() { _ = tx.Rollback(context.WithoutCancel(ctx)) }()
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtext('newsletter_import'),$1)`, installation); err != nil {
		return result, err
	}
	var previous []byte
	err = tx.QueryRow(ctx, `SELECT result FROM notification_newsletter_imports WHERE installation_id=$1`, installation).Scan(&previous)
	if err == nil {
		err = json.Unmarshal(previous, &result)
		return result, err
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return result, err
	}
	var enabled bool
	err = tx.QueryRow(ctx, `SELECT enabled FROM plugin_installations WHERE id=$1 AND plugin_id='community.email-alerts' FOR UPDATE`, installation).Scan(&enabled)
	if errors.Is(err, pgx.ErrNoRows) || err == nil && enabled {
		return result, ErrNewsletterImport
	}
	if err != nil {
		return result, err
	}
	// Same lock order as dispatch: native configuration, subscriptions, address.
	err = tx.QueryRow(ctx, `SELECT enabled FROM notification_newsletter_settings WHERE singleton FOR UPDATE`).Scan(&enabled)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return result, err
	}
	if err == nil && enabled {
		return result, ErrNewsletterImport
	}
	state := plugins.NewInstanceStateStore(s.pool, s.newsletterCipher)
	keys, err := state.Keys(ctx, installation, plugins.HostScopeAPI)
	if err != nil {
		return result, err
	}
	snapshot := newsletterSnapshot{State: map[string]json.RawMessage{}, Configs: map[string]map[string]any{}}
	for _, key := range keys {
		value, found, err := state.Read(ctx, installation, plugins.HostScopeAPI, key)
		if err != nil {
			return result, err
		}
		if found {
			if !json.Valid(value) {
				return result, fmt.Errorf("%w: invalid plugin state", ErrNewsletterImport)
			}
			snapshot.State[key] = value
		}
	}
	configs, err := plugins.NewRuntimeConfigStore(s.pool, s.newsletterCipher).ListGlobalConfigs(ctx, installation)
	if err != nil {
		return result, err
	}
	for _, config := range configs {
		snapshot.Configs[config.Key] = config.Value
	}
	content := newsletter.DefaultSettings("en")
	if raw := snapshot.State["settings"]; len(raw) > 0 {
		if err = json.Unmarshal(raw, &content); err != nil {
			return result, err
		}
	}
	content.Normalize()
	if err = content.Validate(); err != nil {
		return result, fmt.Errorf("%w: invalid templates", ErrNewsletterImport)
	}
	if content.Schedule.Timezone != "" {
		if _, err = time.LoadLocation(content.Schedule.Timezone); err != nil {
			return result, ErrNewsletterImport
		}
	}
	raw, err := json.Marshal(content)
	if err != nil {
		return result, err
	}
	tag, err := tx.Exec(ctx, `INSERT INTO notification_newsletter_settings(singleton,enabled,settings) VALUES(true,false,$1) ON CONFLICT(singleton) DO NOTHING`, raw)
	if err != nil {
		return result, err
	}
	result.SettingsCopied = tag.RowsAffected() > 0
	var recipients []newsletter.Recipient
	if raw := snapshot.State["recipients"]; len(raw) > 0 {
		if err = json.Unmarshal(raw, &recipients); err != nil {
			return result, err
		}
	}
	result.Recipients = len(recipients)
	for _, recipient := range recipients {
		p, ok := importedDeliveryPreferences(recipient)
		user, parseErr := strconv.Atoi(recipient.UserID)
		if !ok || parseErr != nil || user <= 0 {
			result.NeedsReview++
			continue
		}
		var profile string
		err = tx.QueryRow(ctx, `SELECT e.profile_id FROM notification_email_prefs e JOIN users u ON u.id=e.user_id AND u.enabled
 WHERE e.user_id=$1 AND lower(e.custom_email)=lower($2) AND e.custom_email<>''`, user, recipient.Email).Scan(&profile)
		if errors.Is(err, pgx.ErrNoRows) {
			result.NeedsReview++
			continue
		}
		if err != nil {
			return result, err
		}
		if s.profileIsChild(ctx, user, profile) {
			result.NeedsReview++
			continue
		}
		raw, err := json.Marshal(p)
		if err != nil {
			return result, err
		}
		// Start at import time. Historical windows remain in the archive rather
		// than replaying old mail through a different availability source.
		tag, err := tx.Exec(ctx, `INSERT INTO notification_delivery_preferences(user_id,profile_id,preferences) VALUES($1,$2,$3) ON CONFLICT DO NOTHING`, user, profile, raw)
		if err != nil {
			return result, err
		}
		if tag.RowsAffected() > 0 {
			result.Subscriptions++
		} else {
			result.NeedsReview++
		}
	}
	if smtp := snapshot.Configs["smtp"]; smtp != nil {
		settings := catalog.NewEncryptedSettingsRepo(catalog.NewServerSettingsRepo(s.pool), s.newsletterCipher)
		err = settings.UpdateAtomicWithTx(ctx, tx, func(current map[string]string, _ pgx.Tx) (map[string]string, error) {
			if current[mail.SettingSMTPHost] != "" {
				return nil, nil
			}
			value := func(key string) string {
				if v, ok := smtp[key].(string); ok {
					return v
				}
				return ""
			}
			if value("host") == "" || value("from_address") == "" {
				return nil, nil
			}
			port := "587"
			if n, ok := smtp["port"].(float64); ok && n >= 1 && n <= 65535 {
				port = strconv.Itoa(int(n))
			}
			security := value("security")
			if security == "" {
				security = "starttls"
			}
			if security != "none" && security != "tls" && security != "starttls" {
				return nil, ErrNewsletterImport
			}
			result.SMTPCopied = true
			return map[string]string{mail.SettingEnabled: "false", mail.SettingSMTPHost: value("host"), mail.SettingSMTPPort: port,
				mail.SettingSMTPSecurity: security, mail.SettingSMTPUsername: value("username"), mail.SettingSMTPPassword: value("password"),
				mail.SettingFromAddress: value("from_address"), mail.SettingFromName: value("from_name")}, nil
		})
		if err != nil {
			return result, err
		}
	}
	archive, err := json.Marshal(snapshot)
	if err != nil {
		return result, err
	}
	sealed, err := s.newsletterCipher.Encrypt(string(archive), secret.RowAAD("notification_newsletter_imports", "snapshot", strconv.Itoa(installation)))
	if err != nil {
		return result, err
	}
	raw, err = json.Marshal(result)
	if err != nil {
		return result, err
	}
	_, err = tx.Exec(ctx, `INSERT INTO notification_newsletter_imports(installation_id,snapshot,result) VALUES($1,$2,$3)`, installation, sealed, raw)
	if err != nil {
		return result, err
	}
	return result, tx.Commit(ctx)
}

func importedDeliveryPreferences(r newsletter.Recipient) (DeliveryPreferences, bool) {
	p := defaultDeliveryPreferences()
	if !r.Enabled || !r.Verified || len(r.Libraries) > 0 || r.MaxAgeHours > 0 {
		return p, false
	}
	switch r.Mode {
	case "instant":
		p.Alerts = "instant"
	case "instant_daily":
		p.Alerts = "instant"
		p.Newsletter = "daily"
	case "daily":
		p.Newsletter = "daily"
	case "weekly":
		p.Newsletter = "weekly"
	default:
		return p, false
	}
	if len(r.Types) > 0 {
		p.Movies, p.Episodes = false, false
		for _, kind := range r.Types {
			switch strings.TrimSpace(kind) {
			case "movie":
				p.Movies = true
			case "episode":
				p.Episodes = true
			default:
				return p, false
			}
		}
	}
	return p, true
}
