package requests

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

// monitoredSeasonsTimeout bounds the Sonarr read on a title detail, so a slow
// or unreachable server only costs the detail its monitored seasons.
const monitoredSeasonsTimeout = 4 * time.Second

// monitoredSeasonsMaxBody caps a Sonarr series answer; one series is small.
const monitoredSeasonsMaxBody = 1 << 20

var monitoredSeasonsHTTPClient = &http.Client{Timeout: monitoredSeasonsTimeout}

// sonarrSeries is the part of a Sonarr series this server reads. The read is
// local to this package because arrclient imports it.
type sonarrSeries struct {
	Monitored bool `json:"monitored"`
	Seasons   []struct {
		SeasonNumber int  `json:"seasonNumber"`
		Monitored    bool `json:"monitored"`
	} `json:"seasons"`
}

// monitoredSeasons returns the seasons of the series with tvdbID that an
// enabled Sonarr connection already monitors, whoever added it there. A
// series the server does not monitor contributes nothing. Errors are logged
// and leave the answer to the servers that did reply.
func (s *Service) monitoredSeasons(ctx context.Context, tvdbID int) map[int]bool {
	if tvdbID <= 0 {
		return nil
	}
	integrations, err := s.store.ListIntegrations(ctx)
	if err != nil {
		slog.WarnContext(ctx, "requests: listing download servers for monitored seasons", "error", err)
		return nil
	}
	ctx, cancel := context.WithTimeout(ctx, monitoredSeasonsTimeout)
	defer cancel()
	monitored := map[int]bool{}
	for _, in := range integrations {
		if !isSonarrConnection(in) {
			continue
		}
		series, err := findSonarrSeries(ctx, in.BaseURL, in.APIKeyRef, tvdbID)
		if err != nil {
			slog.WarnContext(ctx, "requests: reading monitored seasons from sonarr", "server", in.Name, "tvdb_id", tvdbID, "error", err)
			continue
		}
		mergeMonitoredSeasons(monitored, series)
	}
	return monitored
}

// findSonarrSeries reads the series Sonarr already has for a TVDB id from
// Sonarr's own library; an unknown id answers an empty list.
func findSonarrSeries(ctx context.Context, baseURL, apiKey string, tvdbID int) ([]sonarrSeries, error) {
	url := fmt.Sprintf("%s/api/v3/series?tvdbId=%d", strings.TrimRight(strings.TrimSpace(baseURL), "/"), tvdbID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-Api-Key", strings.TrimSpace(apiKey))
	resp, err := monitoredSeasonsHTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, monitoredSeasonsMaxBody))
		return nil, fmt.Errorf("sonarr answered %d", resp.StatusCode)
	}
	var out []sonarrSeries
	if err := json.NewDecoder(io.LimitReader(resp.Body, monitoredSeasonsMaxBody)).Decode(&out); err != nil {
		return nil, fmt.Errorf("decoding sonarr series: %w", err)
	}
	return out, nil
}

// isSonarrConnection reports an enabled arr connection to a Sonarr server
// that has what a read needs.
func isSonarrConnection(in Integration) bool {
	kind, _ := in.PluginConfig["service_kind"].(string)
	return in.Enabled && in.CapabilityID == arrCapabilityID && kind == kindSonarr &&
		strings.TrimSpace(in.BaseURL) != "" && strings.TrimSpace(in.APIKeyRef) != ""
}

// mergeMonitoredSeasons adds the regular seasons a monitored series monitors.
func mergeMonitoredSeasons(into map[int]bool, series []sonarrSeries) {
	for _, s := range series {
		if !s.Monitored {
			continue
		}
		for _, season := range s.Seasons {
			if season.Monitored && season.SeasonNumber > 0 {
				into[season.SeasonNumber] = true
			}
		}
	}
}

// markMonitoredSeasons counts a season a download server already monitors
// as requested: asking for it again would only repeat what is coming.
func markMonitoredSeasons(seasons []RequestSeason, monitored map[int]bool) {
	for i := range seasons {
		if monitored[seasons[i].Number] {
			seasons[i].Requested = true
		}
	}
}
