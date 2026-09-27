package requests

import (
	"cmp"
	"fmt"
	"maps"
	"slices"
	"strings"
)

// Routing: Silo decides which server each quality tier of a request goes to,
// and hands the router plugin only that server. Routes are evaluated per tier
// in order: the first enabled route whose conditions match the request and
// that has a destination for the tier wins, and the media type's fallback
// route (no conditions) comes last. A media type with no routes keeps the
// plugin's own routing (every connection is handed over and the plugin picks).

// The Sonarr/Radarr plugin's config keys and kinds that routing sets or reads.
const (
	configServiceKind  = "service_kind"
	configIsDefault    = "is_default"
	configIsDefault4K  = "is_default_4k"
	configIs4K         = "is_4k"
	configAnimeEnabled = "anime_enabled"
	kindRadarr         = "radarr"
	kindSonarr         = "sonarr"
)

// fallbackRouteName names a media type's fallback until an admin renames it.
const fallbackRouteName = "Everything else"

// Route sends the requests it matches to a server per quality tier.
type Route struct {
	ID         string
	MediaType  MediaType
	Position   int
	Name       string
	Enabled    bool
	IsFallback bool
	Conditions RouteConditions
	HD         RouteDestination
	UHD        RouteDestination
	// SkipUHD stops a matching title from getting a 4K copy at all, rather
	// than letting the 4K tier fall through to a later route.
	SkipUHD  bool
	Revision int64
}

// RouteDestination is a server and the settings a route overrides on it. An
// empty IntegrationID means the route has no destination for the tier.
type RouteDestination struct {
	IntegrationID string
	// Overrides replace keys of the server's plugin config for requests this
	// route sends, e.g. root_folder, quality_profile_id, tags, series_type.
	Overrides map[string]any
}

// RouteConditions narrow a route to some requests. Every set field must match
// (AND); a list matches when the request has any of its values (OR). An empty
// condition set matches everything.
type RouteConditions struct {
	Anime             *bool    `json:"anime,omitempty"`
	GenreIDs          []int    `json:"genre_ids,omitempty"`
	KeywordIDs        []int    `json:"keyword_ids,omitempty"`
	OriginalLanguages []string `json:"original_languages,omitempty"`
	OriginCountries   []string `json:"origin_countries,omitempty"`
	// YearFrom and YearTo bound the release (or first-air) year, inclusive;
	// a decade is 1980-1989.
	YearFrom         int   `json:"year_from,omitempty"`
	YearTo           int   `json:"year_to,omitempty"`
	NetworkIDs       []int `json:"network_ids,omitempty"`
	CompanyIDs       []int `json:"company_ids,omitempty"`
	RequesterUserIDs []int `json:"requester_user_ids,omitempty"`
}

// Matches reports whether a request satisfies every set condition, judged on
// its stored routing facts.
func (c RouteConditions) Matches(req Request) bool {
	f := req.RoutingFacts
	switch {
	case c.Anime != nil && *c.Anime != f.Anime:
		return false
	case len(c.GenreIDs) > 0 && !anyInt(c.GenreIDs, f.GenreIDs):
		return false
	case len(c.KeywordIDs) > 0 && !anyInt(c.KeywordIDs, f.KeywordIDs):
		return false
	case len(c.OriginalLanguages) > 0 && !anyFold(c.OriginalLanguages, []string{f.OriginalLanguage}):
		return false
	case len(c.OriginCountries) > 0 && !anyFold(c.OriginCountries, f.OriginCountries):
		return false
	case c.YearFrom > 0 && (f.Year == 0 || f.Year < c.YearFrom):
		return false
	case c.YearTo > 0 && (f.Year == 0 || f.Year > c.YearTo):
		return false
	case len(c.NetworkIDs) > 0 && !anyInt(c.NetworkIDs, f.NetworkIDs):
		return false
	case len(c.CompanyIDs) > 0 && !anyInt(c.CompanyIDs, f.CompanyIDs):
		return false
	case len(c.RequesterUserIDs) > 0 && !slices.Contains(c.RequesterUserIDs, req.RequestedByUserID):
		return false
	}
	return true
}

func anyInt(want, have []int) bool {
	for _, v := range have {
		if slices.Contains(want, v) {
			return true
		}
	}
	return false
}

func anyFold(want, have []string) bool {
	for _, v := range have {
		for _, w := range want {
			if v != "" && strings.EqualFold(v, w) {
				return true
			}
		}
	}
	return false
}

// RouteDecision is where one quality tier of a request goes, and which route
// sent it there. Skip marks a tier a matching route chose not to send at all
// (skip_uhd): the title gets no copy in that tier, whatever force-dual says.
type RouteDecision struct {
	RouteID       string
	RouteName     string
	IntegrationID string
	Overrides     map[string]any
	Skip          bool
}

// orderRoutes sorts a media type's routes into evaluation order: by position,
// the fallback last.
func orderRoutes(routes []Route) []Route {
	out := slices.Clone(routes)
	slices.SortStableFunc(out, func(a, b Route) int {
		if a.IsFallback != b.IsFallback {
			if a.IsFallback {
				return 1
			}
			return -1
		}
		return cmp.Or(cmp.Compare(a.Position, b.Position), cmp.Compare(a.ID, b.ID))
	})
	return out
}

// decideRoutes picks a destination for each quality. A tier no route sends
// anywhere is absent from the result; a tier a route skips is present with
// Skip set.
func decideRoutes(routes []Route, req Request, qualities []Quality) map[Quality]RouteDecision {
	ordered := orderRoutes(routes)
	out := make(map[Quality]RouteDecision, len(qualities))
	for _, q := range qualities {
		for _, route := range ordered {
			if !route.Enabled || route.MediaType != req.MediaType || !route.Conditions.Matches(req) {
				continue
			}
			if q == Quality2160p && route.SkipUHD {
				out[q] = RouteDecision{RouteID: route.ID, RouteName: route.Name, Skip: true}
				break
			}
			dest := route.HD
			if q == Quality2160p {
				dest = route.UHD
			}
			if dest.IntegrationID == "" {
				continue
			}
			out[q] = RouteDecision{
				RouteID:       route.ID,
				RouteName:     route.Name,
				IntegrationID: dest.IntegrationID,
				Overrides:     dest.Overrides,
			}
			break
		}
	}
	return out
}

// routedConnection builds the one connection a routed tier is sent to. The
// route already chose the server, so the connection says so in the Sonarr/
// Radarr plugin's own terms: it is the tier's default, and the plugin's anime
// overlay is off because the route's overrides replace it. A plugin without
// those keys ignores them.
func routedConnection(fc *fulfillContext, d RouteDecision, mediaType MediaType, q Quality) (ResolvedRouterConnection, int, string, error) {
	var in *Integration
	for i := range fc.integrations {
		if fc.integrations[i].ID == d.IntegrationID {
			in = &fc.integrations[i]
			break
		}
	}
	switch {
	case in == nil:
		return ResolvedRouterConnection{}, 0, "", fmt.Errorf("route %q sends to a server that no longer exists", d.RouteName)
	case !in.Enabled:
		return ResolvedRouterConnection{}, 0, "", fmt.Errorf("route %q sends to %q, which is disabled", d.RouteName, in.Name)
	case in.InstallationID == nil || in.CapabilityID == "":
		return ResolvedRouterConnection{}, 0, "", fmt.Errorf("%s (route %q)", msgRouterUnbound, d.RouteName)
	case strings.TrimSpace(in.APIKeyRef) == "":
		return ResolvedRouterConnection{}, 0, "", fmt.Errorf("%s (route %q)", msgRouterNoKey, d.RouteName)
	case !integrationSupportsMediaType(*in, mediaType):
		// The server's media types can change after a route points at it.
		return ResolvedRouterConnection{}, 0, "", fmt.Errorf("route %q sends %s to %q, which does not take them", d.RouteName, mediaTypePlural(mediaType), in.Name)
	}
	// A server can be switched to the other kind after a route points at it.
	if kind, _ := in.PluginConfig[configServiceKind].(string); kind != "" {
		if want := map[MediaType]string{MediaTypeMovie: kindRadarr, MediaTypeSeries: kindSonarr}[mediaType]; want != "" && kind != want {
			return ResolvedRouterConnection{}, 0, "", fmt.Errorf("route %q sends %s to %q, a %s server", d.RouteName, mediaType, in.Name, kind)
		}
	}
	config := maps.Clone(in.PluginConfig)
	if config == nil {
		config = map[string]any{}
	}
	maps.Copy(config, d.Overrides)
	config[configIsDefault] = q == Quality1080p
	config[configIsDefault4K] = q == Quality2160p
	if q == Quality2160p {
		config[configIs4K] = true
	}
	config[configAnimeEnabled] = false
	return ResolvedRouterConnection{
		ID:      in.ID,
		BaseURL: in.BaseURL,
		APIKey:  strings.TrimSpace(in.APIKeyRef),
		Config:  config,
	}, *in.InstallationID, in.CapabilityID, nil
}
