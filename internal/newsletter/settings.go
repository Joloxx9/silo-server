package newsletter

import (
	"strings"
	"time"
)

const maxAgeLimit = 24 * 60

type Settings struct {
	Templates  Templates   `json:"templates"`
	Theme      Theme       `json:"theme"`
	CustomVars []CustomVar `json:"custom_vars"`
	Schedule   Schedule    `json:"schedule"`
	Delivery   Delivery    `json:"delivery"`
	// Language is the language of the renderer's own words
	// (summaries, labels, dates): en or pl.
	Language string `json:"language"`
	Recap    Recap  `json:"recap"`
}

// Recap is the newsletter part of digests: each recipient's own viewing and
// what was popular on the server over the digest's period.
type Recap struct {
	// Personal adds the receiving profile's own viewing to digests.
	Personal bool `json:"personal"`
	// Popular adds the titles most accounts watched.
	Popular bool `json:"popular"`
	// PopularCount is how many popular titles are listed.
	PopularCount int `json:"popular_count"`
	// PopularMinViewers is how many accounts must have watched a title for
	// it to be listed. Never below 2, so one person's viewing is never shown.
	PopularMinViewers int `json:"popular_min_viewers"`
	// WeeklyWhenEmpty sends the weekly digest even when nothing new arrived,
	// as long as the recap has something to show.
	WeeklyWhenEmpty bool `json:"weekly_when_empty"`
}

type Templates struct {
	Subject string `json:"subject"`
	HTML    string `json:"html"`
	Text    string `json:"text"`
}

type Theme struct {
	AccentColor     string `json:"accent_color"`
	BackgroundColor string `json:"background_color"`
	CardColor       string `json:"card_color"`
	TextColor       string `json:"text_color"`
	MutedColor      string `json:"muted_color"`
	LogoURL         string `json:"logo_url"`
	HeaderTitle     string `json:"header_title"`
	Intro           string `json:"intro"`
	FooterText      string `json:"footer_text"`
	ButtonText      string `json:"button_text"`
}

type CustomVar struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

type Schedule struct {
	Timezone     string `json:"timezone"`
	DailyHour    int    `json:"daily_hour"`
	DailyMinute  int    `json:"daily_minute"`
	WeeklyDay    int    `json:"weekly_day"` // 0 = Sunday
	WeeklyHour   int    `json:"weekly_hour"`
	WeeklyMinute int    `json:"weekly_minute"`
	// WeeklyDays is how many days a weekly digest and its recap cover.
	WeeklyDays int `json:"weekly_days"`
}

type Delivery struct {
	Paused         bool `json:"paused"`
	SettleMinutes  int  `json:"settle_minutes"`
	MaxItems       int  `json:"max_items"`
	EmbedImages    bool `json:"embed_images"`
	MaxImages      int  `json:"max_images"`
	SynopsisLength int  `json:"synopsis_length"`
	// InstantGapMinutes is the least time between two instant emails to one
	// recipient; items that arrive sooner are held and sent together.
	InstantGapMinutes int `json:"instant_gap_minutes"`
	// InstantDailyCap limits instant emails per recipient per day (0 = no
	// limit); items past the cap go out in the first email of the next day.
	InstantDailyCap int `json:"instant_daily_cap"`
	// Quiet hours hold instant emails overnight. Anything that arrives
	// between QuietStart and QuietEnd (local hours, the window may wrap past
	// midnight) goes out together when the window ends.
	QuietEnabled bool `json:"quiet_enabled"`
	QuietStart   int  `json:"quiet_start"`
	QuietEnd     int  `json:"quiet_end"`
	// MaxAgeHours is the default for how recently an item must have been
	// added to be emailed at all. Anything older is never treated as new,
	// whatever a recipient's state says. Recipients can choose their own.
	MaxAgeHours int `json:"max_age_hours"`
	// BurstThreshold: a send with more items than this goes out as one
	// compact summary (a card per show, no episode lists, no cap) instead of
	// a long email. 0 turns it off.
	BurstThreshold int `json:"burst_threshold"`
	// MaxMovies and MaxShows cap movies and shows in one email separately;
	// 0 means only MaxItems applies. Anything cut is counted in more_count.
	MaxMovies int `json:"max_movies"`
	MaxShows  int `json:"max_shows"`
	// SendDelaySeconds is the pause between two recipients' emails, for mail
	// servers that limit how fast they accept messages.
	SendDelaySeconds int `json:"send_delay_seconds"`
}

// Recipient is one address that receives alerts. Admins add recipients on
// the admin page; users add themselves on the self-service page.
type Recipient struct {
	ID        string   `json:"id"`
	Name      string   `json:"name"`
	Email     string   `json:"email"`
	Enabled   bool     `json:"enabled"`
	Mode      string   `json:"mode"`      // instant | instant_daily | daily | weekly
	Types     []string `json:"types"`     // movie, episode; empty = all
	Libraries []string `json:"libraries"` // library ids; empty = all
	// MaxAgeHours is how recently an item must have been added to count as
	// new for this recipient; 0 uses the server default.
	MaxAgeHours int       `json:"max_age_hours,omitempty"`
	Source      string    `json:"source"` // admin | user
	UserID      string    `json:"user_id,omitempty"`
	Verified    bool      `json:"verified"`
	Token       string    `json:"token"`
	Created     time.Time `json:"created"`

	// PendingEmail and VerifyToken hold an address change a user has not
	// confirmed yet.
	PendingEmail string `json:"pending_email,omitempty"`
	VerifyToken  string `json:"verify_token,omitempty"`
}

func defaultSettings() Settings {
	return defaultSettingsFor(langEN)
}

// defaultSettingsFor returns the built-in settings with templates and
// wording in language l.
func defaultSettingsFor(l string) Settings {
	s := Settings{
		Language:   cleanLanguage(l),
		CustomVars: []CustomVar{},
		Templates: Templates{
			Subject: defaultSubjectTemplate,
			HTML:    defaultHTMLTemplate,
			Text:    defaultTextTemplate,
		},
		Theme: Theme{
			AccentColor:     "#7c5cff",
			BackgroundColor: "#0f1115",
			CardColor:       "#181b22",
			TextColor:       "#f2f3f5",
			MutedColor:      "#9aa0ab",
			HeaderTitle:     "New on {{server_name}}",
			Intro:           "Here's what just landed on the server.",
			FooterText:      "You're receiving this because you signed up for new media alerts.",
			ButtonText:      "Watch now",
		},
		Schedule: Schedule{DailyHour: 18, WeeklyDay: 5, WeeklyHour: 18, WeeklyDays: 7},
		Delivery: Delivery{
			SettleMinutes:     5,
			MaxItems:          60,
			EmbedImages:       true,
			MaxImages:         40,
			SynopsisLength:    320,
			InstantGapMinutes: 10,
			MaxAgeHours:       168,
			BurstThreshold:    25,
		},
		Recap: Recap{
			Personal:          true,
			Popular:           true,
			PopularCount:      5,
			PopularMinViewers: 2,
			WeeklyWhenEmpty:   true,
		},
	}
	if s.Language == langPL {
		s.Templates = Templates{Subject: plSubjectTemplate, HTML: plHTMLTemplate, Text: plTextTemplate}
		s.Theme.HeaderTitle = "Nowości na {{server_name}}"
		s.Theme.Intro = "Oto, co właśnie pojawiło się na serwerze."
		s.Theme.FooterText = "Dostajesz ten e-mail, bo masz włączone powiadomienia o nowościach."
		s.Theme.ButtonText = "Oglądaj"
	}
	return s
}

func (s *Settings) normalize() {
	if s.CustomVars == nil {
		s.CustomVars = []CustomVar{}
	}
	s.Language = cleanLanguage(s.Language)
	def := defaultSettingsFor(s.Language)
	s.Recap.PopularCount = clamp(s.Recap.PopularCount, 1, 20)
	s.Recap.PopularMinViewers = clamp(s.Recap.PopularMinViewers, 2, 50)
	if strings.TrimSpace(s.Templates.Subject) == "" {
		s.Templates.Subject = def.Templates.Subject
	}
	if strings.TrimSpace(s.Templates.HTML) == "" {
		s.Templates.HTML = def.Templates.HTML
	}
	if strings.TrimSpace(s.Templates.Text) == "" {
		s.Templates.Text = def.Templates.Text
	}
	if s.Delivery.SettleMinutes < 0 {
		s.Delivery.SettleMinutes = 0
	}
	if s.Delivery.SettleMinutes > 240 {
		s.Delivery.SettleMinutes = 240
	}
	if s.Delivery.MaxItems <= 0 {
		s.Delivery.MaxItems = def.Delivery.MaxItems
	}
	if s.Delivery.MaxItems > 500 {
		s.Delivery.MaxItems = 500
	}
	if s.Delivery.MaxImages < 0 {
		s.Delivery.MaxImages = 0
	}
	if s.Delivery.MaxImages > 150 {
		s.Delivery.MaxImages = 150
	}
	if s.Delivery.SynopsisLength < 0 {
		s.Delivery.SynopsisLength = 0
	}
	s.Delivery.InstantGapMinutes = clamp(s.Delivery.InstantGapMinutes, 0, 1440)
	s.Delivery.InstantDailyCap = clamp(s.Delivery.InstantDailyCap, 0, 500)
	s.Delivery.BurstThreshold = clamp(s.Delivery.BurstThreshold, 0, 500)
	s.Delivery.QuietStart = clamp(s.Delivery.QuietStart, 0, 23)
	s.Delivery.QuietEnd = clamp(s.Delivery.QuietEnd, 0, 23)
	if s.Delivery.MaxAgeHours <= 0 {
		s.Delivery.MaxAgeHours = 168
	}
	s.Delivery.MaxAgeHours = clamp(s.Delivery.MaxAgeHours, 1, maxAgeLimit)
	s.Schedule.DailyHour = clamp(s.Schedule.DailyHour, 0, 23)
	s.Schedule.WeeklyHour = clamp(s.Schedule.WeeklyHour, 0, 23)
	s.Schedule.DailyMinute = clamp(s.Schedule.DailyMinute, 0, 59)
	s.Schedule.WeeklyMinute = clamp(s.Schedule.WeeklyMinute, 0, 59)
	if s.Schedule.WeeklyDays <= 0 {
		s.Schedule.WeeklyDays = 7
	}
	s.Schedule.WeeklyDays = clamp(s.Schedule.WeeklyDays, 1, 60)
	s.Delivery.MaxMovies = clamp(s.Delivery.MaxMovies, 0, 500)
	s.Delivery.MaxShows = clamp(s.Delivery.MaxShows, 0, 500)
	s.Delivery.SendDelaySeconds = clamp(s.Delivery.SendDelaySeconds, 0, 600)
	s.Schedule.WeeklyDay = clamp(s.Schedule.WeeklyDay, 0, 6)
	cleaned := s.CustomVars[:0]
	for _, v := range s.CustomVars {
		v.Key = strings.TrimSpace(v.Key)
		if v.Key != "" {
			cleaned = append(cleaned, v)
		}
	}
	s.CustomVars = cleaned
}

func (s Settings) location() *time.Location {
	if s.Schedule.Timezone != "" {
		if loc, err := time.LoadLocation(s.Schedule.Timezone); err == nil {
			return loc
		}
	}
	return time.UTC
}

func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func DefaultSettings(language string) Settings { return defaultSettingsFor(language) }
func (s *Settings) Normalize()                 { s.normalize() }

// Validate checks a saved template before it can enter the delivery worker.
func (s Settings) Validate() error {
	_, err := renderTemplates(s, map[string]any{})
	return err
}
