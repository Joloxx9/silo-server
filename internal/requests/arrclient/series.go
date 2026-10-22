package arrclient

import (
	"context"
	"fmt"
)

// SeriesSeasonResource is one season of a Sonarr series.
type SeriesSeasonResource struct {
	SeasonNumber int  `json:"seasonNumber"`
	Monitored    bool `json:"monitored"`
}

// SeriesResource is the part of a Sonarr series this server reads.
type SeriesResource struct {
	ID        int                    `json:"id"`
	TvdbID    int                    `json:"tvdbId"`
	Monitored bool                   `json:"monitored"`
	Seasons   []SeriesSeasonResource `json:"seasons"`
}

// FindSeriesByTVDB returns the series Sonarr already has for a TVDB id. It
// reads Sonarr's own library, so an unknown id answers an empty list.
func FindSeriesByTVDB(ctx context.Context, client *Client, tvdbID int) ([]SeriesResource, error) {
	var out []SeriesResource
	if err := client.GetJSON(ctx, fmt.Sprintf("/api/v3/series?tvdbId=%d", tvdbID), &out); err != nil {
		return nil, err
	}
	return out, nil
}
