package requests

import (
	"context"
	"errors"
	"fmt"

	"github.com/Silo-Server/silo-server/internal/metadata/tmdb"
)

// TMDBPersonCreditsClient lists a person's movies and series. Detected by type
// assertion on the service's TMDBClient, like TMDBCertificationClient.
type TMDBPersonCreditsClient interface {
	GetPersonCredits(ctx context.Context, personID int) ([]tmdb.PersonCredit, error)
}

// PersonCredit is one title in a person's filmography with the viewer's
// availability and request state, and what the person did on it.
type PersonCredit struct {
	Media        MediaResult
	Character    string
	Jobs         []string
	EpisodeCount int
}

// PersonCredits returns a person's filmography (by TMDB person ID) the way
// search shows titles: each with whether it is in the library and whether
// the viewer may request it. Titles above the viewer's rating ceiling are
// left out, as discovery leaves them out.
func (s *Service) PersonCredits(ctx context.Context, viewer Viewer, personTMDBID int) ([]PersonCredit, error) {
	if s == nil || s.store == nil || s.tmdb == nil {
		return nil, fmt.Errorf("request service is not configured")
	}
	ctx = withPolicyCache(ctx)
	if err := s.ensureRequestsEnabled(ctx); err != nil {
		return nil, err
	}
	if personTMDBID <= 0 {
		return nil, fmt.Errorf("%w: tmdb person id is required", ErrInvalidInput)
	}
	client, ok := s.tmdb.(TMDBPersonCreditsClient)
	if !ok {
		return nil, fmt.Errorf("requests: tmdb client cannot list person credits")
	}
	raw, err := client.GetPersonCredits(ctx, personTMDBID)
	if errors.Is(err, tmdb.ErrNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	page := &tmdb.MediaPage{Page: 1, TotalPages: 1, TotalResults: len(raw), Results: make([]tmdb.MediaResult, 0, len(raw))}
	roles := make(map[string]tmdb.PersonCredit, len(raw))
	for _, credit := range raw {
		page.Results = append(page.Results, credit.MediaResult)
		roles[creditKey(credit.MediaType, credit.ID)] = credit
	}
	ceiling, err := s.viewerContentCeiling(ctx, viewer)
	if err != nil {
		return nil, err
	}
	enriched, err := s.enrichPageWithCeiling(ctx, viewer, page, ceiling)
	if err != nil {
		return nil, err
	}
	out := make([]PersonCredit, 0, len(enriched.Results))
	for _, media := range enriched.Results {
		role := roles[creditKey(string(media.MediaType), media.TMDBID)]
		out = append(out, PersonCredit{
			Media:        media,
			Character:    role.Character,
			Jobs:         role.Jobs,
			EpisodeCount: role.EpisodeCount,
		})
	}
	return out, nil
}

// creditKey matches a TMDB credit to its enriched result; TMDB's "tv" and
// Silo's "series" name the same thing.
func creditKey(mediaType string, id int) string {
	if mediaType == "tv" {
		mediaType = string(MediaTypeSeries)
	}
	return fmt.Sprintf("%s:%d", mediaType, id)
}
