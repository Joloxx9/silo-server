package notifications

import (
	"testing"
	"time"

	"github.com/Silo-Server/silo-server/internal/newsletter"
)

func TestNewsletterCalendarSchedule(t *testing.T) {
	loc, err := time.LoadLocation("Europe/Warsaw")
	if err != nil {
		t.Fatal(err)
	}
	s := newsletter.Schedule{Timezone: "Europe/Warsaw", DailyHour: 8, WeeklyDay: 0, WeeklyHour: 8}
	now := time.Date(2026, 10, 25, 8, 0, 0, 0, loc) // daylight-saving time ended overnight
	yesterday := now.AddDate(0, 0, -1)
	lastWeek := now.AddDate(0, 0, -7)
	for _, tc := range []struct {
		name, mode string
		now        time.Time
		last       *time.Time
		want       bool
	}{
		{"before hour", "daily", now.Add(-time.Minute), &yesterday, false},
		{"daily across DST", "daily", now, &yesterday, true},
		{"already sent", "daily", now, &now, false},
		{"weekly across DST", "weekly", now, &lastWeek, true},
		{"weekly catch up", "weekly", now.AddDate(0, 0, 1), &lastWeek, true},
		{"weekly already sent", "weekly", now.AddDate(0, 0, 1), &now, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := newsletterDue(tc.now, tc.last, tc.mode, s); got != tc.want {
				t.Fatalf("due=%v want %v", got, tc.want)
			}
		})
	}
}

func TestPushPreferencesDoNotEnableEmail(t *testing.T) {
	p := defaultDeliveryPreferences()
	if p.Alerts != "off" || p.Newsletter != "off" {
		t.Fatal("new profiles must not opt in to email")
	}
	p.PushEpisodes = false
	if p.allowsPush(DeliveryTypeEpisodeAvailable) {
		t.Fatal("episode push ignored opt-out")
	}
	if !p.allowsPush(DeliveryTypeRequestFulfilled) {
		t.Fatal("episode choice disabled request updates")
	}
	p.Push = false
	for _, kind := range []string{DeliveryTypeEpisodeAvailable, DeliveryTypeRequestFulfilled, "unknown"} {
		if p.allowsPush(kind) {
			t.Fatalf("disabled push allowed %s", kind)
		}
	}
}

func TestNewsletterImportedSubscriptionsKeepConsentAndFilters(t *testing.T) {
	r := newsletter.Recipient{Enabled: true, Verified: true, Mode: "instant_daily", Types: []string{"episode"}}
	p, ok := importedDeliveryPreferences(r)
	if !ok || p.Alerts != "instant" || p.Newsletter != "daily" || p.Movies || !p.Episodes {
		t.Fatalf("combined subscription lost its mode or filter: %+v %v", p, ok)
	}
	for _, change := range []func(*newsletter.Recipient){
		func(r *newsletter.Recipient) { r.Verified = false },
		func(r *newsletter.Recipient) { r.Enabled = false },
		func(r *newsletter.Recipient) { r.Libraries = []string{"restricted"} },
		func(r *newsletter.Recipient) { r.MaxAgeHours = 24 },
		func(r *newsletter.Recipient) { r.Types = []string{"unknown"} },
	} {
		copy := r
		change(&copy)
		if _, ok := importedDeliveryPreferences(copy); ok {
			t.Fatalf("unsafe subscription activated: %+v", copy)
		}
	}
}

func TestNewsletterQuietHours(t *testing.T) {
	c := newsletter.DefaultSettings("en")
	c.Schedule.Timezone = "Europe/Warsaw"
	c.Delivery.QuietEnabled = true
	c.Delivery.QuietStart, c.Delivery.QuietEnd = 22, 7
	loc := newsletterLocation(c.Schedule)
	for _, tc := range []struct {
		hour int
		want bool
	}{{21, false}, {22, true}, {0, true}, {6, true}, {7, false}} {
		if got := newsletterQuiet(time.Date(2026, 10, 25, tc.hour, 0, 0, 0, loc), c); got != tc.want {
			t.Fatalf("quiet at %d: %v", tc.hour, got)
		}
	}
}
