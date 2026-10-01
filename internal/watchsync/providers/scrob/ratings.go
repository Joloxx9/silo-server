package scrob

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/Silo-Server/silo-server/internal/historyimport"
	"github.com/Silo-Server/silo-server/internal/watchsync"
)

// Scrob rates on a 0-10 float scale; Silo's provider scale is the same range
// rounded to an integer, so ratings pass through with simple rounding.

type scrobRatingEntry struct {
	Media   scrobMedia `json:"media"`
	Rating  float64    `json:"rating"`
	RatedAt time.Time  `json:"rated_at"`
}

type scrobRatingsResponse struct {
	Results []scrobRatingEntry `json:"results"`
}

// FetchRatings reads GET /ratings, which Scrob returns as a single
// unpaginated list — every rating the account has, movies, shows, seasons,
// and episodes together. That makes the read a true snapshot for movies and
// series, unlike providers that page and can be caught mid-change.
//
// The endpoint only ever reports tmdb_id on the rated item (not imdb_id or
// tvdb_id, even though Scrob stores both), so a TVDB-only rated show cannot
// be matched here and is counted as unidentified.
func (p *Provider) FetchRatings(ctx context.Context, _ watchsync.ServerConfig, conn watchsync.Connection) (watchsync.RatingImportBatch, error) {
	serverURL, err := p.connectionServerURL(conn)
	if err != nil {
		return watchsync.RatingImportBatch{}, err
	}
	var payload scrobRatingsResponse
	if err := p.do(ctx, http.MethodGet, serverURL, conn.AccessToken, "/ratings", nil, &payload); err != nil {
		return watchsync.RatingImportBatch{}, err
	}

	var rows []watchsync.RemoteRating
	unidentified := map[string]int{}
	for _, entry := range payload.Results {
		kind := entry.Media.Type
		if kind != historyimport.KindMovie && kind != historyimport.KindSeries {
			// Season and episode ratings: Silo only rates movies and series.
			continue
		}
		value := providerRating(entry.Rating)
		if value == 0 {
			continue
		}
		key := ratingItemKey(kind, entry.Media)
		if key == "" {
			unidentified[kind]++
			continue
		}
		rows = append(rows, watchsync.RemoteRating{
			RemoteFavorite: watchsync.RemoteFavorite{
				Provider:        p.Key(),
				ProviderItemKey: key,
				Kind:            kind,
				Title:           entry.Media.Title,
				TMDBID:          intString(entry.Media.TMDBID),
			},
			Rating:  value,
			RatedAt: entry.RatedAt,
		})
	}

	batch := watchsync.RatingImportBatch{Rows: rows}
	for _, kind := range []string{historyimport.KindMovie, historyimport.KindSeries} {
		if n := unidentified[kind]; n > 0 {
			batch.Warnings = append(batch.Warnings, fmt.Sprintf(
				"scrob returned %d %s ratings without a TMDB id; skipped %s rating removals", n, kind, kind))
			continue
		}
		batch.SnapshotKinds = append(batch.SnapshotKinds, kind)
	}
	return batch, nil
}

func ratingItemKey(kind string, media scrobMedia) string {
	if kind == historyimport.KindSeries {
		return showKey(media.TMDBID, media.TVDBID, media.IMDbID)
	}
	return movieKey(media.TMDBID, media.TVDBID, media.IMDbID)
}

func showKey(tmdbID, tvdbID int, imdbID string) string {
	switch {
	case tvdbID > 0:
		return "tvdb:" + strconv.Itoa(tvdbID)
	case tmdbID > 0:
		return "tmdb:" + strconv.Itoa(tmdbID)
	case imdbID != "":
		return "imdb:" + imdbID
	default:
		return ""
	}
}

// providerRating rounds a rating to the integer scale; 0 means unrated.
func providerRating(rating float64) int {
	return int(math.Round(rating))
}

const errRatingNeedsExternalID = "Scrob rating sync requires a movie or series with a TMDB or TVDB id"

type scrobRatingIn struct {
	TMDBID    int     `json:"tmdb_id,omitempty"`
	TVDBID    int     `json:"tvdb_id,omitempty"`
	MediaType string  `json:"media_type"`
	Rating    float64 `json:"rating"`
}

// ExportRatings sets movie and show ratings with one POST /ratings per item:
// Scrob has no bulk ratings-write endpoint. Scrob replaces an existing
// rating, so resending one is harmless.
func (p *Provider) ExportRatings(ctx context.Context, _ watchsync.ServerConfig, conn watchsync.Connection, items []watchsync.LocalRating) (watchsync.ExportResult, error) {
	serverURL, err := p.connectionServerURL(conn)
	if err != nil {
		return watchsync.ExportResult{}, err
	}
	result := watchsync.ExportResult{Failed: make(map[string]string)}
	for _, item := range items {
		if item.Kind != historyimport.KindMovie && item.Kind != historyimport.KindSeries {
			result.Failed[item.MediaItemID] = errRatingNeedsExternalID
			continue
		}
		tmdbID, tvdbID := parseInt(item.TMDBID), parseInt(item.TVDBID)
		if tmdbID <= 0 && tvdbID <= 0 {
			result.Failed[item.MediaItemID] = errRatingNeedsExternalID
			continue
		}
		if item.Rating < 1 || item.Rating > 10 {
			result.Failed[item.MediaItemID] = "Scrob ratings must be from 1 to 10"
			continue
		}
		body := scrobRatingIn{TMDBID: tmdbID, TVDBID: tvdbID, MediaType: item.Kind, Rating: float64(item.Rating)}
		var encoded bytes.Buffer
		if err := json.NewEncoder(&encoded).Encode(body); err != nil {
			return watchsync.ExportResult{}, fmt.Errorf("encode scrob rating: %w", err)
		}
		if err := p.do(ctx, http.MethodPost, serverURL, conn.AccessToken, "/ratings", &encoded, nil); err != nil {
			result.Failed[item.MediaItemID] = err.Error()
			continue
		}
		result.Sent = append(result.Sent, item.MediaItemID)
	}
	if len(result.Failed) == 0 {
		result.Failed = nil
	}
	return result, nil
}

// RemoveRatings clears movie and show ratings with one DELETE /ratings per
// item. A title Scrob has no rating for 404s; that is folded into NotFound
// rather than Failed so a removal batch reconciles cleanly.
func (p *Provider) RemoveRatings(ctx context.Context, _ watchsync.ServerConfig, conn watchsync.Connection, items []watchsync.LocalFavorite) (watchsync.ExportResult, error) {
	serverURL, err := p.connectionServerURL(conn)
	if err != nil {
		return watchsync.ExportResult{}, err
	}
	result := watchsync.ExportResult{Failed: make(map[string]string)}
	for _, item := range items {
		if item.Kind != historyimport.KindMovie && item.Kind != historyimport.KindSeries {
			result.Failed[item.MediaItemID] = errRatingNeedsExternalID
			continue
		}
		tmdbID, tvdbID := parseInt(item.TMDBID), parseInt(item.TVDBID)
		if tmdbID <= 0 && tvdbID <= 0 {
			result.Failed[item.MediaItemID] = errRatingNeedsExternalID
			continue
		}
		query := url.Values{"media_type": {item.Kind}}
		if tmdbID > 0 {
			query.Set("tmdb_id", strconv.Itoa(tmdbID))
		}
		if tvdbID > 0 {
			query.Set("tvdb_id", strconv.Itoa(tvdbID))
		}
		path := "/ratings?" + query.Encode()
		err := p.do(ctx, http.MethodDelete, serverURL, conn.AccessToken, path, nil, nil)
		switch {
		case err == nil:
			result.Sent = append(result.Sent, item.MediaItemID)
		case isNotFound(err):
			result.NotFound = append(result.NotFound, item.MediaItemID)
		default:
			result.Failed[item.MediaItemID] = err.Error()
		}
	}
	if len(result.Failed) == 0 {
		result.Failed = nil
	}
	return result, nil
}
