package requests

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"

	"github.com/Silo-Server/silo-server/internal/idgen"
	"github.com/Silo-Server/silo-server/internal/metadata/tmdb"
)

// Route administration. Every media type has one fallback route, with the
// fixed ID FallbackRouteID(mediaType): saving it creates it, it takes no
// conditions, and it cannot be deleted. The other routes are ordered rules
// that must narrow (at least one condition) and must do something (a
// destination, or skip 4K).

// maxRoutesPerMediaType bounds the rules one media type can have, keeping the
// route list a small bounded collection.
const maxRoutesPerMediaType = 100

// FallbackRouteID is the ID of a media type's fallback route.
func FallbackRouteID(mediaType MediaType) string { return "fallback-" + string(mediaType) }

// routingOwnedConfigKeys are the plugin config keys routing sets itself; a
// route cannot override them.
var routingOwnedConfigKeys = []string{
	configServiceKind, configIsDefault, configIsDefault4K, configIs4K,
	configAnimeEnabled, "anime_root_folder", "anime_quality_profile_id", "anime_tags",
}

var (
	languageCode = regexp.MustCompile(`^[a-z]{2,3}$`)
	countryCode  = regexp.MustCompile(`^[A-Z]{2}$`)
)

// RoutePreview is how the routes would send one title right now.
type RoutePreview struct {
	Facts RoutingFacts
	Tiers []RoutePreviewTier
}

// RoutePreviewTier is one quality tier's outcome. RouteID is empty when no
// route sends the tier anywhere; Reason then says why.
type RoutePreviewTier struct {
	Quality         Quality
	RouteID         string
	RouteName       string
	IntegrationID   string
	IntegrationName string
	Overrides       map[string]any
	Reason          string
}

func (s *Service) routeStore() (RouteStore, error) {
	store, ok := s.store.(RouteStore)
	if !ok {
		return nil, fmt.Errorf("request store does not support route administration")
	}
	return store, nil
}

// RouteStore is implemented by the PostgreSQL repository.
type RouteStore interface {
	GetRoute(ctx context.Context, id string) (*Route, error)
	SaveRouteConditional(ctx context.Context, route Route, expected int64) (*Route, error)
	DeleteRouteConditional(ctx context.Context, id string, expected int64) error
	ReorderRoutes(ctx context.Context, mediaType MediaType, ids []string) error
}

// unsavedFallback is a media type's fallback before its first save: revision
// zero, no destinations. Administration shows it so it can be edited; routing
// never sees it.
func unsavedFallback(mediaType MediaType) Route {
	return Route{ID: FallbackRouteID(mediaType), MediaType: mediaType, Position: 1000, Name: fallbackRouteName, Enabled: true, IsFallback: true}
}

// ListRoutesAdmin returns every route, in evaluation order per media type,
// with each media type's fallback (unsaved if it never was).
func (s *Service) ListRoutesAdmin(ctx context.Context, viewer Viewer) ([]Route, error) {
	if !viewer.IsAdmin {
		return nil, ErrForbidden
	}
	routes, err := s.store.ListRoutes(ctx)
	if err != nil {
		return nil, err
	}
	var out []Route
	for _, mediaType := range []MediaType{MediaTypeMovie, MediaTypeSeries} {
		var ofType []Route
		hasFallback := false
		for _, route := range routes {
			if route.MediaType == mediaType {
				ofType = append(ofType, route)
				hasFallback = hasFallback || route.IsFallback
			}
		}
		if !hasFallback {
			ofType = append(ofType, unsavedFallback(mediaType))
		}
		out = append(out, orderRoutes(ofType)...)
	}
	return out, nil
}

// GetRoute returns one route; a fallback that was never saved comes back
// unsaved (revision zero).
func (s *Service) GetRoute(ctx context.Context, viewer Viewer, id string) (*Route, error) {
	if !viewer.IsAdmin {
		return nil, ErrForbidden
	}
	store, err := s.routeStore()
	if err != nil {
		return nil, err
	}
	id = strings.TrimSpace(id)
	route, err := store.GetRoute(ctx, id)
	if errors.Is(err, ErrNotFound) {
		for _, mediaType := range []MediaType{MediaTypeMovie, MediaTypeSeries} {
			if id == FallbackRouteID(mediaType) {
				fallback := unsavedFallback(mediaType)
				return &fallback, nil
			}
		}
	}
	return route, err
}

// CreateRoute adds a rule after the media type's existing rules.
func (s *Service) CreateRoute(ctx context.Context, viewer Viewer, route Route) (*Route, error) {
	if !viewer.IsAdmin {
		return nil, ErrForbidden
	}
	store, err := s.routeStore()
	if err != nil {
		return nil, err
	}
	id, err := idgen.NextID()
	if err != nil {
		return nil, err
	}
	route.ID = id
	route.IsFallback = false
	if err := s.validateRoute(ctx, &route); err != nil {
		return nil, err
	}
	routes, err := s.store.ListRoutes(ctx)
	if err != nil {
		return nil, err
	}
	// The first rule switches the media type from the plugin's routing to
	// Silo's, where a title no rule matches goes to the fallback. Without a
	// fallback HD server those titles would have nowhere to go.
	fallbackReady := false
	rules := 0
	for _, existing := range routes {
		if existing.MediaType != route.MediaType {
			continue
		}
		if existing.IsFallback {
			fallbackReady = existing.HD.IntegrationID != ""
			continue
		}
		rules++
		if existing.Position >= route.Position {
			route.Position = existing.Position + 1
		}
	}
	if rules >= maxRoutesPerMediaType {
		return nil, &ValidationError{FormError: fmt.Sprintf("A media type can have at most %d rules.", maxRoutesPerMediaType)}
	}
	if !fallbackReady {
		return nil, &ValidationError{FormError: "Choose the default server for " + mediaTypePlural(route.MediaType) + " before adding rules; titles no rule matches go there."}
	}
	return store.SaveRouteConditional(ctx, route, 0)
}

// UpdateRouteConditional replaces a route. The fallback route is created on
// its first save (expected 0). Position is kept; ReorderRoutes changes it.
func (s *Service) UpdateRouteConditional(ctx context.Context, viewer Viewer, route Route, expected int64) (*Route, error) {
	if !viewer.IsAdmin {
		return nil, ErrForbidden
	}
	store, err := s.routeStore()
	if err != nil {
		return nil, err
	}
	route.IsFallback = route.ID == FallbackRouteID(MediaTypeMovie) || route.ID == FallbackRouteID(MediaTypeSeries)
	current, err := store.GetRoute(ctx, route.ID)
	switch {
	case errors.Is(err, ErrNotFound) && route.IsFallback:
		route.MediaType = MediaType(strings.TrimPrefix(route.ID, "fallback-"))
		route.Position = 1000
	case err != nil:
		return nil, err
	default:
		route.MediaType = current.MediaType
		route.Position = current.Position
	}
	if err := s.validateRoute(ctx, &route); err != nil {
		return nil, err
	}
	return store.SaveRouteConditional(ctx, route, expected)
}

func (s *Service) DeleteRouteConditional(ctx context.Context, viewer Viewer, id string, expected int64) error {
	if !viewer.IsAdmin {
		return ErrForbidden
	}
	store, err := s.routeStore()
	if err != nil {
		return err
	}
	current, err := store.GetRoute(ctx, strings.TrimSpace(id))
	if err != nil {
		return err
	}
	if current.IsFallback {
		return &ValidationError{FormError: "The default destination cannot be deleted; change its servers instead."}
	}
	return store.DeleteRouteConditional(ctx, current.ID, expected)
}

// ReorderRoutes sets the evaluation order of a media type's rules and returns
// its routes in the new order. ids must list every rule of the media type
// exactly once; the fallback always stays last and is not listed.
func (s *Service) ReorderRoutes(ctx context.Context, viewer Viewer, mediaType MediaType, ids []string) ([]Route, error) {
	if !viewer.IsAdmin {
		return nil, ErrForbidden
	}
	store, err := s.routeStore()
	if err != nil {
		return nil, err
	}
	mediaType, err = normalizeMediaType(mediaType)
	if err != nil {
		return nil, err
	}
	routes, err := s.store.ListRoutes(ctx)
	if err != nil {
		return nil, err
	}
	var want []string
	for _, route := range routes {
		if route.MediaType == mediaType && !route.IsFallback {
			want = append(want, route.ID)
		}
	}
	got := slices.Clone(ids)
	slices.Sort(want)
	slices.Sort(got)
	if !slices.Equal(want, got) {
		return nil, &ValidationError{FormError: "The order must list every rule for this media type exactly once; reload and try again."}
	}
	if err := store.ReorderRoutes(ctx, mediaType, ids); err != nil {
		return nil, err
	}
	all, err := s.ListRoutesAdmin(ctx, viewer)
	if err != nil {
		return nil, err
	}
	return slices.DeleteFunc(all, func(r Route) bool { return r.MediaType != mediaType }), nil
}

// PreviewRoute shows how the routes would send a title for a requester now,
// from TMDB's current facts. Both tiers are shown regardless of the
// requester's 4K entitlement.
func (s *Service) PreviewRoute(ctx context.Context, viewer Viewer, mediaType MediaType, tmdbID, requesterUserID int) (*RoutePreview, error) {
	if !viewer.IsAdmin {
		return nil, ErrForbidden
	}
	mediaType, err := normalizeMediaType(mediaType)
	if err != nil {
		return nil, err
	}
	if tmdbID <= 0 {
		return nil, fmt.Errorf("%w: tmdb id is required", ErrInvalidInput)
	}
	if s.tmdb == nil {
		return nil, ErrIntegrationUnreachable
	}
	detail, err := s.tmdb.GetMediaDetail(ctx, tmdbMediaType(mediaType), tmdbID)
	switch {
	case errors.Is(err, tmdb.ErrNotFound) || (err == nil && detail == nil):
		return nil, ErrNotFound
	case err != nil:
		// TMDB is down or refusing: a dependency failure, worth retrying.
		return nil, fmt.Errorf("%w: %w", ErrIntegrationUnreachable, err)
	}
	fc, err := s.newFulfillContext(ctx)
	if err != nil {
		return nil, err
	}
	req := Request{MediaType: mediaType, TMDBID: tmdbID, RequestedByUserID: requesterUserID, RoutingFacts: routingFactsFrom(detail, s.now())}
	routes := fc.routesFor(mediaType)
	qualities := []Quality{Quality1080p, Quality2160p}
	decisions := decideRoutes(routes, req, qualities)
	preview := &RoutePreview{Facts: req.RoutingFacts}
	for _, q := range qualities {
		tier := RoutePreviewTier{Quality: q}
		decision, ok := decisions[q]
		switch {
		case len(routes) == 0:
			tier.Reason = "No routes are set up for this media type, so the request plugin picks the server."
		case !ok:
			tier.Reason = "No rule sends " + qualityLabel(q) + " for this title."
		case decision.Skip:
			tier.RouteID, tier.RouteName = decision.RouteID, decision.RouteName
			tier.Reason = decision.RouteName + " skips 4K for this title."
		default:
			tier.RouteID, tier.RouteName = decision.RouteID, decision.RouteName
			tier.IntegrationID, tier.Overrides = decision.IntegrationID, decision.Overrides
			in := integrationByID(fc, decision.IntegrationID)
			if in != nil {
				tier.IntegrationName = in.Name
			}
			tier.Reason = routedServerProblem(in, mediaType)
		}
		preview.Tiers = append(preview.Tiers, tier)
	}
	return preview, nil
}

// routedServerProblem explains why a tier routed to the server would fail
// when sent, in the cases routedConnection refuses it, and is "" when the
// server can take it.
func routedServerProblem(in *Integration, mediaType MediaType) string {
	const fails = ", so this tier would fail."
	switch {
	case in == nil:
		return "The server this rule sends to no longer exists" + fails
	case !in.Enabled:
		return in.Name + " is disabled" + fails
	case in.InstallationID == nil || in.CapabilityID == "":
		return in.Name + " is not bound to a plugin installation (re-save it)" + fails
	case strings.TrimSpace(in.APIKeyRef) == "":
		return in.Name + " has no API key" + fails
	case !integrationSupportsMediaType(*in, mediaType):
		return in.Name + " does not take " + mediaTypePlural(mediaType) + fails
	}
	if kind := serverKindMismatch(*in, mediaType); kind != "" {
		return fmt.Sprintf("%s is a %s server%s", in.Name, kind, fails)
	}
	return ""
}

// serverKindMismatch returns a server's type when it cannot take the media
// type (a Sonarr server for movies, a Radarr server for series), and "" when
// it can or does not say.
func serverKindMismatch(in Integration, mediaType MediaType) string {
	kind, _ := in.PluginConfig[configServiceKind].(string)
	want := map[MediaType]string{MediaTypeMovie: kindRadarr, MediaTypeSeries: kindSonarr}[mediaType]
	if kind == "" || want == "" || kind == want {
		return ""
	}
	return kind
}

// ensureRoutesKeepServerKind refuses to switch a server to a type, or to media
// types, the routes sending to it cannot use: their requests would fail when
// sent.
func (s *Service) ensureRoutesKeepServerKind(ctx context.Context, in Integration) error {
	routes, err := s.store.ListRoutes(ctx)
	if err != nil {
		return err
	}
	var wrongKind, unsupported []string
	for _, r := range routes {
		if r.HD.IntegrationID != in.ID && r.UHD.IntegrationID != in.ID {
			continue
		}
		if serverKindMismatch(in, r.MediaType) != "" {
			wrongKind = append(wrongKind, r.Name)
		}
		if !integrationSupportsMediaType(in, r.MediaType) {
			unsupported = append(unsupported, r.Name)
		}
	}
	fields := map[string]string{}
	if len(wrongKind) > 0 {
		fields["plugin_config."+configServiceKind] = "Routing sends requests of the other media type to this server (" +
			strings.Join(wrongKind, ", ") + "); change those routes first."
	}
	if len(unsupported) > 0 {
		fields["supported_media_types"] = "Routing sends a media type this server would no longer take to it (" +
			strings.Join(unsupported, ", ") + "); change those routes first."
	}
	if len(fields) == 0 {
		return nil
	}
	return &ValidationError{FieldErrors: fields}
}

// validateRoute normalizes a route and checks it against the configured
// servers, answering a ValidationError keyed by field.
func (s *Service) validateRoute(ctx context.Context, route *Route) error {
	fields := map[string]string{}
	route.Name = strings.TrimSpace(route.Name)
	if route.IsFallback && route.Name == "" {
		route.Name = fallbackRouteName
	}
	switch {
	case route.Name == "":
		fields["name"] = "Give the rule a name."
	case utf8.RuneCountInString(route.Name) > 100:
		fields["name"] = "Keep the name under 100 characters."
	}
	mediaType, err := normalizeMediaType(route.MediaType)
	if err != nil {
		fields["media_type"] = "Choose movies or series."
	}
	route.MediaType = mediaType
	route.Conditions = normalizeConditions(route.Conditions)
	validateConditions(route.Conditions, fields)

	integrations, err := s.store.ListIntegrations(ctx)
	if err != nil {
		return err
	}
	for field, dest := range map[string]*RouteDestination{"hd": &route.HD, "uhd": &route.UHD} {
		validateDestination(field, dest, route.MediaType, integrations, fields)
	}
	if route.IsFallback {
		route.Enabled = true
		route.SkipUHD = false
		if !conditionsEmpty(route.Conditions) {
			fields["conditions"] = "The default destination applies to everything; it takes no conditions."
		}
		// A saved fallback moves the media type to Silo's routing; without an
		// HD server, every title no rule matches would fail.
		if route.HD.IntegrationID == "" && fields["hd.integration_id"] == "" {
			fields["hd.integration_id"] = "Choose the server titles no rule matches go to."
		}
	} else {
		if conditionsEmpty(route.Conditions) {
			fields["conditions"] = "Add at least one condition; the default destination handles everything else."
		}
		if route.HD.IntegrationID == "" && route.UHD.IntegrationID == "" && !route.SkipUHD {
			fields["hd"] = "Choose a server for HD or 4K, or skip 4K."
		}
		if route.SkipUHD && route.UHD.IntegrationID != "" {
			fields["uhd"] = "A rule that skips 4K cannot also send 4K somewhere."
		}
	}
	if len(fields) > 0 {
		return &ValidationError{FieldErrors: fields}
	}
	return nil
}

func validateDestination(field string, dest *RouteDestination, mediaType MediaType, integrations []Integration, fields map[string]string) {
	dest.IntegrationID = strings.TrimSpace(dest.IntegrationID)
	if dest.IntegrationID == "" {
		dest.Overrides = nil
		return
	}
	var in *Integration
	for i := range integrations {
		if integrations[i].ID == dest.IntegrationID {
			in = &integrations[i]
		}
	}
	if in == nil {
		fields[field+".integration_id"] = "That server no longer exists."
		return
	}
	if kind, _ := in.PluginConfig[configServiceKind].(string); kind != "" {
		wantKind := map[MediaType]string{MediaTypeMovie: kindRadarr, MediaTypeSeries: kindSonarr}[mediaType]
		if wantKind != "" && kind != wantKind {
			fields[field+".integration_id"] = fmt.Sprintf("%s is a %s server; %s go to %s.", in.Name, kind, mediaTypePlural(mediaType), wantKind)
		}
	}
	if mediaType != "" && fields[field+".integration_id"] == "" && !integrationSupportsMediaType(*in, mediaType) {
		fields[field+".integration_id"] = fmt.Sprintf("%s does not take %s.", in.Name, mediaTypePlural(mediaType))
	}
	for _, key := range routingOwnedConfigKeys {
		if _, ok := dest.Overrides[key]; ok {
			fields[field+".overrides."+key] = "Routing sets " + key + " itself."
		}
	}
	if len(dest.Overrides) == 0 {
		dest.Overrides = nil
	}
}

func mediaTypePlural(mediaType MediaType) string {
	if mediaType == MediaTypeSeries {
		return string(MediaTypeSeries)
	}
	return "movies"
}

func normalizeConditions(c RouteConditions) RouteConditions {
	lower := func(values []string) []string {
		var out []string
		for _, v := range values {
			if v = strings.ToLower(strings.TrimSpace(v)); v != "" && !slices.Contains(out, v) {
				out = append(out, v)
			}
		}
		return out
	}
	upper := func(values []string) []string {
		var out []string
		for _, v := range values {
			if v = strings.ToUpper(strings.TrimSpace(v)); v != "" && !slices.Contains(out, v) {
				out = append(out, v)
			}
		}
		return out
	}
	ids := func(values []int) []int {
		out := slices.Clone(values)
		slices.Sort(out)
		return slices.Compact(out)
	}
	c.GenreIDs, c.KeywordIDs = ids(c.GenreIDs), ids(c.KeywordIDs)
	c.NetworkIDs, c.CompanyIDs = ids(c.NetworkIDs), ids(c.CompanyIDs)
	c.RequesterUserIDs = ids(c.RequesterUserIDs)
	c.OriginalLanguages, c.OriginCountries = lower(c.OriginalLanguages), upper(c.OriginCountries)
	return c
}

func validateConditions(c RouteConditions, fields map[string]string) {
	for field, values := range map[string][]int{
		"genre_ids": c.GenreIDs, "keyword_ids": c.KeywordIDs, "network_ids": c.NetworkIDs,
		"company_ids": c.CompanyIDs, "requester_user_ids": c.RequesterUserIDs,
	} {
		for _, v := range values {
			if v <= 0 {
				fields["conditions."+field] = "IDs must be positive."
				break
			}
		}
	}
	for _, v := range c.OriginalLanguages {
		if !languageCode.MatchString(v) {
			fields["conditions.original_languages"] = "Use ISO 639-1 language codes such as ja or en."
			break
		}
	}
	for _, v := range c.OriginCountries {
		if !countryCode.MatchString(v) {
			fields["conditions.origin_countries"] = "Use ISO 3166-1 country codes such as JP or US."
			break
		}
	}
	for field, year := range map[string]int{"year_from": c.YearFrom, "year_to": c.YearTo} {
		if year != 0 && (year < 1870 || year > 2200) {
			fields["conditions."+field] = "Use a year between 1870 and 2200."
		}
	}
	if c.YearFrom != 0 && c.YearTo != 0 && c.YearFrom > c.YearTo {
		fields["conditions.year_to"] = "The end year comes before the start year."
	}
}

func conditionsEmpty(c RouteConditions) bool {
	return c.Anime == nil && len(c.GenreIDs) == 0 && len(c.KeywordIDs) == 0 && len(c.OriginalLanguages) == 0 &&
		len(c.OriginCountries) == 0 && c.YearFrom == 0 && c.YearTo == 0 && len(c.NetworkIDs) == 0 &&
		len(c.CompanyIDs) == 0 && len(c.RequesterUserIDs) == 0
}

func (r *Repository) GetRoute(ctx context.Context, id string) (*Route, error) {
	route, err := scanRoute(r.pool.QueryRow(ctx, `SELECT `+routeColumns+` FROM request_routes WHERE id = $1`, id))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &route, nil
}

// SaveRouteConditional inserts or replaces a route. expected is the revision
// the editor read: zero creates, -1 overwrites whatever is there.
func (r *Repository) SaveRouteConditional(ctx context.Context, route Route, expected int64) (*Route, error) {
	conditions, err := json.Marshal(route.Conditions)
	if err != nil {
		return nil, err
	}
	hdOverrides, err := json.Marshal(nonNilOverrides(route.HD.Overrides))
	if err != nil {
		return nil, err
	}
	uhdOverrides, err := json.Marshal(nonNilOverrides(route.UHD.Overrides))
	if err != nil {
		return nil, err
	}
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	// Only a create (expected 0) or the fallback's first save may find no
	// row; a rule deleted under an editor must not come back.
	allowMissing := expected == 0 || route.IsFallback
	if err := lockRevision(ctx, tx, `SELECT revision FROM request_routes WHERE id = $1 FOR UPDATE`, []any{route.ID}, expected, allowMissing); err != nil {
		return nil, err
	}
	// A missing row cannot be locked, so two first saves of the fallback can
	// both get here; the revision predicate lets only one of them win.
	saved, err := scanRoute(tx.QueryRow(ctx, `
		INSERT INTO request_routes (id, media_type, position, name, enabled, is_fallback, conditions,
			hd_integration_id, hd_overrides, uhd_integration_id, uhd_overrides, skip_uhd)
		VALUES ($1, $2, $3, $4, $5, $6, $7, nullif($8, ''), $9, nullif($10, ''), $11, $12)
		ON CONFLICT (id) DO UPDATE SET
			name = EXCLUDED.name, enabled = EXCLUDED.enabled, conditions = EXCLUDED.conditions,
			hd_integration_id = EXCLUDED.hd_integration_id, hd_overrides = EXCLUDED.hd_overrides,
			uhd_integration_id = EXCLUDED.uhd_integration_id, uhd_overrides = EXCLUDED.uhd_overrides,
			skip_uhd = EXCLUDED.skip_uhd, updated_at = now()
		WHERE $13::bigint = -1 OR request_routes.revision = $13::bigint
		RETURNING `+routeColumns,
		route.ID, route.MediaType, route.Position, route.Name, route.Enabled, route.IsFallback, conditions,
		route.HD.IntegrationID, hdOverrides, route.UHD.IntegrationID, uhdOverrides, route.SkipUHD, expected))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrStaleRevision
	}
	if err != nil {
		return nil, fmt.Errorf("save request route: %w", err)
	}
	return &saved, tx.Commit(ctx)
}

func nonNilOverrides(overrides map[string]any) map[string]any {
	if overrides == nil {
		return map[string]any{}
	}
	return overrides
}

func (r *Repository) DeleteRouteConditional(ctx context.Context, id string, expected int64) error {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := lockRevision(ctx, tx, `SELECT revision FROM request_routes WHERE id = $1 FOR UPDATE`, []any{id}, expected, false); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM request_routes WHERE id = $1 AND NOT is_fallback`, id); err != nil {
		return fmt.Errorf("delete request route: %w", err)
	}
	return tx.Commit(ctx)
}

func (r *Repository) ReorderRoutes(ctx context.Context, mediaType MediaType, ids []string) error {
	if _, err := r.pool.Exec(ctx, `
		UPDATE request_routes r SET position = o.position - 1, updated_at = now()
		FROM unnest($2::text[]) WITH ORDINALITY AS o(id, position)
		WHERE r.id = o.id AND r.media_type = $1 AND NOT r.is_fallback
	`, mediaType, ids); err != nil {
		return fmt.Errorf("reorder request routes: %w", err)
	}
	return nil
}
