package scrob

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Silo-Server/silo-server/internal/historyimport"
	"github.com/Silo-Server/silo-server/internal/watchsync"
)

func TestScrobTimeUnmarshalsNaiveTimestamps(t *testing.T) {
	// Regression test: Scrob serializes timestamps with Python's naive
	// datetime.isoformat(), no timezone suffix - encoding/json's default
	// time.Time unmarshaler rejects that outright.
	var got scrobTime
	if err := json.Unmarshal([]byte(`"2026-10-01T12:06:08"`), &got); err != nil {
		t.Fatalf("UnmarshalJSON() error: %v", err)
	}
	want := time.Date(2026, 10, 1, 12, 6, 8, 0, time.UTC)
	if !got.Time().Equal(want) {
		t.Fatalf("Time() = %v, want %v", got.Time(), want)
	}

	// A future, tz-aware response (RFC 3339) must still decode.
	var tzAware scrobTime
	if err := json.Unmarshal([]byte(`"2026-10-01T12:06:08Z"`), &tzAware); err != nil {
		t.Fatalf("UnmarshalJSON() error on RFC3339 input: %v", err)
	}
	if !tzAware.Time().Equal(want) {
		t.Fatalf("Time() = %v, want %v", tzAware.Time(), want)
	}

	var empty scrobTime
	if err := json.Unmarshal([]byte(`""`), &empty); err != nil {
		t.Fatalf("UnmarshalJSON() error on empty input: %v", err)
	}
	if !empty.Time().IsZero() {
		t.Fatalf("Time() = %v, want zero", empty.Time())
	}
}

func TestNormalizeServerURL(t *testing.T) {
	cases := []struct {
		name    string
		in      string
		want    string
		wantErr bool
	}{
		{name: "adds scheme", in: "scrob.example.com", want: "https://scrob.example.com"},
		{name: "trims trailing slash", in: "https://scrob.example.com/", want: "https://scrob.example.com"},
		{name: "drops path and query", in: "https://scrob.example.com/foo?x=1", want: "https://scrob.example.com"},
		{name: "keeps port", in: "http://localhost:8787", want: "http://localhost:8787"},
		{name: "empty is an error", in: "   ", wantErr: true},
		{name: "non-http scheme is an error", in: "ftp://scrob.example.com", wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := normalizeServerURL(tc.in)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("normalizeServerURL(%q) = %q, want error", tc.in, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("normalizeServerURL(%q) unexpected error: %v", tc.in, err)
			}
			if got != tc.want {
				t.Fatalf("normalizeServerURL(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestMovieKeyPrefersIMDbThenTMDBThenTVDB(t *testing.T) {
	if got := movieKey(100, 200, "tt1"); got != "imdb:tt1" {
		t.Fatalf("movieKey = %q, want imdb:tt1", got)
	}
	if got := movieKey(100, 200, ""); got != "tmdb:100" {
		t.Fatalf("movieKey = %q, want tmdb:100", got)
	}
	if got := movieKey(0, 200, ""); got != "tvdb:200" {
		t.Fatalf("movieKey = %q, want tvdb:200", got)
	}
	if got := movieKey(0, 0, ""); got != "" {
		t.Fatalf("movieKey = %q, want empty", got)
	}
}

func TestEpisodeKeyFallsBackToShowContext(t *testing.T) {
	// A direct episode id wins outright.
	if got := episodeKey(0, 0, 1, 2, 0, 555, ""); got != "tvdb:555" {
		t.Fatalf("episodeKey = %q, want tvdb:555", got)
	}
	// No episode-level id: fall back to the show plus season/episode numbers.
	if got := episodeKey(0, 999, 1, 2, 0, 0, ""); got != "show:tvdb:999:s1:e2" {
		t.Fatalf("episodeKey = %q, want show:tvdb:999:s1:e2", got)
	}
	if got := episodeKey(888, 0, 1, 2, 0, 0, ""); got != "show:tmdb:888:s1:e2" {
		t.Fatalf("episodeKey = %q, want show:tmdb:888:s1:e2", got)
	}
	if got := episodeKey(0, 0, 1, 2, 0, 0, ""); got != "" {
		t.Fatalf("episodeKey = %q, want empty", got)
	}
}

func TestProviderRatingRounds(t *testing.T) {
	cases := map[float64]int{0: 0, 7: 7, 7.4: 7, 7.6: 8, 10: 10}
	for in, want := range cases {
		if got := providerRating(in); got != want {
			t.Fatalf("providerRating(%v) = %d, want %d", in, got, want)
		}
	}
}

func TestWatchEventBodyMovie(t *testing.T) {
	watchedAt := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	play := watchsync.LocalPlay{
		HistoryID: "h1",
		Kind:      historyimport.KindMovie,
		TMDBID:    "42",
		WatchedAt: watchedAt,
	}
	body, ok := watchEventBody(play)
	if !ok {
		t.Fatalf("watchEventBody() ok = false, want true")
	}
	if body.TMDBID != 42 || body.MediaType != "movie" || !body.Completed {
		t.Fatalf("unexpected body: %+v", body)
	}
	if body.WatchedAt != "2026-01-02T03:04:05Z" {
		t.Fatalf("WatchedAt = %q", body.WatchedAt)
	}
}

func TestWatchEventBodyEpisodeRequiresSeriesID(t *testing.T) {
	play := watchsync.LocalPlay{
		HistoryID:     "h2",
		Kind:          historyimport.KindEpisode,
		TMDBID:        "42",
		SeasonNumber:  1,
		EpisodeNumber: 2,
	}
	if _, ok := watchEventBody(play); ok {
		t.Fatalf("watchEventBody() ok = true for episode with no series id, want false")
	}

	play.SeriesTMDBID = "7"
	body, ok := watchEventBody(play)
	if !ok {
		t.Fatalf("watchEventBody() ok = false, want true")
	}
	if body.SeriesTMDBID != 7 || body.SeasonNumber == nil || *body.SeasonNumber != 1 || body.EpisodeNumber == nil || *body.EpisodeNumber != 2 {
		t.Fatalf("unexpected body: %+v", body)
	}
}

func TestUnwatchItemPathRequiresExternalID(t *testing.T) {
	if _, ok := unwatchItemPath(historyimport.KindMovie, "", ""); ok {
		t.Fatalf("unwatchItemPath() ok = true with no id, want false")
	}
	path, ok := unwatchItemPath(historyimport.KindEpisode, "42", "")
	if !ok || path != "/history/item?media_type=episode&tmdb_id=42" {
		t.Fatalf("unwatchItemPath() = %q, %v", path, ok)
	}
}

func TestHistoryPlayFromEventSkipsIncompleteOrUnidentified(t *testing.T) {
	if _, ok := historyPlayFromEvent("scrob", scrobHistoryEvent{Media: scrobMedia{Type: "movie", TMDBID: 1}}); ok {
		t.Fatalf("expected no row for an event with no watched_at")
	}
	watchedAt := scrobTimePtr(time.Now())
	if _, ok := historyPlayFromEvent("scrob", scrobHistoryEvent{Media: scrobMedia{Type: "movie"}, WatchedAt: watchedAt}); ok {
		t.Fatalf("expected no row for a movie with no external id")
	}
	row, ok := historyPlayFromEvent("scrob", scrobHistoryEvent{
		Media:     scrobMedia{Type: "episode", TMDBID: 1, ShowTMDBID: 9},
		WatchedAt: watchedAt,
	})
	if ok {
		t.Fatalf("expected no row for an episode missing season/episode numbers, got %+v", row)
	}
}

func TestRequestsGoThroughTheAPIProxyPrefix(t *testing.T) {
	// Regression test: a standard Scrob deployment only publishes the
	// frontend's port. The frontend serves its own pages at the bare paths
	// (e.g. /ratings isn't a route it knows, so unauthenticated requests
	// there 302 to /login) and only forwards to the backend under
	// /api/proxy/*. Hitting a bare backend path here used to silently "work"
	// against a fake server that didn't care about the path, while actually
	// hitting Scrob's login redirect in production.
	var gotPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		_ = json.NewEncoder(w).Encode(scrobRatingsResponse{})
	}))
	defer server.Close()

	p := NewProvider(server.Client())
	conn := watchsync.Connection{
		AccessToken:      "secret",
		SecretAttributes: map[string]string{serverURLFieldKey: server.URL},
	}
	if _, err := p.FetchRatings(context.Background(), watchsync.ServerConfig{}, conn); err != nil {
		t.Fatalf("FetchRatings() error: %v", err)
	}
	if gotPath != "/api/proxy/ratings" {
		t.Fatalf("request path = %q, want /api/proxy/ratings", gotPath)
	}
}

func TestFetchWatchedAggregatesByItem(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Api-Key") != "secret" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		resp := scrobHistoryResponse{
			Page:       1,
			TotalPages: 1,
			Results: []scrobHistoryEvent{
				{Media: scrobMedia{Type: "movie", TMDBID: 1, Title: "A"}, WatchedAt: scrobTimePtr(time.Unix(100, 0))},
				{Media: scrobMedia{Type: "movie", TMDBID: 1, Title: "A"}, WatchedAt: scrobTimePtr(time.Unix(200, 0))},
			},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	p := NewProvider(server.Client())
	conn := watchsync.Connection{
		AccessToken:      "secret",
		SecretAttributes: map[string]string{serverURLFieldKey: server.URL},
	}
	rows, err := p.FetchWatched(context.Background(), watchsync.ServerConfig{}, conn)
	if err != nil {
		t.Fatalf("FetchWatched() error: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("FetchWatched() = %d rows, want 1", len(rows))
	}
	if rows[0].PlayCount != 2 {
		t.Fatalf("PlayCount = %d, want 2", rows[0].PlayCount)
	}
	if rows[0].LastWatchedAt == nil || !rows[0].LastWatchedAt.Equal(time.Unix(200, 0)) {
		t.Fatalf("LastWatchedAt = %v, want %v", rows[0].LastWatchedAt, time.Unix(200, 0))
	}
}

func TestConnectWithAPIKeyConfigRejectsInvalidKey(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()

	p := NewProvider(server.Client())
	config := watchsync.ConnectionConfigValues{
		connectionConfigKey: map[string]any{serverURLFieldKey: server.URL},
	}
	if _, _, err := p.ConnectWithAPIKeyConfig(context.Background(), "bad-key", config); err == nil {
		t.Fatalf("ConnectWithAPIKeyConfig() error = nil, want an error for a rejected key")
	}
}

func scrobTimePtr(t time.Time) *scrobTime {
	st := scrobTime(t)
	return &st
}
