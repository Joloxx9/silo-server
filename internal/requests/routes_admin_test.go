package requests

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/Silo-Server/silo-server/internal/metadata/tmdb"
)

// routeAdminService wires a Service over the lifecycle test schema, with the
// route table copied in and Radarr/Sonarr servers seeded.
func routeAdminService(t *testing.T) (*Service, *Repository) {
	t.Helper()
	repo, pool := lifecycleTestRepository(t)
	ctx := t.Context()
	if _, err := pool.Exec(ctx, `CREATE TABLE request_routes (LIKE public.request_routes INCLUDING ALL)`); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `CREATE TRIGGER route_revision BEFORE INSERT OR UPDATE ON request_routes
		FOR EACH ROW EXECUTE FUNCTION public.advance_request_editor_revision()`); err != nil {
		t.Fatal(err)
	}
	for _, seed := range [][2]string{
		{"radarr", `{"service_kind":"radarr"}`}, {"radarr-anime", `{"service_kind":"radarr"}`}, {"sonarr", `{"service_kind":"sonarr"}`},
	} {
		if _, err := pool.Exec(ctx, `INSERT INTO request_integrations (id, name, enabled, capability_id, plugin_config) VALUES ($1, $1, true, 'arr', $2::jsonb)`, seed[0], seed[1]); err != nil {
			t.Fatal(err)
		}
	}
	return NewService(repo, &fakeTMDBClient{}, &fakePresence{}), repo
}

var routeAdmin = Viewer{UserID: 1, IsAdmin: true}

func fieldErrors(t *testing.T, err error) map[string]string {
	t.Helper()
	var verr *ValidationError
	if !errors.As(err, &verr) {
		t.Fatalf("err = %v, want a ValidationError", err)
	}
	return verr.FieldErrors
}

func TestRouteAdministrationDatabase(t *testing.T) {
	svc, repo := routeAdminService(t)
	ctx := t.Context()

	// Until the fallback has an HD server there is nowhere for titles no rule
	// matches to go, so rules wait for it.
	_, err := svc.CreateRoute(ctx, routeAdmin, Route{
		MediaType: MediaTypeMovie, Name: "Anime", Enabled: true,
		Conditions: RouteConditions{Anime: boolPtr(true)}, HD: RouteDestination{IntegrationID: "radarr-anime"},
	})
	var verr *ValidationError
	if !errors.As(err, &verr) || !strings.Contains(verr.FormError, "default server for movies") {
		t.Fatalf("rule before the fallback: err = %v, want the default server asked for", err)
	}
	unsaved, err := svc.GetRoute(ctx, routeAdmin, FallbackRouteID(MediaTypeMovie))
	if err != nil || !unsaved.IsFallback || unsaved.Revision != 0 {
		t.Fatalf("unsaved fallback = %+v, %v; want a revision-zero fallback", unsaved, err)
	}

	// The fallback is created on its first save and takes no conditions.
	fallback, err := svc.UpdateRouteConditional(ctx, routeAdmin, Route{
		ID: FallbackRouteID(MediaTypeMovie), HD: RouteDestination{IntegrationID: "radarr"},
	}, 0)
	if err != nil {
		t.Fatalf("save fallback: %v", err)
	}
	if !fallback.IsFallback || fallback.MediaType != MediaTypeMovie || fallback.Name != "Everything else" || !fallback.Enabled {
		t.Fatalf("fallback = %+v", fallback)
	}

	anime, err := svc.CreateRoute(ctx, routeAdmin, Route{
		MediaType: MediaTypeMovie, Name: " Anime ", Enabled: true,
		Conditions: RouteConditions{Anime: boolPtr(true), OriginalLanguages: []string{"JA", "ja"}},
		HD:         RouteDestination{IntegrationID: "radarr-anime", Overrides: map[string]any{"root_folder": "/anime"}},
	})
	if err != nil {
		t.Fatalf("create anime rule: %v", err)
	}
	eighties, err := svc.CreateRoute(ctx, routeAdmin, Route{
		MediaType: MediaTypeMovie, Name: "80s", Enabled: true,
		Conditions: RouteConditions{YearFrom: 1980, YearTo: 1989}, SkipUHD: true,
	})
	if err != nil {
		t.Fatalf("create 80s rule: %v", err)
	}
	if anime.Name != "Anime" || !slices.Equal(anime.Conditions.OriginalLanguages, []string{"ja"}) || eighties.Position <= anime.Position {
		t.Fatalf("anime = %+v 80s = %+v, want normalized and appended in order", anime, eighties)
	}

	// A stale editor loses.
	stale := *anime
	stale.Name = "Anime (old tab)"
	if _, err := svc.UpdateRouteConditional(ctx, routeAdmin, stale, anime.Revision-1); !errors.Is(err, ErrStaleRevision) {
		t.Fatalf("stale update: err = %v, want ErrStaleRevision", err)
	}
	renamed := *anime
	renamed.Name = "Anime and donghua"
	updated, err := svc.UpdateRouteConditional(ctx, routeAdmin, renamed, anime.Revision)
	if err != nil || updated.Name != "Anime and donghua" || updated.Position != anime.Position {
		t.Fatalf("update = %+v, %v; want renamed in place", updated, err)
	}

	ordered, err := svc.ReorderRoutes(ctx, routeAdmin, MediaTypeMovie, []string{eighties.ID, anime.ID})
	if err != nil {
		t.Fatalf("reorder: %v", err)
	}
	var names []string
	for _, route := range ordered {
		names = append(names, route.Name)
	}
	if !slices.Equal(names, []string{"80s", "Anime and donghua", "Everything else"}) {
		t.Fatalf("order = %v, want the new order with the fallback last", names)
	}
	if _, err := svc.ReorderRoutes(ctx, routeAdmin, MediaTypeMovie, []string{anime.ID}); err == nil {
		t.Fatal("a reorder missing a rule was accepted")
	}

	if err := svc.DeleteRouteConditional(ctx, routeAdmin, fallback.ID, fallback.Revision); err == nil {
		t.Fatal("deleting the fallback was accepted")
	}
	// Reordering is an edit: it moves the rules to new revisions.
	if err := svc.DeleteRouteConditional(ctx, routeAdmin, eighties.ID, eighties.Revision); !errors.Is(err, ErrStaleRevision) {
		t.Fatalf("delete with the pre-reorder revision: err = %v, want ErrStaleRevision", err)
	}
	if err := svc.DeleteRouteConditional(ctx, routeAdmin, eighties.ID, ordered[0].Revision); err != nil {
		t.Fatalf("delete rule: %v", err)
	}
	if _, err := repo.GetRoute(ctx, eighties.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("deleted rule: err = %v, want ErrNotFound", err)
	}
}

func TestRouteValidationDatabase(t *testing.T) {
	svc, _ := routeAdminService(t)
	ctx := t.Context()
	for _, tc := range []struct {
		name  string
		route Route
		field string
	}{
		{"no name", Route{MediaType: MediaTypeMovie, Conditions: RouteConditions{Anime: boolPtr(true)}, HD: RouteDestination{IntegrationID: "radarr"}}, "name"},
		{"no condition", Route{MediaType: MediaTypeMovie, Name: "All", HD: RouteDestination{IntegrationID: "radarr"}}, "conditions"},
		{"no effect", Route{MediaType: MediaTypeMovie, Name: "Nothing", Conditions: RouteConditions{Anime: boolPtr(true)}}, "hd"},
		{"wrong kind", Route{MediaType: MediaTypeMovie, Name: "Wrong", Conditions: RouteConditions{Anime: boolPtr(true)}, HD: RouteDestination{IntegrationID: "sonarr"}}, "hd.integration_id"},
		{"unknown server", Route{MediaType: MediaTypeMovie, Name: "Gone", Conditions: RouteConditions{Anime: boolPtr(true)}, HD: RouteDestination{IntegrationID: "nope"}}, "hd.integration_id"},
		{"routing-owned key", Route{MediaType: MediaTypeMovie, Name: "Sneaky", Conditions: RouteConditions{Anime: boolPtr(true)},
			HD: RouteDestination{IntegrationID: "radarr", Overrides: map[string]any{"is_default": true}}}, "hd.overrides.is_default"},
		{"backwards years", Route{MediaType: MediaTypeMovie, Name: "Years", Conditions: RouteConditions{YearFrom: 1990, YearTo: 1980}, HD: RouteDestination{IntegrationID: "radarr"}}, "conditions.year_to"},
		{"bad language", Route{MediaType: MediaTypeMovie, Name: "Lang", Conditions: RouteConditions{OriginalLanguages: []string{"japanese"}}, HD: RouteDestination{IntegrationID: "radarr"}}, "conditions.original_languages"},
		{"skip and send 4K", Route{MediaType: MediaTypeMovie, Name: "Both", Conditions: RouteConditions{Anime: boolPtr(true)}, SkipUHD: true, UHD: RouteDestination{IntegrationID: "radarr"}}, "uhd"},
	} {
		_, err := svc.CreateRoute(ctx, routeAdmin, tc.route)
		if fields := fieldErrors(t, err); fields[tc.field] == "" {
			t.Errorf("%s: field errors = %v, want %s", tc.name, fields, tc.field)
		}
	}
	_, err := svc.UpdateRouteConditional(ctx, routeAdmin, Route{
		ID: FallbackRouteID(MediaTypeSeries), Conditions: RouteConditions{Anime: boolPtr(true)}, HD: RouteDestination{IntegrationID: "sonarr"},
	}, 0)
	if fields := fieldErrors(t, err); fields["conditions"] == "" {
		t.Fatalf("fallback with conditions: field errors = %v", fields)
	}
}

func TestRouteAdministrationRequiresAdmin(t *testing.T) {
	svc := newTestService(newFakeStore())
	member := Viewer{UserID: 2}
	if _, err := svc.ListRoutesAdmin(context.Background(), member); !errors.Is(err, ErrForbidden) {
		t.Fatalf("list: err = %v", err)
	}
	if _, err := svc.CreateRoute(context.Background(), member, Route{}); !errors.Is(err, ErrForbidden) {
		t.Fatalf("create: err = %v", err)
	}
	if _, err := svc.PreviewRoute(context.Background(), member, MediaTypeMovie, 1, 0); !errors.Is(err, ErrForbidden) {
		t.Fatalf("preview: err = %v", err)
	}
}

func TestPreviewRoute(t *testing.T) {
	store := routingStore(RoutingFacts{})
	svc := newTestServiceWithTMDB(store, &fakeTMDBClient{detail: &tmdb.MediaDetail{ID: 129, KeywordIDs: []int{210024}}})

	preview, err := svc.PreviewRoute(context.Background(), routeAdmin, MediaTypeMovie, 129, 7)
	if err != nil {
		t.Fatalf("preview: %v", err)
	}
	if !preview.Facts.Anime || len(preview.Tiers) != 2 {
		t.Fatalf("preview = %+v", preview)
	}
	hd, uhd := preview.Tiers[0], preview.Tiers[1]
	if hd.RouteName != "Anime" || hd.IntegrationName != "radarr-anime" || uhd.RouteName != "Everything else" {
		t.Fatalf("tiers = %+v", preview.Tiers)
	}
}

func TestRouteAdministrationGuardsDatabase(t *testing.T) {
	svc, repo := routeAdminService(t)
	ctx := t.Context()

	// A fallback without an HD server would fail every unmatched title.
	_, err := svc.UpdateRouteConditional(ctx, routeAdmin, Route{ID: FallbackRouteID(MediaTypeMovie), UHD: RouteDestination{IntegrationID: "radarr"}}, 0)
	if fields := fieldErrors(t, err); fields["hd.integration_id"] == "" {
		t.Fatalf("fallback without HD: field errors = %v", fields)
	}

	// Two first saves of the fallback: the second editor read revision zero
	// too, and must lose instead of overwriting.
	first, err := repo.SaveRouteConditional(ctx, Route{ID: FallbackRouteID(MediaTypeMovie), MediaType: MediaTypeMovie, Position: 1000,
		Name: "Everything else", Enabled: true, IsFallback: true, HD: RouteDestination{IntegrationID: "radarr"}}, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repo.SaveRouteConditional(ctx, Route{ID: first.ID, MediaType: MediaTypeMovie, Position: 1000,
		Name: "Other editor", Enabled: true, IsFallback: true, HD: RouteDestination{IntegrationID: "radarr-anime"}}, 0); !errors.Is(err, ErrStaleRevision) {
		t.Fatalf("second first save: err = %v, want ErrStaleRevision", err)
	}

	// A rule deleted under an editor does not come back on save.
	rule, err := svc.CreateRoute(ctx, routeAdmin, Route{MediaType: MediaTypeMovie, Name: "アニメとドンファのための特別なルール、とても長い名前でも大丈夫", Enabled: true,
		Conditions: RouteConditions{Anime: boolPtr(true)}, HD: RouteDestination{IntegrationID: "radarr-anime"}})
	if err != nil {
		t.Fatalf("create a rule with a long non-Latin name: %v", err)
	}
	if err := svc.DeleteRouteConditional(ctx, routeAdmin, rule.ID, rule.Revision); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.SaveRouteConditional(ctx, *rule, rule.Revision); !errors.Is(err, ErrStaleRevision) && !errors.Is(err, ErrNotFound) {
		t.Fatalf("save a deleted rule: err = %v, want it refused", err)
	}
	if _, err := repo.GetRoute(ctx, rule.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("deleted rule came back: %v", err)
	}
}

func TestPreviewRouteTellsMissingFromUnreachable(t *testing.T) {
	store := routingStore(RoutingFacts{})
	missing := newTestServiceWithTMDB(store, &fakeTMDBClient{detailErr: fmt.Errorf("lookup: %w", tmdb.ErrNotFound)})
	if _, err := missing.PreviewRoute(context.Background(), routeAdmin, MediaTypeMovie, 1, 0); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing title: err = %v, want ErrNotFound", err)
	}
	down := newTestServiceWithTMDB(store, &fakeTMDBClient{detailErr: errors.New("tmdb: server error 503 after 3 retries")})
	if _, err := down.PreviewRoute(context.Background(), routeAdmin, MediaTypeMovie, 1, 0); !errors.Is(err, ErrIntegrationUnreachable) {
		t.Fatalf("TMDB down: err = %v, want ErrIntegrationUnreachable", err)
	}
}

// Switching a server to the other type would fail every request its routes
// send it, so the switch is refused while a route uses the server, and the
// preview flags a server already switched.
func TestServerTypeSwitchKeepsRoutesWorking(t *testing.T) {
	store := routingStore(RoutingFacts{})
	svc := newTestServiceWithTMDB(store, &fakeTMDBClient{detail: &tmdb.MediaDetail{ID: 129, KeywordIDs: []int{210024}}})

	switched := store.integrations[2] // radarr-anime, which the movie rule "Anime" sends to
	switched.PluginConfig = map[string]any{"service_kind": "sonarr", "root_folder": "/tv"}
	_, err := svc.UpdateIntegration(context.Background(), routeAdmin, switched)
	var verr *ValidationError
	if !errors.As(err, &verr) || !strings.Contains(verr.FieldErrors["plugin_config.service_kind"], "Anime") {
		t.Fatalf("switch: err = %v, want a service_kind field error naming the route", err)
	}

	store.integrations[2] = switched
	preview, err := svc.PreviewRoute(context.Background(), routeAdmin, MediaTypeMovie, 129, 7)
	if err != nil {
		t.Fatal(err)
	}
	if hd := preview.Tiers[0]; !strings.Contains(hd.Reason, "a sonarr server") {
		t.Fatalf("hd tier = %+v, want the type mismatch noted", hd)
	}
}

// The preview flags every server problem a routed submission would fail on,
// so an unusable route never reads as working.
func TestPreviewRouteFlagsUnusableServers(t *testing.T) {
	for _, tc := range []struct {
		name     string
		unusable func(*Integration)
		want     string
	}{
		{"not bound", func(in *Integration) { in.InstallationID = nil }, "not bound to a plugin installation"},
		{"no key", func(in *Integration) { in.APIKeyRef = "" }, "has no API key"},
		{"does not take movies", func(in *Integration) { in.SupportedMediaTypes = []string{"series"} }, "does not take movies"},
		{"gone", func(in *Integration) { in.ID = "deleted" }, "no longer exists"},
	} {
		store := routingStore(RoutingFacts{})
		tc.unusable(&store.integrations[0]) // radarr-hd, the fallback's HD server
		svc := newTestServiceWithTMDB(store, &fakeTMDBClient{detail: &tmdb.MediaDetail{ID: 129}})
		preview, err := svc.PreviewRoute(context.Background(), routeAdmin, MediaTypeMovie, 129, 7)
		if err != nil {
			t.Fatalf("%s: preview: %v", tc.name, err)
		}
		if hd := preview.Tiers[0]; !strings.Contains(hd.Reason, tc.want) || !strings.HasSuffix(hd.Reason, "would fail.") {
			t.Errorf("%s: HD tier = %+v, want a failure note containing %q", tc.name, hd, tc.want)
		}
		if uhd := preview.Tiers[1]; uhd.Reason != "" {
			t.Errorf("%s: 4K tier = %+v, want no failure note for the working 4K server", tc.name, uhd)
		}
	}
}

// A route cannot send a media type to a server that does not take it, and a
// server a route uses cannot stop taking the route's media type.
func TestRoutesRespectSupportedMediaTypes(t *testing.T) {
	store := routingStore(RoutingFacts{})
	seriesOnly := routerInst("generic-series")
	seriesOnly.SupportedMediaTypes = []string{"series"}
	store.integrations = append(store.integrations, seriesOnly)
	svc := newTestServiceWithTMDB(store, &fakeTMDBClient{})

	route := Route{MediaType: MediaTypeMovie, Name: "Anime", Enabled: true,
		Conditions: RouteConditions{Anime: boolPtr(true)}, HD: RouteDestination{IntegrationID: "generic-series"}}
	if fields := fieldErrors(t, svc.validateRoute(context.Background(), &route)); !strings.Contains(fields["hd.integration_id"], "does not take movies") {
		t.Fatalf("field errors = %v, want hd.integration_id refused", fields)
	}

	narrowed := store.integrations[2] // radarr-anime, which the movie rule "Anime" sends to
	narrowed.SupportedMediaTypes = []string{"series"}
	_, err := svc.UpdateIntegration(context.Background(), routeAdmin, narrowed)
	var verr *ValidationError
	if !errors.As(err, &verr) || !strings.Contains(verr.FieldErrors["supported_media_types"], "Anime") {
		t.Fatalf("narrow: err = %v, want a supported_media_types field error naming the route", err)
	}
}
