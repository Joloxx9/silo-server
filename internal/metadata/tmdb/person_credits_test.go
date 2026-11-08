package tmdb

import (
	"reflect"
	"testing"
)

// A filmography lists each title once with every role on it, leaves out adult
// titles and talk-show appearances, and puts announced (undated) titles first,
// then the newest.
func TestNormalizePersonCredits(t *testing.T) {
	resp := personCreditsResponse{
		Cast: []personCreditResponse{
			{ID: 1, MediaType: "movie", Title: "Old", ReleaseDate: "2010-05-01", Character: "Hero"},
			{ID: 2, MediaType: "movie", Title: "New", ReleaseDate: "2024-01-01", Character: "Lead"},
			{ID: 3, MediaType: "movie", Title: "Announced", Character: "Lead"},
			{ID: 4, MediaType: "tv", Name: "Talk", FirstAirDate: "2020-01-01", GenreIDs: []int{10767}, Character: "Self"},
			{ID: 5, MediaType: "tv", Name: "Show", FirstAirDate: "2019-12-20", Character: "Witcher", EpisodeCount: 24},
			{ID: 5, MediaType: "tv", Name: "Show", FirstAirDate: "2019-12-20", Character: "Witcher", EpisodeCount: 8},
			{ID: 6, MediaType: "movie", Title: "Adult", ReleaseDate: "2015-01-01", Adult: true},
			{ID: 7, MediaType: "person", Name: "Nobody"},
		},
		Crew: []personCreditResponse{
			{ID: 2, MediaType: "movie", Title: "New", ReleaseDate: "2024-01-01", Job: "Producer"},
			{ID: 2, MediaType: "movie", Title: "New", ReleaseDate: "2024-01-01", Job: "Producer"},
			{ID: 8, MediaType: "movie", Title: "Directed", ReleaseDate: "2018-03-03", Job: "Director"},
		},
	}
	got := normalizePersonCredits(resp)
	type row struct {
		id        int
		media     string
		character string
		jobs      []string
		episodes  int
	}
	var rows []row
	for _, c := range got {
		rows = append(rows, row{c.ID, c.MediaType, c.Character, c.Jobs, c.EpisodeCount})
	}
	want := []row{
		{3, "movie", "Lead", nil, 0},
		{2, "movie", "Lead", []string{"Producer"}, 0},
		{5, "series", "Witcher", nil, 24},
		{8, "movie", "", []string{"Director"}, 0},
		{1, "movie", "Hero", nil, 0},
	}
	if !reflect.DeepEqual(rows, want) {
		t.Fatalf("credits = %+v\nwant %+v", rows, want)
	}
}
