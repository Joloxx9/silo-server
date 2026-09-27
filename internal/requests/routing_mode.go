package requests

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

// Routing mode: Standard sends each media type to its one server, and its 4K
// copies to its one 4K server, with each server's own settings; the routing
// rules are kept but paused. Advanced routes with the rules. Standard needs at
// most one enabled server of each kind per media type (a normal one and a 4K
// one), so adding or enabling a second turns Advanced on.

// RoutingMode is how requests find their server.
type RoutingMode string

const (
	RoutingStandard RoutingMode = "standard"
	RoutingAdvanced RoutingMode = "advanced"
)

// RoutingSettings is the stored routing mode.
type RoutingSettings struct {
	Mode      RoutingMode
	Revision  int64
	UpdatedAt time.Time
}

// StandardDestination is where Standard sends a media type: its one normal
// server and its one 4K server, either of which may be missing.
type StandardDestination struct {
	MediaType        MediaType
	HDIntegrationID  string
	UHDIntegrationID string
}

// RoutingOverview is the routing mode with what Standard would do: where it
// sends each media type, or why it cannot be used.
type RoutingOverview struct {
	RoutingSettings
	Standard []StandardDestination
	// StandardBlocker says why Standard cannot be used; empty when it can.
	StandardBlocker string
}

// RoutingModeStore reads and writes the routing mode. The PostgreSQL
// repository implements it.
type RoutingModeStore interface {
	GetRoutingSettings(ctx context.Context) (RoutingSettings, error)
	// UpdateRoutingModeConditional sets the mode when the revision still
	// matches expected (-1 to overwrite). Standard is refused with a
	// ValidationError when the servers do not allow it.
	UpdateRoutingModeConditional(ctx context.Context, mode RoutingMode, expected int64) (RoutingSettings, error)
}

var standardMediaTypes = []MediaType{MediaTypeMovie, MediaTypeSeries}

// serverServes reports whether a server takes a media type: a Radarr or
// Sonarr by its kind, any other by the media types it lists.
func serverServes(in Integration, mediaType MediaType) bool {
	if kind, _ := in.PluginConfig[configServiceKind].(string); kind != "" {
		return kind == map[MediaType]string{MediaTypeMovie: kindRadarr, MediaTypeSeries: kindSonarr}[mediaType]
	}
	return integrationSupportsMediaType(in, mediaType)
}

// is4KServer reports whether a server is marked as the 4K one.
func is4KServer(in Integration) bool {
	for _, key := range []string{configIs4K, configIsDefault4K} {
		switch flagged := in.PluginConfig[key].(type) {
		case bool:
			if flagged {
				return true
			}
		case string:
			if flagged == "true" {
				return true
			}
		}
	}
	return false
}

// standardLayout works out where Standard sends each media type from the
// enabled servers, and why Standard cannot be used when a media type has more
// than one normal or more than one 4K server.
func standardLayout(integrations []Integration) ([]StandardDestination, string) {
	var out []StandardDestination
	var problems []string
	for _, mediaType := range standardMediaTypes {
		var hd, uhd []Integration
		for _, in := range integrations {
			if !in.Enabled || !serverServes(in, mediaType) {
				continue
			}
			if is4KServer(in) {
				uhd = append(uhd, in)
			} else {
				hd = append(hd, in)
			}
		}
		noun := mediaTypePlural(mediaType)
		if len(hd) > 1 {
			problems = append(problems, fmt.Sprintf("%s can go to more than one server (%s)", capitalize(noun), serverNames(hd)))
		}
		if len(uhd) > 1 {
			problems = append(problems, fmt.Sprintf("more than one 4K server takes %s (%s)", noun, serverNames(uhd)))
		}
		if len(hd) > 1 || len(uhd) > 1 || len(hd)+len(uhd) == 0 {
			continue
		}
		dest := StandardDestination{MediaType: mediaType}
		if len(hd) == 1 {
			dest.HDIntegrationID = hd[0].ID
		}
		if len(uhd) == 1 {
			dest.UHDIntegrationID = uhd[0].ID
		}
		out = append(out, dest)
	}
	if len(problems) > 0 {
		return nil, capitalize(strings.Join(problems, "; ")) +
			". Standard sends each request to one server, plus one server marked 4K."
	}
	return out, ""
}

func serverNames(servers []Integration) string {
	names := make([]string, 0, len(servers))
	for _, in := range servers {
		names = append(names, in.Name)
	}
	slices.Sort(names)
	return strings.Join(names, ", ")
}

func capitalize(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

// standardRouteID names the route Standard routes a media type with. It is
// not stored; targets record it as the route that sent them.
func standardRouteID(mediaType MediaType) string { return "standard-" + string(mediaType) }

// standardRouteName is how targets and the preview name Standard's route.
const standardRouteName = "Standard"

// standardRoutes is Standard's routing for a media type as one fallback
// route: its normal server for HD and its 4K server for 4K, with no
// overrides; for series, an anime route ahead of it sets Sonarr's anime
// series type. A media type whose server is not a Radarr or Sonarr gets none, so
// that plugin keeps routing it itself.
func standardRoutes(integrations []Integration, layout []StandardDestination, mediaType MediaType) []Route {
	for _, dest := range layout {
		if dest.MediaType != mediaType {
			continue
		}
		for _, id := range []string{dest.HDIntegrationID, dest.UHDIntegrationID} {
			for _, in := range integrations {
				if in.ID == id {
					if kind, _ := in.PluginConfig[configServiceKind].(string); kind == "" {
						return nil
					}
				}
			}
		}
		routes := []Route{{
			ID: standardRouteID(mediaType), MediaType: mediaType, Position: 1000, Name: standardRouteName,
			Enabled: true, IsFallback: true,
			HD:  RouteDestination{IntegrationID: dest.HDIntegrationID},
			UHD: RouteDestination{IntegrationID: dest.UHDIntegrationID},
		}}
		if mediaType == MediaTypeSeries {
			// Anime goes to the same servers with Sonarr's anime series type,
			// which numbers episodes the way anime releases do; Seerr does
			// the same. Other settings stay the server's own.
			anime := func(id string) RouteDestination {
				if id == "" {
					return RouteDestination{}
				}
				return RouteDestination{IntegrationID: id, Overrides: map[string]any{configSeriesType: seriesTypeAnime}}
			}
			routes = append([]Route{{
				ID: standardRouteID(mediaType) + "-anime", MediaType: mediaType, Position: 0, Name: standardRouteName,
				Enabled: true, Conditions: RouteConditions{Anime: new(true)},
				HD: anime(dest.HDIntegrationID), UHD: anime(dest.UHDIntegrationID),
			}}, routes...)
		}
		return routes
	}
	return nil
}

// isStandardRouting reports whether routes are Standard's for the media type.
func isStandardRouting(routes []Route, mediaType MediaType) bool {
	return len(routes) > 0 && routes[len(routes)-1].ID == standardRouteID(mediaType)
}

func (s *Service) routingModeStore() (RoutingModeStore, error) {
	store, ok := s.store.(RoutingModeStore)
	if !ok {
		return nil, fmt.Errorf("request store does not support routing modes")
	}
	return store, nil
}

// GetRoutingOverview returns the routing mode and what Standard would do.
func (s *Service) GetRoutingOverview(ctx context.Context, v Viewer) (*RoutingOverview, error) {
	if !v.IsAdmin {
		return nil, ErrForbidden
	}
	store, err := s.routingModeStore()
	if err != nil {
		return nil, err
	}
	settings, err := store.GetRoutingSettings(ctx)
	if err != nil {
		return nil, err
	}
	integrations, err := s.store.ListIntegrations(ctx)
	if err != nil {
		return nil, err
	}
	layout, blocker := standardLayout(integrations)
	return &RoutingOverview{RoutingSettings: settings, Standard: layout, StandardBlocker: blocker}, nil
}

// UpdateRoutingModeConditional switches between Standard and Advanced.
func (s *Service) UpdateRoutingModeConditional(ctx context.Context, v Viewer, mode RoutingMode, expected int64) (*RoutingOverview, error) {
	if !v.IsAdmin {
		return nil, ErrForbidden
	}
	if mode != RoutingStandard && mode != RoutingAdvanced {
		return nil, fmt.Errorf("%w: invalid routing mode", ErrInvalidInput)
	}
	store, err := s.routingModeStore()
	if err != nil {
		return nil, err
	}
	if _, err := store.UpdateRoutingModeConditional(ctx, mode, expected); err != nil {
		return nil, err
	}
	return s.GetRoutingOverview(ctx, v)
}

// lockRoutingMode orders changes to the routing mode and to the servers, so a
// server added while Standard is turned on cannot leave Standard on with two
// servers of a kind.
func lockRoutingMode(ctx context.Context, tx pgx.Tx) error {
	_, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtext('request-routing-mode'))`)
	return err
}

func (r *Repository) GetRoutingSettings(ctx context.Context) (RoutingSettings, error) {
	return scanRoutingSettings(r.pool.QueryRow(ctx, `SELECT mode, revision, updated_at FROM request_routing WHERE id`))
}

func scanRoutingSettings(row pgx.Row) (RoutingSettings, error) {
	var out RoutingSettings
	err := row.Scan(&out.Mode, &out.Revision, &out.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		// The migration inserts the row; a database without it routes with
		// the rules, as before Standard existed.
		return RoutingSettings{Mode: RoutingAdvanced}, nil
	}
	if err != nil {
		return RoutingSettings{}, fmt.Errorf("get request routing mode: %w", err)
	}
	return out, nil
}

func (r *Repository) UpdateRoutingModeConditional(ctx context.Context, mode RoutingMode, expected int64) (RoutingSettings, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return RoutingSettings{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := lockRoutingMode(ctx, tx); err != nil {
		return RoutingSettings{}, err
	}
	if err := lockRevision(ctx, tx, `SELECT revision FROM request_routing WHERE id FOR UPDATE`, nil, expected, true); err != nil {
		return RoutingSettings{}, err
	}
	integrations, err := r.listIntegrations(ctx, tx)
	if err != nil {
		return RoutingSettings{}, err
	}
	layout, blocker := standardLayout(integrations)
	if mode == RoutingStandard && blocker != "" {
		return RoutingSettings{}, &ValidationError{FieldErrors: map[string]string{"mode": blocker}}
	}
	current, err := scanRoutingSettings(tx.QueryRow(ctx, `SELECT mode, revision, updated_at FROM request_routing WHERE id`))
	if err != nil {
		return RoutingSettings{}, err
	}
	if current.Mode == RoutingStandard && mode == RoutingAdvanced {
		if err := seedAdvancedFromStandard(ctx, tx, layout, integrations); err != nil {
			return RoutingSettings{}, err
		}
	}
	out, err := setRoutingMode(ctx, tx, mode)
	if err != nil {
		return RoutingSettings{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return RoutingSettings{}, err
	}
	return out, nil
}

func setRoutingMode(ctx context.Context, tx pgx.Tx, mode RoutingMode) (RoutingSettings, error) {
	return scanRoutingSettings(tx.QueryRow(ctx, `
		INSERT INTO request_routing (id, mode, updated_at) VALUES (true, $1, now())
		ON CONFLICT (id) DO UPDATE SET mode = EXCLUDED.mode, updated_at = now()
		RETURNING mode, revision, updated_at`, mode))
}

// seedAdvancedFromStandard gives Everything else the servers Standard was
// using, where it has none, so turning Advanced on sends requests where they
// went before. That includes a 4K server an admin once cleared from
// Everything else: Standard was sending 4K copies there since. Only Radarr and
// Sonarr servers that still take the media type, as saved now, are used; a
// media type another plugin (Seerr) routed itself stays with that plugin.
func seedAdvancedFromStandard(ctx context.Context, tx pgx.Tx, layout []StandardDestination, integrations []Integration) error {
	usable := func(id string, mediaType MediaType, fourK bool) bool {
		for _, in := range integrations {
			if in.ID != id {
				continue
			}
			kind, _ := in.PluginConfig[configServiceKind].(string)
			return kind != "" && in.Enabled && serverServes(in, mediaType) && is4KServer(in) == fourK
		}
		return false
	}
	for _, dest := range layout {
		if !usable(dest.HDIntegrationID, dest.MediaType, false) {
			continue
		}
		var uhd *string
		if usable(dest.UHDIntegrationID, dest.MediaType, true) {
			uhd = &dest.UHDIntegrationID
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO request_routes (id, media_type, position, name, is_fallback, hd_integration_id, uhd_integration_id)
			VALUES ($1, $2, 1000, $3, true, $4, $5)
			ON CONFLICT (id) DO UPDATE SET
				hd_integration_id = coalesce(request_routes.hd_integration_id, EXCLUDED.hd_integration_id),
				uhd_integration_id = coalesce(request_routes.uhd_integration_id, EXCLUDED.uhd_integration_id)
			WHERE request_routes.hd_integration_id IS NULL
			   OR (request_routes.uhd_integration_id IS NULL AND EXCLUDED.uhd_integration_id IS NOT NULL AND NOT request_routes.skip_uhd)`,
			FallbackRouteID(dest.MediaType), dest.MediaType, fallbackRouteName, dest.HDIntegrationID, uhd); err != nil {
			return fmt.Errorf("carry standard routing into everything else: %w", err)
		}
	}
	return nil
}

// standardBeforeSave reads, in a transaction that is about to add or change a
// server, whether Standard is on and where it sends requests. Pass the result
// to advanceIfStandardBroken after the save.
func (r *Repository) standardBeforeSave(ctx context.Context, tx pgx.Tx) (layout []StandardDestination, standard bool, err error) {
	if err := lockRoutingMode(ctx, tx); err != nil {
		return nil, false, err
	}
	current, err := scanRoutingSettings(tx.QueryRow(ctx, `SELECT mode, revision, updated_at FROM request_routing WHERE id FOR UPDATE`))
	if err != nil || current.Mode != RoutingStandard {
		return nil, false, err
	}
	integrations, err := r.listIntegrations(ctx, tx)
	if err != nil {
		return nil, false, err
	}
	layout, _ = standardLayout(integrations)
	return layout, true, nil
}

// advanceIfStandardBroken turns Advanced on when a saved server leaves a media
// type with two servers of a kind, and gives Everything else the servers
// Standard was using before, so requests keep going where they went.
func (r *Repository) advanceIfStandardBroken(ctx context.Context, tx pgx.Tx, before []StandardDestination) error {
	integrations, err := r.listIntegrations(ctx, tx)
	if err != nil {
		return err
	}
	if _, blocker := standardLayout(integrations); blocker == "" {
		return nil
	}
	if err := seedAdvancedFromStandard(ctx, tx, before, integrations); err != nil {
		return err
	}
	if _, err := setRoutingMode(ctx, tx, RoutingAdvanced); err != nil {
		return fmt.Errorf("turn advanced routing on: %w", err)
	}
	return nil
}
