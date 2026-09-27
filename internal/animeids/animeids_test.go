package animeids

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// list builds a published list with n series and one film split in parts.
func list(n int) []byte {
	raw := map[string]map[string]any{}
	for i := 1; i <= n; i++ {
		raw[fmt.Sprint(i)] = map[string]any{"tvdb_id": 1000 + i, "mal_id": i}
	}
	raw["film"] = map[string]any{"imdb_id": "tt0000001, tt0000002"}
	raw["unmapped"] = map[string]any{"mal_id": 9}
	raw["null"] = map[string]any{"tvdb_id": nil}
	body, _ := json.Marshal(raw)
	return body
}

func TestParse(t *testing.T) {
	ids, err := parse(list(minEntries))
	if err != nil {
		t.Fatal(err)
	}
	got := map[listedID]bool{}
	for _, id := range ids {
		got[id] = true
	}
	if len(ids) != minEntries+2 || !got[listedID{"tvdb", "1001"}] || !got[listedID{"imdb", "tt0000002"}] {
		t.Fatalf("parsed %d IDs, want every series by TVDB ID and each part of the film by IMDb ID", len(ids))
	}
	if _, err := parse(list(10)); err == nil || !strings.Contains(err.Error(), "only") {
		t.Fatalf("a short list: %v, want it refused", err)
	}
	if _, err := parse([]byte("<html>rate limited</html>")); err == nil {
		t.Fatal("parsed a page that is not the list")
	}
}

func testPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("SILO_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("SILO_TEST_DATABASE_URL is not set")
	}
	admin, err := pgxpool.New(t.Context(), dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(admin.Close)
	schema := fmt.Sprintf("anime_ids_%d", time.Now().UnixNano())
	quoted := pgx.Identifier{schema}.Sanitize()
	if _, err := admin.Exec(t.Context(), `CREATE SCHEMA `+quoted); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = admin.Exec(context.Background(), `DROP SCHEMA `+quoted+` CASCADE`) })
	config, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	config.ConnConfig.RuntimeParams["search_path"] = schema + ",public"
	pool, err := pgxpool.NewWithConfig(t.Context(), config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	for _, table := range []string{"anime_ids", "anime_ids_refresh"} {
		if _, err := pool.Exec(t.Context(), `CREATE TABLE `+table+` (LIKE public.`+table+` INCLUDING ALL)`); err != nil {
			t.Fatal(err)
		}
	}
	return pool
}

func TestRefreshDatabase(t *testing.T) {
	pool := testPool(t)
	ctx := t.Context()
	body := list(minEntries)
	var serve atomic.Value
	serve.Store("ok")
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		switch serve.Load() {
		case "down":
			http.Error(w, "down", http.StatusBadGateway)
		case "broken":
			_, _ = w.Write(list(5))
		default:
			if r.Header.Get("If-None-Match") == `"v1"` {
				w.WriteHeader(http.StatusNotModified)
				return
			}
			w.Header().Set("ETag", `"v1"`)
			_, _ = w.Write(body)
		}
	}))
	defer server.Close()
	refresher := &Refresher{pool: pool, client: server.Client(), url: server.URL}
	store := NewStore(pool)

	got, err := refresher.Refresh(ctx)
	if err != nil || got.Entries != minEntries+2 {
		t.Fatalf("first refresh = %+v, %v", got, err)
	}
	for _, tc := range []struct {
		movie bool
		tvdb  int
		imdb  string
		want  bool
	}{
		{false, 1001, "", true},
		{true, 1001, "", false}, // a movie's TVDB ID is another numbering
		{true, 0, "tt0000002", true},
		{false, 0, "tt0000002", true},
		{false, 42, "tt9999999", false},
		{false, 0, "", false},
	} {
		if listed, err := store.Listed(ctx, tc.movie, tc.tvdb, tc.imdb); err != nil || listed != tc.want {
			t.Fatalf("Listed(%v, %d, %q) = %v, %v; want %v", tc.movie, tc.tvdb, tc.imdb, listed, err, tc.want)
		}
	}

	// The same list again is not downloaded again.
	if got, err := refresher.Refresh(ctx); err != nil || !got.Unchanged {
		t.Fatalf("unchanged refresh = %+v, %v", got, err)
	}

	// A failed or broken download keeps the stored list and says why.
	for _, mode := range []string{"down", "broken"} {
		serve.Store(mode)
		if _, err := pool.Exec(ctx, `UPDATE anime_ids_refresh SET etag = ''`); err != nil {
			t.Fatal(err)
		}
		if _, err := refresher.Refresh(ctx); err == nil {
			t.Fatalf("%s: refresh succeeded", mode)
		}
		var status, message string
		var count int
		_ = pool.QueryRow(ctx, `SELECT last_status, last_error FROM anime_ids_refresh`).Scan(&status, &message)
		_ = pool.QueryRow(ctx, `SELECT count(*) FROM anime_ids`).Scan(&count)
		if status != "error" || message == "" || count != minEntries+2 {
			t.Fatalf("%s: status %q %q, %d IDs; want the error recorded and the list kept", mode, status, message, count)
		}
	}

	// A list that lost its rows is downloaded again, not answered 304.
	serve.Store("ok")
	if _, err := pool.Exec(ctx, `UPDATE anime_ids_refresh SET etag = '"v1"'; DELETE FROM anime_ids`); err != nil {
		t.Fatal(err)
	}
	if got, err := refresher.Refresh(ctx); err != nil || got.Unchanged || got.Entries != minEntries+2 {
		t.Fatalf("refresh of an emptied list = %+v, %v; want it downloaded", got, err)
	}

	// A server whose lease ran out and was taken over drops its result.
	c, claimed, err := refresher.claim(ctx)
	if err != nil || !claimed {
		t.Fatal(claimed, err)
	}
	if _, err := pool.Exec(ctx, `UPDATE anime_ids_refresh SET last_attempt_at = clock_timestamp()`); err != nil {
		t.Fatal(err)
	}
	if err := refresher.replace(ctx, c, []listedID{{"tvdb", "1"}}, "stale"); !errors.Is(err, errClaimLost) {
		t.Fatalf("replace after a takeover: %v, want errClaimLost", err)
	}
	if _, err := pool.Exec(ctx, `UPDATE anime_ids_refresh SET last_status = 'ok'`); err != nil {
		t.Fatal(err)
	}

	// A refresh another server holds keeps this one out until its lease ends.
	serve.Store("ok")
	if _, err := pool.Exec(ctx, `UPDATE anime_ids_refresh SET last_status = 'refreshing', last_attempt_at = now()`); err != nil {
		t.Fatal(err)
	}
	before := requests.Load()
	if got, err := refresher.Refresh(ctx); err != nil || !got.Skipped || requests.Load() != before {
		t.Fatalf("held refresh = %+v, %v; want it skipped without a download", got, err)
	}
	if _, err := pool.Exec(ctx, `UPDATE anime_ids_refresh SET last_attempt_at = now() - interval '11 minutes'`); err != nil {
		t.Fatal(err)
	}
	if got, err := refresher.Refresh(ctx); err != nil || got.Skipped {
		t.Fatalf("expired claim = %+v, %v; want it taken over", got, err)
	}
}

func TestRedirectsStayOnTheListHost(t *testing.T) {
	client := newClient()
	req, _ := http.NewRequest(http.MethodGet, "https://example.com/x", nil)
	if err := client.CheckRedirect(req, []*http.Request{{}}); err == nil {
		t.Fatal("followed a redirect off the list host")
	}
	req, _ = http.NewRequest(http.MethodGet, "https://"+sourceHost+"/x", nil)
	if err := client.CheckRedirect(req, []*http.Request{{}}); err != nil {
		t.Fatal(err)
	}
}
