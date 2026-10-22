package requests

import (
	"testing"

	"github.com/Silo-Server/silo-server/internal/requests/arrclient"
)

func TestMergeMonitoredSeasons(t *testing.T) {
	got := map[int]bool{}
	mergeMonitoredSeasons(got, []arrclient.SeriesResource{
		{Monitored: true, Seasons: []arrclient.SeriesSeasonResource{
			{SeasonNumber: 0, Monitored: true},
			{SeasonNumber: 1, Monitored: false},
			{SeasonNumber: 2, Monitored: true},
		}},
		// An unmonitored series wants nothing, whatever its seasons say.
		{Monitored: false, Seasons: []arrclient.SeriesSeasonResource{{SeasonNumber: 3, Monitored: true}}},
	})
	if len(got) != 1 || !got[2] {
		t.Fatalf("monitored = %v, want only season 2", got)
	}
}

func TestMarkMonitoredSeasons(t *testing.T) {
	seasons := []RequestSeason{{Number: 1}, {Number: 2}, {Number: 3, Requested: true}}
	markMonitoredSeasons(seasons, map[int]bool{2: true})
	if seasons[0].Requested || !seasons[1].Requested || !seasons[2].Requested {
		t.Fatalf("seasons = %+v, want 2 marked and 3 kept", seasons)
	}
}

func TestIsSonarrConnection(t *testing.T) {
	sonarr := Integration{Enabled: true, CapabilityID: "arr", BaseURL: "http://sonarr:8989", APIKeyRef: "key",
		PluginConfig: map[string]any{"service_kind": "sonarr"}}
	if !isSonarrConnection(sonarr) {
		t.Fatal("enabled Sonarr connection not recognized")
	}
	radarr := sonarr
	radarr.PluginConfig = map[string]any{"service_kind": "radarr"}
	disabled := sonarr
	disabled.Enabled = false
	keyless := sonarr
	keyless.APIKeyRef = ""
	for name, in := range map[string]Integration{"radarr": radarr, "disabled": disabled, "keyless": keyless} {
		if isSonarrConnection(in) {
			t.Fatalf("%s counted as a Sonarr connection", name)
		}
	}
}
