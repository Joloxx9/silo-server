package apiv2

import (
	"context"
	"net/http"

	mediarequests "github.com/Silo-Server/silo-server/internal/requests"
)

const opListRequestPersonCredits = "listRequestPersonCredits"

// MediaRequestPersonCreditsService lists a person's filmography with request
// state. The request service implements it; checked by type assertion so
// MediaRequestService fakes need not.
type MediaRequestPersonCreditsService interface {
	PersonCredits(ctx context.Context, viewer mediarequests.Viewer, personTMDBID int) ([]mediarequests.PersonCredit, error)
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
	out := RequestPersonCredits{Credits: make([]RequestPersonCredit, 0, len(credits))}
	for i, credit := range credits {
		jobs := credit.Jobs
		if jobs == nil {
			jobs = []string{}
		}
		out.Credits = append(out.Credits, RequestPersonCredit{
			Media: results[i], Character: credit.Character, Jobs: jobs, EpisodeCount: credit.EpisodeCount,
		})
	}
	return &RequestPersonCreditsOutput{Body: out}, nil
}
