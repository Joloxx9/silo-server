package notifications

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Silo-Server/silo-server/internal/access"
	"github.com/Silo-Server/silo-server/internal/newsletter"
	"github.com/Silo-Server/silo-server/internal/plugins"
	"github.com/Silo-Server/silo-server/internal/userdb"
	"github.com/Silo-Server/silo-server/internal/userstore"
)

func TestNewsletterPreferenceIsolationAndUnsubscribeDB(t *testing.T) {
	pool := schemaDB(t, "newsletter_prefs_")
	ctx := t.Context()
	_, err := pool.Exec(ctx, `CREATE TABLE notification_delivery_preferences (LIKE public.notification_delivery_preferences INCLUDING ALL);
 CREATE TABLE notification_email_prefs (LIKE public.notification_email_prefs INCLUDING ALL);`)
	if err != nil {
		t.Fatal(err)
	}
	system := &System{pool: pool}
	a := defaultDeliveryPreferences()
	a.Push = false
	a.Popular = false
	b := defaultDeliveryPreferences()
	b.PushRequests = false
	if err = system.SetDeliveryPreferences(ctx, 7, "same", a); err != nil {
		t.Fatal(err)
	}
	if err = system.SetDeliveryPreferences(ctx, 8, "same", b); err != nil {
		t.Fatal(err)
	}
	got, err := system.DeliveryPreferences(ctx, 7, "same")
	if err != nil || got != a {
		t.Fatalf("saved preferences not returned: %+v %v", got, err)
	}
	got, err = system.DeliveryPreferences(ctx, 8, "same")
	if err != nil || got != b {
		t.Fatalf("other account changed: %+v %v", got, err)
	}
	_, err = pool.Exec(ctx, `INSERT INTO notification_email_prefs(user_id,profile_id,unsubscribe_token) VALUES(7,'same','newsletter-token');
 UPDATE notification_delivery_preferences SET preferences=preferences || '{"newsletter":"weekly","alerts":"instant"}'::jsonb;`)
	if err != nil {
		t.Fatal(err)
	}
	ok, err := NewEmailPrefsRepository(pool).UnsubscribeByToken(ctx, "newsletter-token")
	if err != nil || !ok {
		t.Fatalf("unsubscribe %v %v", ok, err)
	}
	got, err = system.DeliveryPreferences(ctx, 7, "same")
	if err != nil {
		t.Fatal(err)
	}
	if got.Newsletter != "off" || got.Alerts != "off" || got.Push {
		t.Fatalf("unsubscribe changed wrong channels: %+v", got)
	}
	got, err = system.DeliveryPreferences(ctx, 8, "same")
	if err != nil {
		t.Fatal(err)
	}
	if got.Newsletter != "weekly" || got.Alerts != "instant" {
		t.Fatalf("unsubscribe crossed account boundary: %+v", got)
	}
	_, err = pool.Exec(ctx, `UPDATE notification_delivery_preferences SET preferences=preferences || '{"newsletter":"daily","alerts":"daily"}'::jsonb WHERE user_id=7;
 UPDATE notification_email_prefs SET custom_email='reader@example.test' WHERE profile_id='same';`)
	if err != nil {
		t.Fatal(err)
	}
	repo := NewEmailPrefsRepository(pool)
	if err = repo.ClearCustomAddress(ctx, 8, "same"); err != nil {
		t.Fatal(err)
	}
	if err = repo.SetMode(ctx, 8, "same", EmailModeOff); err == nil {
		t.Fatal("foreign account changed the destination owner")
	}
	system.EmailPrefs = repo
	foreign, err := system.EmailPreferences(ctx, 8, "same")
	if err != nil || foreign.CustomEmail != "" || foreign.PendingEmail != "" {
		t.Fatalf("foreign destination leaked: %+v %v", foreign, err)
	}
	if err = repo.ClearCustomAddress(ctx, 7, "same"); err != nil {
		t.Fatal(err)
	}
	got, err = system.DeliveryPreferences(ctx, 7, "same")
	if err != nil || got.Newsletter != "off" || got.Alerts != "off" || got.Push {
		t.Fatalf("removing the address must disable mail and preserve push: %+v %v", got, err)
	}
	got, err = system.DeliveryPreferences(ctx, 8, "same")
	if err != nil || got.Newsletter != "weekly" || got.Alerts != "instant" {
		t.Fatalf("removing address crossed account boundary: %+v %v", got, err)
	}
}

func TestNewsletterBackoffDoesNotStarveOtherProfilesDB(t *testing.T) {
	pool := schemaDB(t, "newsletter_backoff_")
	ctx := t.Context()
	_, err := pool.Exec(ctx, `CREATE TABLE users (LIKE public.users INCLUDING ALL);
 CREATE TABLE notification_email_prefs (LIKE public.notification_email_prefs INCLUDING ALL);
 CREATE TABLE notification_delivery_preferences (LIKE public.notification_delivery_preferences INCLUDING ALL);
 CREATE TABLE notification_newsletter_settings (LIKE public.notification_newsletter_settings INCLUDING ALL);
 CREATE TABLE notification_newsletter_history (LIKE public.notification_newsletter_history INCLUDING ALL);
 INSERT INTO users(id,username,enabled) VALUES(7,'newsletter-test',true);
 INSERT INTO notification_email_prefs(user_id,profile_id,custom_email)
 SELECT 7,'p'||n,'reader'||n||'@example.test' FROM generate_series(1,201) n;
 INSERT INTO notification_delivery_preferences(user_id,profile_id,preferences,attempted_at,failures)
 SELECT 7,'p'||n,'{"alerts":"instant","newsletter":"off"}'::jsonb,
 CASE WHEN n<=200 THEN now()-interval '1 minute' ELSE now() END,CASE WHEN n<=200 THEN 20 ELSE 0 END
 FROM generate_series(1,201) n;`)
	if err != nil {
		t.Fatal(err)
	}
	system := &System{pool: pool, Settings: NewSettings(nil), mailSender: &fakeVerificationSender{enabled: true}, publicURL: "https://example.test"}
	if err = system.SetNewsletterSettings(ctx, NewsletterSettings{Enabled: true, Content: newsletter.DefaultSettings("en")}); err != nil {
		t.Fatal(err)
	}
	var before, after time.Time
	if err = pool.QueryRow(ctx, `SELECT attempted_at FROM notification_delivery_preferences WHERE profile_id='p201'`).Scan(&before); err != nil {
		t.Fatal(err)
	}
	// Missing profile storage fails closed before SMTP. Other recipients'
	// backoff must not prevent this profile from reaching that check.
	if err = system.sweepNewsletters(ctx); err != nil {
		t.Fatal(err)
	}
	if err = pool.QueryRow(ctx, `SELECT attempted_at FROM notification_delivery_preferences WHERE profile_id='p201'`).Scan(&after); err != nil {
		t.Fatal(err)
	}
	if !after.After(before) {
		t.Fatal("backing-off recipients consumed the entire batch and starved another profile")
	}
}

func TestNewsletterSendRetainsWindowUntilAcceptedDB(t *testing.T) {
	pool := schemaDB(t, "newsletter_send_")
	ctx := t.Context()
	_, err := pool.Exec(ctx, `CREATE TABLE users (LIKE public.users INCLUDING ALL);
 CREATE TABLE notification_email_prefs (LIKE public.notification_email_prefs INCLUDING ALL);
 CREATE TABLE notification_delivery_preferences (LIKE public.notification_delivery_preferences INCLUDING ALL);
 CREATE TABLE notification_newsletter_settings (LIKE public.notification_newsletter_settings INCLUDING ALL);
 CREATE TABLE notification_newsletter_history (LIKE public.notification_newsletter_history INCLUDING ALL);
 CREATE TABLE release_events (LIKE public.release_events INCLUDING ALL);
 CREATE TABLE media_folders (LIKE public.media_folders INCLUDING ALL);
 CREATE TABLE media_items (LIKE public.media_items INCLUDING ALL);
 CREATE TABLE episodes (LIKE public.episodes INCLUDING ALL);
 INSERT INTO users(id,username,enabled) VALUES(7,'newsletter-test',true);
 INSERT INTO notification_email_prefs(user_id,profile_id,custom_email) VALUES(7,'reader','reader@example.test');
 INSERT INTO notification_delivery_preferences(user_id,profile_id,preferences,alerts_since)
 VALUES(7,'reader','{"alerts":"instant","newsletter":"off"}'::jsonb,now()-interval '1 hour');
 INSERT INTO media_folders(id,type,name) VALUES(1,'movies','Movies');
 INSERT INTO media_items(content_id,type,title) VALUES('newsletter-movie','movie','Example movie');
 INSERT INTO release_events(id,library_id,kind,item_id,available_at,dedupe_key,created_at)
 VALUES('newsletter-event',1,'movie','newsletter-movie',now()-interval '10 minutes','newsletter-movie',now()-interval '10 minutes');`)
	if err != nil {
		t.Fatal(err)
	}
	store := userdb.NewSQLiteUserStore(openUserDB(t))
	if err = store.CreateProfile(ctx, userstore.Profile{ID: "reader", Name: "Reader"}); err != nil {
		t.Fatal(err)
	}
	sender := &fakeVerificationSender{enabled: true, err: errors.New("SMTP unavailable")}
	system := &System{pool: pool, Settings: NewSettings(nil), EmailPrefs: NewEmailPrefsRepository(pool), mailSender: sender,
		stores: preferenceTransactionTestProvider{store: store}, scopes: scopeByProfile{"reader": access.Scope{}}, publicURL: "https://example.test"}
	config := NewsletterSettings{Enabled: true, Content: newsletter.DefaultSettings("en")}
	config.Content.Delivery.InstantGapMinutes = 0
	config.Content.Delivery.EmbedImages = false
	preferences := defaultDeliveryPreferences()
	preferences.Alerts = "instant"
	if err = system.SetDeliveryPreferences(ctx, 7, "reader", preferences); err != nil {
		t.Fatal(err)
	}
	if err = system.SetNewsletterSettings(ctx, config); err != nil {
		t.Fatal(err)
	}
	var before, after time.Time
	if err = pool.QueryRow(ctx, `SELECT alerts_since FROM notification_delivery_preferences`).Scan(&before); err != nil {
		t.Fatal(err)
	}
	key := profileKey{userID: 7, profileID: "reader"}
	if err = system.sendNewsletter(ctx, key, config); err == nil {
		t.Fatal("transport failure not returned")
	}
	if err = pool.QueryRow(ctx, `SELECT alerts_since FROM notification_delivery_preferences`).Scan(&after); err != nil || !before.Equal(after) {
		t.Fatalf("failed send advanced watermark: %v %v", after, err)
	}
	if _, err = pool.Exec(ctx, `UPDATE notification_delivery_preferences SET attempted_at=NULL,failures=0`); err != nil {
		t.Fatal(err)
	}
	accepted, cancel := context.WithCancel(ctx)
	defer cancel()
	sender.err = nil
	sender.cancel = cancel
	if err = system.sendNewsletter(accepted, key, config); err != nil {
		t.Fatal(err)
	}
	if err = pool.QueryRow(ctx, `SELECT alerts_since FROM notification_delivery_preferences`).Scan(&after); err != nil || !after.After(before) {
		t.Fatalf("accepted send was not persisted after cancellation: %v %v", after, err)
	}
	var count int
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM notification_newsletter_history WHERE item_count=1`).Scan(&count); err != nil || count != 1 {
		t.Fatalf("send history: %d %v", count, err)
	}
	sender.cancel = nil
	if err = system.sendNewsletter(ctx, key, config); err != nil {
		t.Fatal(err)
	}
	if len(sender.messages()) != 2 {
		t.Fatal("committed window was emailed again")
	}
}

func TestNewsletterPluginImportArchiveDB(t *testing.T) {
	pool := schemaDB(t, "newsletter_import_")
	ctx := t.Context()
	_, err := pool.Exec(ctx, `CREATE TABLE plugin_installations (LIKE public.plugin_installations INCLUDING ALL);
 CREATE TABLE plugin_runtime_configs (LIKE public.plugin_runtime_configs INCLUDING ALL);
 CREATE TABLE plugin_instance_state (LIKE public.plugin_instance_state INCLUDING ALL);
 CREATE TABLE notification_newsletter_settings (LIKE public.notification_newsletter_settings INCLUDING ALL);
 CREATE TABLE notification_newsletter_imports (LIKE public.notification_newsletter_imports INCLUDING ALL);
 INSERT INTO plugin_installations(id,plugin_id,version,install_path,enabled) VALUES(101,'community.email-alerts','1.9.0','test',true);`)
	if err != nil {
		t.Fatal(err)
	}
	cipher := testPushCipher(t)
	system := &System{pool: pool, newsletterCipher: cipher}
	state := plugins.NewInstanceStateStore(pool, cipher)
	value := []byte(`[{"name":"Reader","email":"reader@example.test","user_id":"7","enabled":true,"verified":false,"mode":"weekly","token":"must-stay-secret","verify_token":"must-stay-secret"}]`)
	if err = state.Write(ctx, 101, plugins.HostScopeAPI, "recipients", value); err != nil {
		t.Fatal(err)
	}
	if _, err = system.ImportNewsletter(ctx, 101); err == nil {
		t.Fatal("running plugin imported")
	}
	if _, err = pool.Exec(ctx, `UPDATE plugin_installations SET enabled=false WHERE id=101`); err != nil {
		t.Fatal(err)
	}
	first, err := system.ImportNewsletter(ctx, 101)
	if err != nil {
		t.Fatal(err)
	}
	if first.Recipients != 1 || first.Subscriptions != 0 || first.NeedsReview != 1 || !first.SettingsCopied {
		t.Fatalf("unexpected import: %+v", first)
	}
	second, err := system.ImportNewsletter(ctx, 101)
	if err != nil || second != first {
		t.Fatalf("replay changed import: %+v %v", second, err)
	}
	if _, err = pool.Exec(ctx, `DELETE FROM plugin_instance_state; DELETE FROM plugin_installations`); err != nil {
		t.Fatal(err)
	}
	archive, err := system.NewsletterArchive(ctx, 101)
	if err != nil || len(archive.Recipients) != 1 || archive.Recipients[0].Email != "reader@example.test" {
		t.Fatalf("archive lost on uninstall: %+v %v", archive, err)
	}
	raw, err := json.Marshal(archive)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "must-stay-secret") {
		t.Fatal("archive API exposed a verification or unsubscribe token")
	}
	var sealed string
	if err = pool.QueryRow(ctx, `SELECT snapshot FROM notification_newsletter_imports WHERE installation_id=101`).Scan(&sealed); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(sealed, "reader@example.test") {
		t.Fatal("archive stored plaintext personal data")
	}
	if _, err = pool.Exec(ctx, `UPDATE notification_newsletter_imports SET installation_id=102 WHERE installation_id=101`); err != nil {
		t.Fatal(err)
	}
	if _, err = system.NewsletterArchive(ctx, 102); err == nil {
		t.Fatal("archive accepted a different encryption binding")
	}
}
