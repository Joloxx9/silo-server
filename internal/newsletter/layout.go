package newsletter

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// Section IDs of the designed newsletter, in the default order.
const (
	SectionPersonal = "personal"
	SectionMovies   = "movies"
	SectionShows    = "shows"
	SectionPopular  = "popular"
)

var sectionIDs = []string{SectionPersonal, SectionMovies, SectionShows, SectionPopular}

// Section styles for movies and shows.
const (
	StyleList  = "list"  // compact card: small poster, title, details
	StyleGrid  = "grid"  // poster grid
	StyleCards = "cards" // large card with backdrop, tagline and episodes
)

// Layout controls how the built-in HTML email looks when the admin has not
// replaced it with a template of their own. Texts may use template
// variables such as {{server_name}} and {{recipient_first_name}}.
type Layout struct {
	// Version is 0 for settings saved before the layout existed.
	Version int `json:"version"`
	// CustomHTML renders Templates.HTML instead of the designed layout.
	CustomHTML bool      `json:"custom_html"`
	Sections   []Section `json:"sections"`
	// Eyebrow is the small line above the heading; empty hides it.
	Eyebrow string `json:"eyebrow"`
	// Greeting starts the introduction, such as "Hi {{recipient_first_name}}!".
	// It is left out when the recipient has no name.
	Greeting     string `json:"greeting"`
	ShowSummary  bool   `json:"show_summary"`  // date and "2 new movies" under the heading
	ShowYear     bool   `json:"show_year"`     // year after the title
	ShowDetail   bool   `json:"show_detail"`   // genres, runtime, network
	ShowRatings  bool   `json:"show_ratings"`  // IMDb and Rotten Tomatoes
	ShowOverview bool   `json:"show_overview"` // synopsis
	ShowEpisodes bool   `json:"show_episodes"` // episode lines under a show
	ShowButton   bool   `json:"show_button"`   // button to the server at the end
}

type Section struct {
	ID      string `json:"id"`
	Enabled bool   `json:"enabled"`
	// Style is list, grid or cards; it applies to movies and shows only.
	Style string `json:"style"`
	// Columns is the number of posters per grid row, 2-5.
	Columns int `json:"columns"`
	// Width is full or half; two half sections in a row sit side by side.
	Width string `json:"width"`
}

func defaultLayout(l string) Layout {
	layout := Layout{
		Version:      1,
		Eyebrow:      "{{server_name}}",
		Greeting:     "Hi {{recipient_first_name}}!",
		ShowSummary:  true,
		ShowYear:     true,
		ShowDetail:   true,
		ShowRatings:  true,
		ShowOverview: true,
		ShowEpisodes: true,
		ShowButton:   true,
	}
	if l == langPL {
		layout.Greeting = "Cześć {{recipient_first_name}}!"
	}
	layout.normalize()
	return layout
}

// normalize keeps every known section exactly once, adding missing ones
// enabled at the end, and fills invalid choices with defaults.
func (l *Layout) normalize() {
	seen := map[string]bool{}
	sections := make([]Section, 0, len(sectionIDs))
	for _, section := range l.Sections {
		if !knownSection(section.ID) || seen[section.ID] {
			continue
		}
		seen[section.ID] = true
		sections = append(sections, section)
	}
	for _, id := range sectionIDs {
		if !seen[id] {
			sections = append(sections, Section{ID: id, Enabled: true})
		}
	}
	for i := range sections {
		s := &sections[i]
		if s.Style != StyleGrid && s.Style != StyleCards {
			s.Style = StyleList
		}
		if s.Columns < 2 || s.Columns > 5 {
			s.Columns = 3
		}
		if s.Width != "half" {
			s.Width = "full"
		}
	}
	l.Sections = sections
	l.Version = 1
	l.Eyebrow = strings.TrimSpace(l.Eyebrow)
	l.Greeting = strings.TrimSpace(l.Greeting)
}

func knownSection(id string) bool {
	for _, known := range sectionIDs {
		if known == id {
			return true
		}
	}
	return false
}

var hexColor = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)

func (t Theme) validate() error {
	for name, c := range map[string]string{"accent": t.AccentColor, "background": t.BackgroundColor, "card": t.CardColor, "text": t.TextColor, "muted": t.MutedColor} {
		if c != "" && !hexColor.MatchString(c) {
			return fmt.Errorf("%s color must look like #rrggbb", name)
		}
	}
	return nil
}

func (l Layout) validate() error {
	for _, text := range []string{l.Eyebrow, l.Greeting} {
		if len([]rune(text)) > 200 {
			return errors.New("header texts can be at most 200 characters")
		}
	}
	return nil
}

// luminance returns the relative brightness (0-1) of a #rrggbb color.
func luminance(hex string) float64 {
	value, err := strconv.ParseUint(strings.TrimPrefix(hex, "#"), 16, 32)
	if err != nil || len(hex) != 7 {
		return 0
	}
	r, g, b := float64(value>>16&0xff), float64(value>>8&0xff), float64(value&0xff)
	return (0.299*r + 0.587*g + 0.114*b) / 255
}

// contrastText is the text color that reads on top of bg.
func contrastText(bg string) string {
	if luminance(bg) > 0.6 {
		return "#111318"
	}
	return "#ffffff"
}

// designedRowWidth is the content width of the 640px email in pixels.
const designedRowWidth = 600

// layoutVars adds what the designed template needs to data: the enabled
// sections in order, paired into rows, with their items already reduced to
// the details the layout shows.
func layoutVars(data map[string]any, l lang, layout Layout, movies, shows []map[string]any) {
	accent, _ := data["accent_color"].(string)
	data["accent_text_color"] = contrastText(accent)
	bg, _ := data["background_color"].(string)
	data["is_light"] = luminance(bg) > 0.6
	data["divider_color"] = "rgba(255,255,255,0.08)"
	if luminance(bg) > 0.6 {
		data["divider_color"] = "rgba(0,0,0,0.08)"
	}
	data["eyebrow"] = renderInline(layout.Eyebrow, data)
	greeting := ""
	if data["recipient_first_name"] != "" || !strings.Contains(layout.Greeting, "recipient_") {
		greeting = renderInline(layout.Greeting, data)
	}
	data["greeting"] = greeting
	data["show_summary"] = layout.ShowSummary
	data["show_button"] = layout.ShowButton && data["server_url"] != ""
	for k, v := range l.layoutLabels() {
		data[k] = v
	}

	hasItems := len(movies) > 0 || len(shows) > 0
	quiet := !hasItems && data["is_digest"] == true
	var blocks []map[string]any
	for _, s := range layout.Sections {
		block := map[string]any{"half": s.Width == "half", "is_personal": false, "is_popular": false,
			"is_catalog": false, "is_quiet": false, "is_list": false, "is_grid": false, "is_cards": false,
			"heading": "", "items": []map[string]any{}, "grid_rows": []map[string]any{}}
		switch s.ID {
		case SectionPersonal:
			if data["has_personal"] != true {
				continue
			}
			block["is_personal"] = true
		case SectionPopular:
			if data["has_popular"] != true {
				continue
			}
			block["is_popular"] = true
			block["heading"] = data["t_popular"]
		case SectionMovies, SectionShows:
			if !s.Enabled {
				continue
			}
			var items []map[string]any
			if s.ID == SectionMovies {
				block["heading"] = data["t_movies"]
				for _, m := range movies {
					items = append(items, designedMovie(m, layout))
				}
			} else {
				block["heading"] = data["t_shows"]
				for _, sh := range shows {
					items = append(items, designedShow(sh, layout))
				}
			}
			if len(items) == 0 {
				// One note for an empty digest, where the first catalog section is.
				if !quiet {
					continue
				}
				block["is_quiet"], quiet = true, false
				break
			}
			block["is_catalog"] = true
			block["items"] = items
			block["is_"+s.Style] = true
			block["columns"] = s.Columns
		}
		blocks = append(blocks, block)
	}

	var rows []map[string]any
	for i := 0; i < len(blocks); i++ {
		if blocks[i]["half"] == true && i+1 < len(blocks) && blocks[i+1]["half"] == true {
			left, right := blocks[i], blocks[i+1]
			left["pad"], right["pad"] = "0 6px 0 0", "0 0 0 6px"
			rows = append(rows, map[string]any{"blocks": []map[string]any{left, right}})
			i++
			continue
		}
		// A half section without a partner takes the whole row.
		blocks[i]["half"], blocks[i]["pad"] = false, "0"
		rows = append(rows, map[string]any{"blocks": []map[string]any{blocks[i]}})
	}
	// Grid cells are sized once the block's width is known.
	for _, block := range blocks {
		if block["is_grid"] == true {
			gridVars(block, block["items"].([]map[string]any), block["columns"].(int))
		}
	}
	data["layout_rows"] = rows
}

// gridVars splits items into poster rows sized for the block's width.
func gridVars(block map[string]any, items []map[string]any, cols int) {
	const gap = 8
	row := designedRowWidth
	if block["half"] == true {
		row = (designedRowWidth - 12) / 2
	}
	width := (row-gap*(cols+1))/cols - 16
	var rows []map[string]any
	for start := 0; start < len(items); start += cols {
		cells := items[start:min(start+cols, len(items))]
		fill := make([]map[string]any, cols-len(cells))
		for i := range fill {
			fill[i] = map[string]any{}
		}
		rows = append(rows, map[string]any{"cells": cells, "fill": fill})
	}
	block["grid_rows"] = rows
	block["gap"] = gap
	block["cell_width"] = width
	block["cell_height"] = width * 3 / 2
	block["cell_percent"] = strconv.Itoa(100/cols) + "%"
}

func str(v any) string {
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	return fmt.Sprint(v)
}

func joinDot(parts ...string) string {
	var kept []string
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			kept = append(kept, p)
		}
	}
	return strings.Join(kept, " · ")
}

// designedItem is the common shape of a movie or show in the designed
// layout. Every key is set, so a missing value never falls through to a
// variable of the same name further out.
func designedItem(layout Layout, name, url, poster, backdropBG, year, detail, overview string, v map[string]any, prefix string) map[string]any {
	item := map[string]any{
		"name": name, "url": url, "poster": poster, "backdrop_bg": backdropBG,
		"year": "", "detail": "", "overview": "", "badge": "", "tagline": "", "cast": "",
		"has_ratings": false, "imdb_rating": "", "rt_critic": "", "rt_audience": "",
		"rt_critic_fresh": false, "rt_audience_fresh": false,
		"episodes": []map[string]any{}, "has_episodes": false,
	}
	if layout.ShowYear {
		item["year"] = year
	}
	if layout.ShowDetail {
		item["detail"] = detail
	}
	if layout.ShowOverview {
		item["overview"] = overview
	}
	if layout.ShowRatings {
		for _, k := range []string{"has_ratings", "imdb_rating", "rt_critic", "rt_audience", "rt_critic_fresh", "rt_audience_fresh"} {
			if value, ok := v[prefix+k]; ok {
				item[k] = value
			}
		}
	}
	return item
}

func designedMovie(m map[string]any, layout Layout) map[string]any {
	detail := joinDot(str(m["genres"]), str(m["runtime"]), str(m["content_rating"]))
	item := designedItem(layout, str(m["title"]), str(m["url"]), str(m["poster"]), str(m["backdrop_bg"]),
		str(m["year"]), detail, str(m["synopsis"]), m, "")
	if layout.ShowOverview {
		item["tagline"] = str(m["tagline"])
	}
	if layout.ShowDetail {
		item["cast"] = str(m["cast"])
	}
	return item
}

func designedShow(s map[string]any, layout Layout) map[string]any {
	detail := joinDot(str(s["network"]), str(s["show_genres"]))
	item := designedItem(layout, str(s["show_name"]), str(s["show_url"]), str(s["show_poster"]), str(s["show_backdrop_bg"]),
		str(s["show_year"]), detail, str(s["show_synopsis"]), s, "show_")
	// The new episodes are what the email is about, so the badge stays.
	item["badge"] = joinDot(str(s["episodes_label"]), str(s["episode_range"]))
	if eps, ok := s["episodes"].([]map[string]any); ok && layout.ShowEpisodes && len(eps) > 0 {
		lines := make([]map[string]any, 0, len(eps))
		for _, ep := range eps {
			synopsis := ""
			if layout.ShowOverview {
				synopsis = str(ep["episode_synopsis"])
			}
			lines = append(lines, map[string]any{
				"episode_code": str(ep["episode_code"]), "episode_name": str(ep["episode_name"]),
				"url": str(ep["url"]), "runtime": str(ep["runtime"]), "air_date": str(ep["air_date"]),
				"episode_synopsis": synopsis,
			})
		}
		item["episodes"], item["has_episodes"] = lines, true
	}
	return item
}

// layoutLabels are the designed template's own words.
func (l lang) layoutLabels() map[string]string {
	if l == langPL {
		return map[string]string{
			"t_movies": "Nowe filmy", "t_shows": "Nowe odcinki i seriale", "t_popular": "Najpopularniejsze na serwerze",
			"t_your_day": "Twój dzień", "t_your_week": "Twój tydzień", "t_watched": "oglądania",
			"t_top_show": "Najczęściej oglądany serial:", "t_recently": "Ostatnio:",
			"t_no_plays_day":  "Dziś nie było seansów — poniżej nowości, od których warto zacząć.",
			"t_no_plays_week": "W tym tygodniu nie było seansów — poniżej nowości, od których warto zacząć.",
			"t_quiet_day":     "Dziś nie pojawiło się nic nowego. To dobry moment, żeby nadrobić zaległości!",
			"t_quiet_week":    "W tym tygodniu nie pojawiło się nic nowego. To dobry moment, żeby nadrobić zaległości!",
			"t_recap":         "oto Twoje podsumowanie.", "t_and_more": "…i jeszcze", "t_more": "na serwerze.",
			"t_starring": "Występują:", "t_aired": "Emisja", "t_manage": "Zarządzaj powiadomieniami", "t_unsubscribe": "Wypisz się",
		}
	}
	return map[string]string{
		"t_movies": "New movies", "t_shows": "New episodes and shows", "t_popular": "Popular on the server",
		"t_your_day": "Your day", "t_your_week": "Your week", "t_watched": "watched",
		"t_top_show": "Most watched show:", "t_recently": "Recently:",
		"t_no_plays_day":  "Nothing watched today. Here's something to start with.",
		"t_no_plays_week": "Nothing watched this week. Here's something to start with.",
		"t_quiet_day":     "Nothing new arrived today. A good moment to catch up!",
		"t_quiet_week":    "Nothing new arrived this week. A good moment to catch up!",
		"t_recap":         "here's your recap.", "t_and_more": "…and", "t_more": "more on the server.",
		"t_starring": "Starring", "t_aired": "Aired", "t_manage": "Manage alerts", "t_unsubscribe": "Unsubscribe",
	}
}
