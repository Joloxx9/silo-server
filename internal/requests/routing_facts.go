package requests

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/Silo-Server/silo-server/internal/metadata/tmdb"
)

// RoutingFacts is what routing rules can match a request on. It is captured
// from TMDB when the request is created and stored with it, so rules evaluate
// the same way at approval, on another server, or after TMDB changes.
type RoutingFacts struct {
	GenreIDs         []int    `json:"genre_ids,omitempty"`
	KeywordIDs       []int    `json:"keyword_ids,omitempty"`
	OriginalLanguage string   `json:"original_language,omitempty"`
	OriginCountries  []string `json:"origin_countries,omitempty"`
	Year             int      `json:"year,omitempty"`
	// NetworkIDs are a series' networks; CompanyIDs a movie's studios.
	NetworkIDs []int `json:"network_ids,omitempty"`
	CompanyIDs []int `json:"company_ids,omitempty"`
	Anime      bool  `json:"anime,omitempty"`
	// ContentRating is the title's US rating ("PG", "TV-14"); "" when TMDB
	// has none, nil when it was never looked up (requests from before it
	// was captured).
	ContentRating *string `json:"content_rating,omitempty"`
	// CapturedAt is unset on requests from before capture, which is how
	// routing tells "no facts yet" from a title TMDB knows little about.
	CapturedAt *time.Time `json:"captured_at,omitempty"`
}

// Captured reports whether the facts were ever read from TMDB.
func (f RoutingFacts) Captured() bool { return f.CapturedAt != nil }

// routingFactsFrom reads the facts off a TMDB detail. A nil detail (TMDB
// unreachable) yields uncaptured facts, so routing retries the lookup later.
func routingFactsFrom(detail *tmdb.MediaDetail, now time.Time) RoutingFacts {
	if detail == nil {
		return RoutingFacts{}
	}
	return RoutingFacts{
		GenreIDs:         detail.GenreIDs,
		KeywordIDs:       detail.KeywordIDs,
		OriginalLanguage: detail.OriginalLanguage,
		OriginCountries:  detail.OriginCountries,
		Year:             detail.Year,
		NetworkIDs:       detail.NetworkIDs,
		CompanyIDs:       detail.CompanyIDs,
		Anime:            detectAnime(detail.KeywordIDs),
		ContentRating:    &detail.USCertification,
		CapturedAt:       &now,
	}
}

// requestDetail fetches the TMDB detail a request is checked and routed
// against. It returns nil when TMDB cannot answer, or when the service has no
// TMDB client (a service wired for another job).
func (s *Service) requestDetail(ctx context.Context, mediaType MediaType, tmdbID int) *tmdb.MediaDetail {
	if s.tmdb == nil {
		return nil
	}
	detail, err := s.tmdb.GetMediaDetail(ctx, tmdbMediaType(mediaType), tmdbID)
	if err != nil {
		return nil
	}
	return detail
}

func encodeRoutingFacts(facts RoutingFacts) ([]byte, error) {
	raw, err := json.Marshal(facts)
	if err != nil {
		return nil, fmt.Errorf("encode routing facts: %w", err)
	}
	return raw, nil
}

func decodeRoutingFacts(raw []byte) (RoutingFacts, error) {
	var facts RoutingFacts
	if len(raw) == 0 {
		return facts, nil
	}
	if err := json.Unmarshal(raw, &facts); err != nil {
		return RoutingFacts{}, fmt.Errorf("decode routing facts: %w", err)
	}
	return facts, nil
}
