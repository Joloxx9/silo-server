package apiv2

import (
	"context"
	"net/http"

	"github.com/Silo-Server/silo-server/internal/api/handlers"
	"github.com/Silo-Server/silo-server/internal/imagesize"
	"github.com/Silo-Server/silo-server/internal/models"
	mediarequests "github.com/Silo-Server/silo-server/internal/requests"
)

const opListRequestPersonCredits = "listRequestPersonCredits"

// MediaRequestPersonCreditsService lists a person's filmography with request
// state. The request service implements it; checked by type assertion so
// MediaRequestService fakes need not.
type MediaRequestPersonCreditsService interface {
	PersonCredits(ctx context.Context, viewer mediarequests.Viewer, personTMDBID int) ([]mediarequests.PersonCredit, error)
}

// CatalogPlayedService reports which catalog items the viewer has finished.
// *handlers.ItemsHandler implements it; checked by type assertion on
// CatalogAccess so fakes need not.
type CatalogPlayedService interface {
	PlayedContentIDs(ctx context.Context, v handlers.ItemViewer, items []*models.MediaItem) map[string]bool
}

// RequestPersonCreditsInput names a person by TMDB ID.
type RequestPersonCreditsInput struct {
	TMDBID int `path:"tmdb_id" minimum:"1" doc:"The person's TMDB identifier (external, not a Spark ID); a catalog person carries it as tmdb_id" example:"73968"`
}

// RequestPersonCredit is one title in a person's filmography.
type RequestPersonCredit struct {
	Media        RequestMediaResult `json:"media" doc:"The title as search shows it, with library availability and the viewer's request state"`
	Character    string             `json:"character,omitempty" doc:"The role played; several roles on one title are joined with ' / '. Empty for a crew-only credit" example:"August Walker"`
	Jobs         []string           `json:"jobs" doc:"Crew jobs on the title, such as Director or Screenplay. Empty, never null"`
	EpisodeCount int                `json:"episode_count,omitempty" doc:"Series: episodes the person is credited on, when TMDB says" example:"24"`
	Watched      bool               `json:"watched" doc:"The viewing profile has finished the library copy: a movie watched, or every episode of a series. False for a title not in the library" example:"false"`
}

// RequestPersonCredits is a person's filmography.
type RequestPersonCredits struct {
	Credits []RequestPersonCredit `json:"credits" doc:"Movies and series, announced (undated) titles first, then newest first. Adult titles, talk and news shows, and titles above the viewer's rating ceiling are left out. Empty, never null"`
}

// RequestPersonCreditsOutput is the listRequestPersonCredits response.
type RequestPersonCreditsOutput struct {
	Body RequestPersonCredits
}

func registerRequestPersonCredits(reg *Registry) {
	op := humaOp(http.MethodGet, Prefix+"/requests/people/{tmdb_id}/credits", opListRequestPersonCredits, requestsTag,
		"List a person's movies and series from TMDB, each with whether it is in the library and whether the viewer may request it.")
	op.Errors = []int{http.StatusNotFound, http.StatusConflict}
	Register(reg, Operation{Operation: op, Class: ClassProfileScoped, ServiceBacked: true}, reg.listRequestPersonCredits)
}

func (reg *Registry) listRequestPersonCredits(ctx context.Context, in *RequestPersonCreditsInput) (*RequestPersonCreditsOutput, error) {
	svc, viewer, p := reg.requestViewer(ctx)
	if p != nil {
		return nil, p
	}
	lister, ok := svc.(MediaRequestPersonCreditsService)
	if !ok {
		return nil, unavailable("requests")
	}
	credits, err := lister.PersonCredits(ctx, viewer, in.TMDBID)
	if err != nil {
		return nil, requestProblem(err)
	}
	media := make([]mediarequests.MediaResult, 0, len(credits))
	for _, credit := range credits {
		media = append(media, credit.Media)
	}
	results := requestMediaResultsOf(media)
	reg.markInWatchlist(ctx, watchlistMarksOf(results))
	played := reg.playedLibraryResults(ctx, results)
	out := RequestPersonCredits{Credits: make([]RequestPersonCredit, 0, len(credits))}
	for i, credit := range credits {
		jobs := credit.Jobs
		if jobs == nil {
			jobs = []string{}
		}
		out.Credits = append(out.Credits, RequestPersonCredit{
			Media: results[i], Character: credit.Character, Jobs: jobs, EpisodeCount: credit.EpisodeCount,
			Watched: results[i].LibraryContentID != "" && played[results[i].LibraryContentID],
		})
	}
	return &RequestPersonCreditsOutput{Body: out}, nil
}

// playedLibraryResults reads which in-library results the viewer has
// finished. It never fails the page: without a catalog seam or a viewer it
// reports nothing played.
func (reg *Registry) playedLibraryResults(ctx context.Context, results []RequestMediaResult) map[string]bool {
	svc, ok := reg.deps.CatalogAccess.(CatalogPlayedService)
	if !ok {
		return nil
	}
	items := make([]*models.MediaItem, 0, len(results))
	for _, r := range results {
		if r.LibraryContentID == "" {
			continue
		}
		itemType := "movie"
		if r.MediaType == string(mediarequests.MediaTypeSeries) {
			itemType = "series"
		}
		items = append(items, &models.MediaItem{ContentID: r.LibraryContentID, Type: itemType})
	}
	if len(items) == 0 {
		return nil
	}
	viewer, p := reg.itemViewer(ctx, imagesize.Request{}, "", "")
	if p != nil {
		return nil
	}
	return svc.PlayedContentIDs(ctx, viewer, items)
}
