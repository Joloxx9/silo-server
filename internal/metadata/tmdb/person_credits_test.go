package tmdb

import (
	"context"
	"net/http"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"
	"time"
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

type memoryPersonCreditsStore struct {
	mu        sync.Mutex
	credits   []PersonCredit
	fetchedAt time.Time
	saved     chan struct{}
}

func (m *memoryPersonCreditsStore) LoadPersonCredits(context.Context, int) ([]PersonCredit, time.Time, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.credits, m.fetchedAt, m.credits != nil, nil
}

func (m *memoryPersonCreditsStore) SavePersonCredits(_ context.Context, _ int, credits []PersonCredit, fetchedAt time.Time) error {
	m.mu.Lock()
	m.credits, m.fetchedAt = credits, fetchedAt
	m.mu.Unlock()
	m.saved <- struct{}{}
	return nil
}

// A stored filmography answers without TMDB while fresh; a stale one still
// answers at once and is refreshed from TMDB in the background.
func TestGetPersonCreditsServesStoredCopy(t *testing.T) {
	for _, tc := range []struct {
		name        string
		age         time.Duration
		wantRefresh bool
	}{
		{name: "fresh", age: time.Hour},
		{name: "stale", age: PersonCreditsFreshFor + time.Hour, wantRefresh: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var calls atomic.Int32
			client := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
				calls.Add(1)
				_, _ = w.Write([]byte(`{"cast":[{"id":9,"media_type":"movie","title":"Fresh","release_date":"2026-01-01"}]}`))
			})
			store := &memoryPersonCreditsStore{
				credits:   []PersonCredit{{MediaResult: MediaResult{ID: 1, MediaType: "movie", Title: "Stored"}}},
				fetchedAt: time.Now().Add(-tc.age),
				saved:     make(chan struct{}, 1),
			}
			client.SetPersonCreditsStore(store)

			got, err := client.GetPersonCredits(context.Background(), 42)
			if err != nil {
				t.Fatalf("GetPersonCredits: %v", err)
			}
			if len(got) != 1 || got[0].Title != "Stored" {
				t.Fatalf("credits = %+v, want the stored copy", got)
			}
			if tc.wantRefresh {
				select {
				case <-store.saved:
				case <-time.After(5 * time.Second):
					t.Fatal("stale copy was not refreshed")
				}
				if calls.Load() != 1 || store.credits[0].Title != "Fresh" {
					t.Fatalf("calls = %d, stored = %+v, want one refresh storing TMDB's answer", calls.Load(), store.credits)
				}
			} else if calls.Load() != 0 {
				t.Fatalf("TMDB calls = %d, want none for a fresh stored copy", calls.Load())
			}
		})
	}
}
