package requests

import (
	"context"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/Silo-Server/silo-server/internal/requests/arrclient"
)

// monitoredSeasonsTimeout bounds the Sonarr read on a title detail, so a slow
// or unreachable server only costs the detail its monitored seasons.
const monitoredSeasonsTimeout = 4 * time.Second

var monitoredSeasonsHTTPClient = &http.Client{Timeout: monitoredSeasonsTimeout}

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
		client := arrclient.New(in.BaseURL, in.APIKeyRef, monitoredSeasonsHTTPClient)
		series, err := arrclient.FindSeriesByTVDB(ctx, client, tvdbID)
		if err != nil {
			slog.WarnContext(ctx, "requests: reading monitored seasons from sonarr", "server", in.Name, "tvdb_id", tvdbID, "error", err)
			continue
		}
		mergeMonitoredSeasons(monitored, series)
	}
	return monitored
}

// isSonarrConnection reports an enabled arr connection to a Sonarr server
// that has what a read needs.
func isSonarrConnection(in Integration) bool {
	kind, _ := in.PluginConfig["service_kind"].(string)
	return in.Enabled && in.CapabilityID == "arr" && kind == "sonarr" &&
		strings.TrimSpace(in.BaseURL) != "" && strings.TrimSpace(in.APIKeyRef) != ""
}

// mergeMonitoredSeasons adds the regular seasons a monitored series monitors.
func mergeMonitoredSeasons(into map[int]bool, series []arrclient.SeriesResource) {
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
