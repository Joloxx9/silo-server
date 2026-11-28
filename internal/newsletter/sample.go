package newsletter

import (
	"context"
	"encoding/base64"
	"fmt"
	"html"
	"time"
)

// samplePoster is a 2:3 placeholder poster, so a preview shows how the
// layout places artwork without reaching the catalog.
func samplePoster(title, from, to string) string {
	svg := fmt.Sprintf(`<svg xmlns="http://www.w3.org/2000/svg" width="400" height="600" viewBox="0 0 400 600"><defs><linearGradient id="g" x1="0" y1="0" x2="0.4" y2="1"><stop offset="0" stop-color="%s"/><stop offset="1" stop-color="%s"/></linearGradient></defs><rect width="400" height="600" fill="url(#g)"/><text x="32" y="540" fill="#ffffff" font-family="Helvetica,Arial,sans-serif" font-size="34" font-weight="700">%s</text></svg>`, from, to, html.EscapeString(title))
	return "data:image/svg+xml;base64," + base64.StdEncoding.EncodeToString([]byte(svg))
}

// sampleEntries is placeholder content for previews, with placeholder
// posters in place of artwork.
func sampleEntries() []Entry {
	now := time.Now().UTC()
	ep := func(n int) *int { return &n }
	at := func(d time.Duration) string { return now.Add(-d).Format(time.RFC3339Nano) }
	series := &Detail{Card: Card{
		ContentID: "series:sample-harbour-lights", Type: "series", Title: "Harbour Lights", Year: 2024,
		Overview: "In a fog-bound fishing town, a newly arrived harbourmaster uncovers a decades-old disappearance that the locals would rather stay buried.",
		Genres:   []string{"Drama", "Mystery"}, Networks: []string{"BBC One"}, RatingTMDB: 8.1, ShowStatus: "Returning Series",
		PosterURL: samplePoster("Harbour Lights", "#24435e", "#0b1622"), RatingIMDB: 8.3, RatingRTCritic: 91,
	}, SeasonCount: 2}
	return []Entry{
		{
			Card: Card{
				ContentID: "episode:sample-1", Type: "episode", Title: "The Lantern Room", AddedAt: at(20 * time.Minute),
				Overview: "Mara finally gets inside the lighthouse, but what she finds in the lantern room changes everything she thought she knew about the night of the storm.",
				SeriesID: series.ContentID, SeriesTitle: series.Title, SeasonNumber: ep(2), EpisodeNumber: ep(3),
				ReleaseDate: now.AddDate(0, 0, -1).Format("2006-01-02"), Runtime: 58,
			},
			Library: LibraryRef{ID: "tv", Name: "TV Shows"},
			Series:  series,
			Pick:    PickWatching,
		},
		{
			Card: Card{
				ContentID: "episode:sample-2", Type: "episode", Title: "Low Tide", AddedAt: at(21 * time.Minute),
				Overview: "A body washes up on the morning tide, and the town closes ranks.",
				SeriesID: series.ContentID, SeriesTitle: series.Title, SeasonNumber: ep(2), EpisodeNumber: ep(4),
				ReleaseDate: now.Format("2006-01-02"), Runtime: 54,
			},
			Library: LibraryRef{ID: "tv", Name: "TV Shows"},
			Series:  series,
			Pick:    PickWatching,
		},
		{
			Card: Card{
				ContentID: "movie:sample-3", Type: "movie", Title: "The Long Way North", Year: 2025, AddedAt: at(45 * time.Minute),
				Overview: "Two estranged sisters drive their late father's vintage camper from Cornwall to Cape Wrath to scatter his ashes, and discover he left them one last puzzle along the way.",
				Genres:   []string{"Comedy", "Drama"}, Runtime: 112, ContentRating: "12A", RatingTMDB: 7.4,
				PosterURL: samplePoster("The Long Way North", "#b0643a", "#3a1b10"), RatingIMDB: 7.2, RatingRTCritic: 84, RatingRTAudience: 78,
			},
			Library: LibraryRef{ID: "movies", Name: "Movies"},
			Movie:   &Detail{Tagline: "Some journeys take a lifetime.", Cast: []CastMember{{Name: "Ada Lark"}, {Name: "June Tamsin"}}},
		},
		{
			Card: Card{
				ContentID: "movie:sample-4", Type: "movie", Title: "Night Ferry", Year: 2024, AddedAt: at(50 * time.Minute),
				Overview: "On the last crossing before a storm closes the port, a ship's purser realises one of her passengers was never on the manifest.",
				Genres:   []string{"Thriller"}, Runtime: 98, ContentRating: "15", RatingIMDB: 6.9, RatingRTCritic: 72,
				PosterURL: samplePoster("Night Ferry", "#3d3a6b", "#0e0d1f"),
			},
			Library: LibraryRef{ID: "movies", Name: "Movies"},
			Pick:    PickWatchlist,
		},
		{
			Card: Card{
				ContentID: "movie:sample-5", Type: "movie", Title: "Paper Moons", Year: 2023, AddedAt: at(55 * time.Minute),
				Overview: "A retired stage magician agrees to teach one last student, who turns out to be after the secret behind his most famous trick.",
				Genres:   []string{"Drama"}, Runtime: 104, RatingIMDB: 7.6, RatingRTCritic: 88, RatingRTAudience: 90,
				PosterURL: samplePoster("Paper Moons", "#5e7a3a", "#16200c"),
			},
			Library: LibraryRef{ID: "movies", Name: "Movies"},
		},
	}
}

func Preview(ctx context.Context, settings Settings) (*Rendered, error) {
	entries := sampleEntries()
	if settings.Language == langPL {
		polishSample(entries)
	}
	PrioritizeEntries(entries)
	return Render(ctx, RenderEnv{Settings: settings, ServerName: "Silo", PublicURL: "https://example.test", Recipient: Recipient{Name: "Viewer"}, Mode: "preview", Now: time.Now(), Preview: true, Recap: sampleRecap(time.Now())}, entries, nil)
}

// polishSample swaps the sample texts for Polish ones, as the catalog's
// Polish metadata would.
func polishSample(entries []Entry) {
	overviews := map[string]string{
		"series:sample-harbour-lights": "W spowitym mgłą miasteczku rybackim nowa kapitan portu odkrywa sprawę zaginięcia sprzed dekad, o której mieszkańcy woleliby zapomnieć.",
		"episode:sample-1":             "Mara wreszcie dostaje się do latarni, a to, co znajduje w pomieszczeniu z laterną, zmienia wszystko, co wiedziała o nocy sztormu.",
		"episode:sample-2":             "Poranny przypływ wyrzuca na brzeg ciało, a miasteczko zamyka się w sobie.",
		"movie:sample-3":               "Dwie skłócone siostry jadą starym kamperem zmarłego ojca z Kornwalii na przylądek Wrath, by rozsypać jego prochy, i odkrywają, że zostawił im po drodze ostatnią zagadkę.",
		"movie:sample-4":               "Podczas ostatniego rejsu przed sztormem, który zamknie port, ochmistrzyni odkrywa, że jednej z pasażerek nigdy nie było na liście.",
		"movie:sample-5":               "Emerytowany iluzjonista zgadza się przyjąć ostatniego ucznia, który okazuje się polować na sekret jego najsłynniejszej sztuczki.",
	}
	genres := map[string]string{"Drama": "Dramat", "Mystery": "Tajemnica", "Comedy": "Komedia", "Thriller": "Thriller"}
	translate := func(c *Card) {
		if o, ok := overviews[c.ContentID]; ok {
			c.Overview = o
		}
		g := make([]string, len(c.Genres))
		for i, name := range c.Genres {
			g[i] = genres[name]
		}
		c.Genres = g
	}
	series := entries[0].Series
	translate(&series.Card)
	for i := range entries {
		e := &entries[i]
		translate(&e.Card)
		if e.Movie != nil {
			e.Movie.Tagline = "Niektóre podróże trwają całe życie."
		}
		if e.Card.Type == "episode" {
			e.Library.Name = "Seriale"
		} else {
			e.Library.Name = "Filmy"
		}
	}
}
