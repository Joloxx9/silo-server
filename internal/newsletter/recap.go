package newsletter

import (
	"sort"
	"time"
)

type popularTitle struct {
	Name    string
	Viewers int
}

type personalStats struct {
	Plays           int
	Seconds         float64
	Movies          int
	Episodes        int
	TopShow         string
	TopShowEpisodes int
	Recent          []string
}

// recapView is what one email's templates see.
type recapView struct {
	From, To time.Time
	Popular  []popularTitle
	// Personal is nil when the recipient isn't linked to an account or the
	// personal recap is off.
	Personal *personalStats
}

func (v *recapView) empty() bool {
	return v == nil || len(v.Popular) == 0 && (v.Personal == nil || v.Personal.Plays == 0)
}

type Play struct {
	UserID         string   `json:"user_id"`
	ProfileID      string   `json:"profile_id"`
	MediaItemID    string   `json:"media_item_id"`
	MediaTitle     string   `json:"media_title"`
	MediaType      string   `json:"media_type"`
	SeriesTitle    string   `json:"series_title"`
	EndedAt        string   `json:"ended_at"`
	Completed      bool     `json:"completed"`
	PlayedSeconds  *float64 `json:"played_seconds"`
	WatchedSeconds float64  `json:"watched_seconds"`
}

// seconds is the best estimate of how long this play actually ran.
func (p Play) seconds() float64 {
	if p.PlayedSeconds != nil && *p.PlayedSeconds > 0 {
		return *p.PlayedSeconds
	}
	return p.WatchedSeconds
}

func summarizePlays(plays []Play) personalStats {
	var st personalStats
	movies := map[string]bool{}
	episodes := map[string]bool{}
	showEpisodes := map[string]map[string]bool{}
	sorted := append([]Play(nil), plays...)
	sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].EndedAt > sorted[j].EndedAt })
	recent := map[string]bool{}
	for _, p := range sorted {
		secs := p.seconds()
		if secs < 60 && !p.Completed {
			continue
		}
		st.Plays++
		st.Seconds += secs
		name := p.MediaTitle
		switch p.MediaType {
		case "movie":
			movies[p.MediaItemID] = true
		case "episode":
			episodes[p.MediaItemID] = true
			if p.SeriesTitle != "" {
				if showEpisodes[p.SeriesTitle] == nil {
					showEpisodes[p.SeriesTitle] = map[string]bool{}
				}
				showEpisodes[p.SeriesTitle][p.MediaItemID] = true
				name = p.SeriesTitle
			}
		}
		if name != "" && !recent[name] && len(st.Recent) < 5 {
			recent[name] = true
			st.Recent = append(st.Recent, name)
		}
	}
	st.Movies, st.Episodes = len(movies), len(episodes)
	for show, watched := range showEpisodes {
		if n := len(watched); n > st.TopShowEpisodes || n == st.TopShowEpisodes && show < st.TopShow {
			st.TopShow, st.TopShowEpisodes = show, n
		}
	}
	return st
}

// sampleRecap is placeholder content for previews without a linked account.
func sampleRecap(now time.Time) *recapView {
	return &recapView{
		From: now.AddDate(0, 0, -7), To: now,
		Popular: []popularTitle{{Name: "Harbour Lights", Viewers: 4}, {Name: "The Long Way North", Viewers: 3}, {Name: "Night Ferry", Viewers: 2}},
		Personal: &personalStats{Plays: 9, Seconds: 6*3600 + 40*60, Movies: 2, Episodes: 7,
			TopShow: "Harbour Lights", TopShowEpisodes: 5, Recent: []string{"Harbour Lights", "The Long Way North", "Night Ferry"}},
	}
}

// recapVars adds the recap's template variables to data.
func recapVars(data map[string]any, l lang, v *recapView, loc *time.Location) {
	data["has_recap"] = false
	data["has_personal"] = false
	data["has_popular"] = false
	if v == nil {
		return
	}
	// A window from Sunday 18:00 to the next Sunday 18:00 reads as Monday to
	// Sunday: seven days, not eight.
	from, to := v.From.In(loc), v.To.In(loc)
	if h, m, sec := from.Clock(); h != 0 || m != 0 || sec != 0 {
		from = from.AddDate(0, 0, 1)
	}
	if from.After(to) {
		from = to
	}
	data["recap_period"] = l.dateRange(from, to)
	popular := make([]map[string]any, 0, len(v.Popular))
	for i, p := range v.Popular {
		popular = append(popular, map[string]any{
			"rank":          i + 1,
			"name":          p.Name,
			"viewers":       p.Viewers,
			"viewers_label": l.word(p.Viewers, "person", "people", "osoba", "osoby", "osób"),
			"is_first":      i == 0,
			"is_last":       i == len(v.Popular)-1,
		})
	}
	data["popular"] = popular
	data["has_popular"] = len(popular) > 0
	if p := v.Personal; p != nil {
		data["has_personal"] = true
		data["personal_has_activity"] = p.Plays > 0
		data["personal_plays"] = p.Plays
		data["personal_watch_time"] = l.minutes(int(p.Seconds/60 + 0.5))
		data["personal_movies"] = p.Movies
		data["personal_movies_label"] = l.word(p.Movies, "movie", "movies", "film", "filmy", "filmów")
		data["personal_episodes"] = p.Episodes
		data["personal_episodes_label"] = l.word(p.Episodes, "episode", "episodes", "odcinek", "odcinki", "odcinków")
		data["personal_top_show"] = p.TopShow
		data["personal_top_show_episodes"] = p.TopShowEpisodes
		data["personal_top_show_label"] = l.count(p.TopShowEpisodes, "episode", "episodes", "odcinek", "odcinki", "odcinków")
		recent := nameList(p.Recent)
		data["personal_recent"] = recent
		data["has_personal_recent"] = len(recent) > 0
	}
	data["has_recap"] = data["has_personal"] == true || data["has_popular"] == true
}

// RecapFor builds a recap from plays already restricted to the recipient.
// Popular titles must likewise have passed the recipient's catalog scope.
func RecapFor(from, to time.Time, plays []Play, popular []PopularTitle, personal bool) *recapView {
	v := &recapView{From: from, To: to, Popular: popular}
	if personal {
		stats := summarizePlays(plays)
		v.Personal = &stats
	}
	return v
}

type PopularTitle = popularTitle

type RecapView = recapView

func (v *recapView) Empty() bool { return v.empty() }
