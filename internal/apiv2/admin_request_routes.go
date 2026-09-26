package apiv2

import (
	"context"
	"errors"
	"net/http"
	"strconv"

	mediarequests "github.com/Silo-Server/silo-server/internal/requests"
)

// Request routing administration: the ordered rules that decide which server
// each quality tier of a request goes to (docs/architecture/media-requests.md,
// "Routing").

const (
	opListRequestRoutes    = "listRequestRoutes"
	opGetRequestRoute      = "getRequestRoute"
	opCreateRequestRoute   = "createRequestRoute"
	opUpdateRequestRoute   = "updateRequestRoute"
	opDeleteRequestRoute   = "deleteRequestRoute"
	opReorderRequestRoutes = "reorderRequestRoutes"
	opPreviewRequestRoute  = "previewRequestRoute"
)

var adminRequestRouteOperationIDs = []string{opListRequestRoutes, opGetRequestRoute, opCreateRequestRoute,
	opUpdateRequestRoute, opDeleteRequestRoute, opReorderRequestRoutes, opPreviewRequestRoute}

// adminRequestRoutes is the route administration slice of the request
// service.
type adminRequestRoutes interface {
	ListRoutesAdmin(context.Context, mediarequests.Viewer) ([]mediarequests.Route, error)
	GetRoute(context.Context, mediarequests.Viewer, string) (*mediarequests.Route, error)
	CreateRoute(context.Context, mediarequests.Viewer, mediarequests.Route) (*mediarequests.Route, error)
	UpdateRouteConditional(context.Context, mediarequests.Viewer, mediarequests.Route, int64) (*mediarequests.Route, error)
	DeleteRouteConditional(context.Context, mediarequests.Viewer, string, int64) error
	ReorderRoutes(context.Context, mediarequests.Viewer, mediarequests.MediaType, []string) ([]mediarequests.Route, error)
	PreviewRoute(context.Context, mediarequests.Viewer, mediarequests.MediaType, int, int) (*mediarequests.RoutePreview, error)
}

// AdminRequestRouteConditions narrow a route. Every set field must match; a
// list matches when the title has any of its values.
type AdminRequestRouteConditions struct {
	Anime             *bool    `json:"anime,omitempty" doc:"Match titles TMDB tags as anime (true) or not (false)"`
	GenreIDs          []int    `json:"genre_ids,omitempty" doc:"TMDB genre IDs"`
	KeywordIDs        []int    `json:"keyword_ids,omitempty" doc:"TMDB keyword IDs"`
	OriginalLanguages []string `json:"original_languages,omitempty" doc:"ISO 639-1 codes of the original language" example:"[\"ja\"]"`
	OriginCountries   []string `json:"origin_countries,omitempty" doc:"ISO 3166-1 country codes" example:"[\"JP\"]"`
	YearFrom          int      `json:"year_from,omitempty" doc:"First release (or first-air) year, inclusive" example:"1980"`
	YearTo            int      `json:"year_to,omitempty" doc:"Last release (or first-air) year, inclusive" example:"1989"`
	NetworkIDs        []int    `json:"network_ids,omitempty" doc:"TMDB network IDs (series)"`
	CompanyIDs        []int    `json:"company_ids,omitempty" doc:"TMDB production company IDs (movies)"`
	RequesterUserIDs  []int    `json:"requester_user_ids,omitempty" doc:"Accounts whose requests the route applies to"`
}

// AdminRequestRouteDestination is where a route sends one quality tier.
type AdminRequestRouteDestination struct {
	IntegrationID string         `json:"integration_id,omitempty" doc:"The request server; empty when the route sends nothing for this tier"`
	Overrides     map[string]any `json:"overrides,omitempty" doc:"Server settings this route replaces, keyed like the server's plugin config: root_folder, quality_profile_id, tags, series_type, minimum_availability, ..."`
}

// AdminRequestRoute is one routing rule, or a media type's fallback.
type AdminRequestRoute struct {
	ID         string                       `json:"id" doc:"Opaque route ID; the fallback's is fallback-movie or fallback-series" example:"fallback-movie"`
	MediaType  string                       `json:"media_type" enum:"movie,series"`
	Position   int                          `json:"position" doc:"Evaluation order within the media type; the fallback is always last"`
	Name       string                       `json:"name" example:"Anime"`
	Enabled    bool                         `json:"enabled"`
	IsFallback bool                         `json:"is_fallback" doc:"The media type's default destination: it has no conditions and cannot be deleted"`
	Conditions AdminRequestRouteConditions  `json:"conditions"`
	HD         AdminRequestRouteDestination `json:"hd" doc:"Where the HD (1080p) copy goes"`
	UHD        AdminRequestRouteDestination `json:"uhd" doc:"Where the 4K copy goes"`
	SkipUHD    bool                         `json:"skip_uhd" doc:"Matching titles get no 4K copy at all"`
}

// AdminRequestRouteBody is the editable part of a route. media_type is read
// on create only.
type AdminRequestRouteBody struct {
	MediaType  string                       `json:"media_type,omitempty" enum:"movie,series" doc:"Required on create; ignored on update"`
	Name       string                       `json:"name,omitempty" maxLength:"100"`
	Enabled    bool                         `json:"enabled"`
	Conditions AdminRequestRouteConditions  `json:"conditions"`
	HD         AdminRequestRouteDestination `json:"hd"`
	UHD        AdminRequestRouteDestination `json:"uhd"`
	SkipUHD    bool                         `json:"skip_uhd"`
}

type AdminRequestRouteIDInput struct {
	ID string `path:"id" minLength:"1" doc:"The route" example:"fallback-movie"`
}
type AdminRequestRouteOutput struct {
	ETag string `header:"ETag"`
	Body AdminRequestRoute
}
type AdminRequestRouteCreateInput struct{ Body AdminRequestRouteBody }
type AdminRequestRouteCreateOutput struct {
	Location string `header:"Location"`
	ETag     string `header:"ETag"`
	Body     AdminRequestRoute
}
type AdminRequestRouteUpdateInput struct {
	AdminRequestRouteIDInput
	IfMatch     string `header:"If-Match"`
	IfNoneMatch string `header:"If-None-Match"`
	Body        AdminRequestRouteBody
}
type AdminRequestRouteDeleteInput struct {
	AdminRequestRouteIDInput
	IfMatch     string `header:"If-Match"`
	IfNoneMatch string `header:"If-None-Match"`
}
type AdminRequestRouteCollectionOutput struct {
	Body Collection[AdminRequestRoute]
}
type AdminRequestRouteReorderInput struct {
	Body struct {
		MediaType string   `json:"media_type" enum:"movie,series"`
		IDs       []string `json:"ids" maxItems:"100" doc:"Every rule of the media type, fallback excluded, in the new order"`
	}
}

// AdminRequestRoutePreviewInput asks how a title would be routed.
type AdminRequestRoutePreviewInput struct {
	Body struct {
		MediaType       string `json:"media_type" enum:"movie,series"`
		TMDBID          int    `json:"tmdb_id" minimum:"1" doc:"TMDB identifier (external, not a Silo ID)" example:"129"`
		RequesterUserID *ID    `json:"requester_user_id,omitempty" doc:"Route as this account's request; account conditions are skipped when absent"`
	}
}

// AdminRequestRouteFacts is what the routes matched on.
type AdminRequestRouteFacts struct {
	GenreIDs         []int    `json:"genre_ids"`
	KeywordIDs       []int    `json:"keyword_ids"`
	OriginalLanguage string   `json:"original_language,omitempty"`
	OriginCountries  []string `json:"origin_countries"`
	Year             int      `json:"year,omitempty"`
	NetworkIDs       []int    `json:"network_ids"`
	CompanyIDs       []int    `json:"company_ids"`
	Anime            bool     `json:"anime"`
}

// AdminRequestRoutePreviewTier is one tier's outcome.
type AdminRequestRoutePreviewTier struct {
	Quality         string         `json:"quality" enum:"1080p,2160p"`
	RouteID         string         `json:"route_id,omitempty"`
	RouteName       string         `json:"route_name,omitempty"`
	IntegrationID   string         `json:"integration_id,omitempty"`
	IntegrationName string         `json:"integration_name,omitempty"`
	Overrides       map[string]any `json:"overrides,omitempty"`
	Note            string         `json:"note,omitempty" doc:"Why no route sends the tier, or why it would fail"`
}

type AdminRequestRoutePreviewOutput struct {
	Body struct {
		Facts AdminRequestRouteFacts         `json:"facts"`
		Tiers []AdminRequestRoutePreviewTier `json:"tiers"`
	}
}

func registerAdminRequestRoutes(reg *Registry) {
	op := func(method, path, id, summary string, guard bool) Operation {
		o := Operation{Operation: humaOp(method, Prefix+path, id, "admin", summary), Class: ClassActingAdmin, DemoRestricted: isMutatingMethod(method), ServiceBacked: true, Guarded: guard}
		o.Errors = []int{http.StatusNotFound, http.StatusConflict, http.StatusUnprocessableEntity}
		if method != http.MethodGet {
			o.RetrySafety = RetrySafetyNonRetryable
		}
		return o
	}
	Register(reg, op(http.MethodGet, "/admin/request-routes", opListRequestRoutes, "List the request routing rules, in evaluation order per media type.", false), reg.listAdminRequestRoutes)
	Register(reg, op(http.MethodGet, "/admin/request-routes/{id}", opGetRequestRoute, "Get one request routing rule.", false), reg.getAdminRequestRoute)
	create := op(http.MethodPost, "/admin/request-routes", opCreateRequestRoute, "Add a request routing rule after the media type's existing rules.", false)
	create.DefaultStatus = http.StatusCreated
	Register(reg, create, reg.createAdminRequestRoute)
	update := op(http.MethodPut, "/admin/request-routes/{id}", opUpdateRequestRoute, "Replace a request routing rule; saving a media type's fallback creates it.", true)
	Register(reg, update, reg.updateAdminRequestRoute)
	del := op(http.MethodDelete, "/admin/request-routes/{id}", opDeleteRequestRoute, "Delete a request routing rule.", true)
	del.DefaultStatus = http.StatusNoContent
	Register(reg, del, reg.deleteAdminRequestRoute)
	Register(reg, op(http.MethodPost, "/admin/request-routes/order", opReorderRequestRoutes, "Set the evaluation order of a media type's routing rules.", false), reg.reorderAdminRequestRoutes)
	preview := op(http.MethodPost, "/admin/request-routes/preview", opPreviewRequestRoute, "Show which server each quality tier of a title would go to.", false)
	preview.RetrySafety = RetrySafetyNaturalIdempotent
	preview.DemoRestricted = false
	Register(reg, preview, reg.previewAdminRequestRoute)
}

func (reg *Registry) adminRequestRouteService() (adminRequestRoutes, *Problem) {
	s, ok := reg.deps.AdminRequests.(adminRequestRoutes)
	if !ok {
		return nil, unavailable("request routing")
	}
	return s, nil
}

func adminRequestRouteOf(r mediarequests.Route) AdminRequestRoute {
	c := r.Conditions
	return AdminRequestRoute{
		ID: r.ID, MediaType: string(r.MediaType), Position: r.Position, Name: r.Name, Enabled: r.Enabled,
		IsFallback: r.IsFallback, SkipUHD: r.SkipUHD,
		Conditions: AdminRequestRouteConditions{
			Anime: c.Anime, GenreIDs: c.GenreIDs, KeywordIDs: c.KeywordIDs, OriginalLanguages: c.OriginalLanguages,
			OriginCountries: c.OriginCountries, YearFrom: c.YearFrom, YearTo: c.YearTo, NetworkIDs: c.NetworkIDs,
			CompanyIDs: c.CompanyIDs, RequesterUserIDs: c.RequesterUserIDs,
		},
		HD:  AdminRequestRouteDestination{IntegrationID: r.HD.IntegrationID, Overrides: r.HD.Overrides},
		UHD: AdminRequestRouteDestination{IntegrationID: r.UHD.IntegrationID, Overrides: r.UHD.Overrides},
	}
}

func (b AdminRequestRouteBody) domain(id string) mediarequests.Route {
	c := b.Conditions
	return mediarequests.Route{
		ID: id, MediaType: mediarequests.MediaType(b.MediaType), Name: b.Name, Enabled: b.Enabled, SkipUHD: b.SkipUHD,
		Conditions: mediarequests.RouteConditions{
			Anime: c.Anime, GenreIDs: c.GenreIDs, KeywordIDs: c.KeywordIDs, OriginalLanguages: c.OriginalLanguages,
			OriginCountries: c.OriginCountries, YearFrom: c.YearFrom, YearTo: c.YearTo, NetworkIDs: c.NetworkIDs,
			CompanyIDs: c.CompanyIDs, RequesterUserIDs: c.RequesterUserIDs,
		},
		HD:  mediarequests.RouteDestination{IntegrationID: b.HD.IntegrationID, Overrides: b.HD.Overrides},
		UHD: mediarequests.RouteDestination{IntegrationID: b.UHD.IntegrationID, Overrides: b.UHD.Overrides},
	}
}

func routeTag(ctx context.Context, r mediarequests.Route) EntityTag {
	return adminRequestTag(ctx, "route", r.ID, r.Revision)
}

func (reg *Registry) listAdminRequestRoutes(ctx context.Context, _ *struct{}) (*AdminRequestRouteCollectionOutput, error) {
	s, p := reg.adminRequestRouteService()
	if p != nil {
		return nil, p
	}
	routes, err := s.ListRoutesAdmin(ctx, adminRequestViewer(ctx))
	if err != nil {
		return nil, requestProblem(err)
	}
	items := make([]AdminRequestRoute, 0, len(routes))
	for _, r := range routes {
		items = append(items, adminRequestRouteOf(r))
	}
	return &AdminRequestRouteCollectionOutput{Body: NewCollection(items)}, nil
}

func (reg *Registry) getAdminRequestRoute(ctx context.Context, in *AdminRequestRouteIDInput) (*AdminRequestRouteOutput, error) {
	s, p := reg.adminRequestRouteService()
	if p != nil {
		return nil, p
	}
	r, err := s.GetRoute(ctx, adminRequestViewer(ctx), in.ID)
	if err != nil {
		return nil, requestProblem(err)
	}
	return &AdminRequestRouteOutput{ETag: routeTag(ctx, *r).String(), Body: adminRequestRouteOf(*r)}, nil
}

func (reg *Registry) createAdminRequestRoute(ctx context.Context, in *AdminRequestRouteCreateInput) (*AdminRequestRouteCreateOutput, error) {
	s, p := reg.adminRequestRouteService()
	if p != nil {
		return nil, p
	}
	r, err := s.CreateRoute(ctx, adminRequestViewer(ctx), in.Body.domain(""))
	if err != nil {
		return nil, requestProblem(err)
	}
	return &AdminRequestRouteCreateOutput{Location: Prefix + "/admin/request-routes/" + r.ID, ETag: routeTag(ctx, *r).String(), Body: adminRequestRouteOf(*r)}, nil
}

// currentRouteRevision reads the route an editor is replacing. A fallback
// that has never been saved reads as revision zero.
func (reg *Registry) currentRouteRevision(ctx context.Context, s adminRequestRoutes, id string) (EntityTag, int64, *Problem) {
	current, err := s.GetRoute(ctx, adminRequestViewer(ctx), id)
	if err != nil {
		return EntityTag{}, 0, requestProblem(err)
	}
	return routeTag(ctx, *current), current.Revision, nil
}

func (reg *Registry) updateAdminRequestRoute(ctx context.Context, in *AdminRequestRouteUpdateInput) (*AdminRequestRouteOutput, error) {
	s, p := reg.adminRequestRouteService()
	if p != nil {
		return nil, p
	}
	tag, revision, p := reg.currentRouteRevision(ctx, s, in.ID)
	if p != nil {
		return nil, p
	}
	rev, p := adminRequestGuard(AdminRequestPreconditions{in.IfMatch, in.IfNoneMatch}, tag, revision)
	if p != nil {
		return nil, p
	}
	r, err := s.UpdateRouteConditional(ctx, adminRequestViewer(ctx), in.Body.domain(in.ID), rev)
	if errors.Is(err, mediarequests.ErrStaleRevision) {
		current, _, p := reg.currentRouteRevision(ctx, s, in.ID)
		if p != nil {
			return nil, p
		}
		return nil, NewProblem(TypePreconditionFailed, "The routing rule changed; reload before saving.").WithHeader("ETag", current.String())
	}
	if err != nil {
		return nil, requestProblem(err)
	}
	return &AdminRequestRouteOutput{ETag: routeTag(ctx, *r).String(), Body: adminRequestRouteOf(*r)}, nil
}

func (reg *Registry) deleteAdminRequestRoute(ctx context.Context, in *AdminRequestRouteDeleteInput) (*struct{}, error) {
	s, p := reg.adminRequestRouteService()
	if p != nil {
		return nil, p
	}
	current, err := s.GetRoute(ctx, adminRequestViewer(ctx), in.ID)
	if err != nil {
		return nil, requestProblem(err)
	}
	rev, p := adminRequestGuard(AdminRequestPreconditions{in.IfMatch, in.IfNoneMatch}, routeTag(ctx, *current), current.Revision)
	if p != nil {
		return nil, p
	}
	err = s.DeleteRouteConditional(ctx, adminRequestViewer(ctx), in.ID, rev)
	if errors.Is(err, mediarequests.ErrStaleRevision) {
		return nil, NewProblem(TypePreconditionFailed, "The routing rule changed; reload before deleting.")
	}
	if err != nil {
		return nil, requestProblem(err)
	}
	return nil, nil
}

func (reg *Registry) reorderAdminRequestRoutes(ctx context.Context, in *AdminRequestRouteReorderInput) (*AdminRequestRouteCollectionOutput, error) {
	s, p := reg.adminRequestRouteService()
	if p != nil {
		return nil, p
	}
	routes, err := s.ReorderRoutes(ctx, adminRequestViewer(ctx), mediarequests.MediaType(in.Body.MediaType), in.Body.IDs)
	if err != nil {
		return nil, requestProblem(err)
	}
	items := make([]AdminRequestRoute, 0, len(routes))
	for _, r := range routes {
		items = append(items, adminRequestRouteOf(r))
	}
	return &AdminRequestRouteCollectionOutput{Body: NewCollection(items)}, nil
}

func (reg *Registry) previewAdminRequestRoute(ctx context.Context, in *AdminRequestRoutePreviewInput) (*AdminRequestRoutePreviewOutput, error) {
	s, p := reg.adminRequestRouteService()
	if p != nil {
		return nil, p
	}
	requester := 0
	if in.Body.RequesterUserID != nil {
		id, err := strconv.Atoi(string(*in.Body.RequesterUserID))
		if err != nil || id <= 0 {
			return nil, NewProblem(TypeValidationFailed, "The request did not pass validation; see errors.").
				WithErrors(ProblemError{Location: "body.requester_user_id", Code: codeInvalid, Detail: "expected an account ID"})
		}
		requester = id
	}
	preview, err := s.PreviewRoute(ctx, adminRequestViewer(ctx), mediarequests.MediaType(in.Body.MediaType), in.Body.TMDBID, requester)
	if err != nil {
		return nil, requestProblem(err)
	}
	out := new(AdminRequestRoutePreviewOutput)
	f := preview.Facts
	out.Body.Facts = AdminRequestRouteFacts{
		GenreIDs: NonNil(f.GenreIDs), KeywordIDs: NonNil(f.KeywordIDs), OriginalLanguage: f.OriginalLanguage,
		OriginCountries: NonNil(f.OriginCountries), Year: f.Year, NetworkIDs: NonNil(f.NetworkIDs),
		CompanyIDs: NonNil(f.CompanyIDs), Anime: f.Anime,
	}
	out.Body.Tiers = make([]AdminRequestRoutePreviewTier, 0, len(preview.Tiers))
	for _, t := range preview.Tiers {
		out.Body.Tiers = append(out.Body.Tiers, AdminRequestRoutePreviewTier{
			Quality: string(t.Quality), RouteID: t.RouteID, RouteName: t.RouteName, IntegrationID: t.IntegrationID,
			IntegrationName: t.IntegrationName, Overrides: t.Overrides, Note: t.Reason,
		})
	}
	return out, nil
}
