package apiv2

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/Silo-Server/silo-server/internal/newsletter"
	"github.com/Silo-Server/silo-server/internal/notifications"
)

type fakeNewsletter struct {
	user        int
	profile     string
	writes      int
	adminWrites int
	last        *notifications.NewsletterSettings
}

func (f *fakeNewsletter) DeliveryPreferences(_ context.Context, u int, p string) (notifications.DeliveryPreferences, error) {
	f.user = u
	f.profile = p
	return notifications.DeliveryPreferences{Alerts: "off", Newsletter: "off"}, nil
}
func (f *fakeNewsletter) SetDeliveryPreferences(_ context.Context, u int, p string, _ notifications.DeliveryPreferences) error {
	f.user = u
	f.profile = p
	f.writes++
	return nil
}
func (f *fakeNewsletter) NewsletterSettings(context.Context) (notifications.NewsletterSettings, error) {
	if f.last != nil {
		return *f.last, nil
	}
	return notifications.NewsletterSettings{Content: newsletter.DefaultSettings("en")}, nil
}
func (f *fakeNewsletter) SetNewsletterSettings(_ context.Context, settings notifications.NewsletterSettings) error {
	f.adminWrites++
	f.last = &settings
	return nil
}

func TestNewsletterAuthority(t *testing.T) {
	f := &fakeNewsletter{}
	deps := pilotDeps(nil, nil)
	deps.NotificationDelivery = f
	h := NewHandler(deps)
	path := Prefix + "/notifications/delivery-preferences"
	for _, headers := range []map[string]string{nil, bearer(memberToken)} {
		r := do(t, h, http.MethodGet, path, "", headers)
		if r.Code == http.StatusOK {
			t.Fatal("profileless caller read delivery preferences")
		}
	}
	r := do(t, h, http.MethodGet, path, "", profileOwner())
	if r.Code != 200 {
		t.Fatalf("read: %d %s", r.Code, r.Body.String())
	}
	if f.profile == "" || f.user == 0 {
		t.Fatal("profile authority not passed to service")
	}
	r = do(t, h, http.MethodPut, path, `{"push":false,"push_episodes":false,"push_requests":true,"push_other":false,"alerts":"off","newsletter":"off","movies":true,"episodes":true,"personal":false,"popular":false}`, profileOwner())
	if r.Code != 200 || f.writes != 1 {
		t.Fatalf("save: %d %s", r.Code, r.Body.String())
	}
	r = do(t, h, http.MethodGet, Prefix+"/admin/notifications/newsletter", "", profileOwner())
	if r.Code != http.StatusForbidden {
		t.Fatalf("member read admin config: %d", r.Code)
	}
	for _, method := range []string{http.MethodGet, http.MethodPost} {
		r = do(t, h, method, Prefix+"/admin/notifications/newsletter/import", `{"installation_id":1}`, profileOwner())
		if r.Code != http.StatusForbidden {
			t.Fatalf("member accessed imports: %d", r.Code)
		}
	}
	settings := newsletterSettingsView(notifications.NewsletterSettings{Content: newsletter.DefaultSettings("pl")})
	settings.Content.Schedule.DailyHour = 19
	settings.Content.Schedule.Timezone = "Europe/Warsaw"
	settings.Content.Delivery.MaxItems = 42
	settings.Content.Delivery.QuietEnabled = true
	settings.Content.CustomVars = []NewsletterCustomVar{{Key: "footer_note", Value: "Example"}}
	raw, err := json.Marshal(settings)
	if err != nil {
		t.Fatal(err)
	}
	r = do(t, h, http.MethodPut, Prefix+"/admin/notifications/newsletter", string(raw), bearer(adminToken))
	if r.Code != http.StatusOK {
		t.Fatalf("admin save: %d %s", r.Code, r.Body.String())
	}
	if f.last == nil || f.last.Content.Schedule.DailyHour != 19 || f.last.Content.Delivery.MaxItems != 42 || !f.last.Content.Delivery.QuietEnabled || len(f.last.Content.CustomVars) != 1 {
		t.Fatalf("admin replacement lost settings: %+v", f.last)
	}
}
