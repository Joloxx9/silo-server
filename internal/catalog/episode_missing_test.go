package catalog

import (
	"strings"
	"testing"
	"time"

	"github.com/Silo-Server/silo-server/internal/models"
)

func TestEpisodeAvailability(t *testing.T) {
	now := time.Date(2026, 10, 22, 12, 0, 0, 0, time.UTC)
	past := now.Add(-24 * time.Hour)
	future := now.Add(24 * time.Hour)
	tests := []struct {
		name      string
		inLibrary bool
		airDate   *time.Time
		want      string
	}{
		{"in library", true, &past, ""},
		{"in library before airing", true, &future, ""},
		{"aired, no file", false, &past, models.EpisodeMissing},
		{"not aired yet", false, &future, models.EpisodeUnaired},
		{"no air date", false, nil, models.EpisodeUnaired},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := episodeAvailability(tt.inLibrary, tt.airDate, now); got != tt.want {
				t.Fatalf("episodeAvailability = %q, want %q", got, tt.want)
			}
		})
	}
}

// Specials and episodes still to air never show as missing.
func TestMissingPredicatesTakeOnlyAiredRegularEpisodes(t *testing.T) {
	for name, predicate := range map[string]string{
		"episodeOrMissingPredicate": episodeOrMissingPredicate,
		"missingEpisodePredicate":   missingEpisodePredicate,
	} {
		if !strings.Contains(predicate, "episodes.season_number > 0 AND episodes.air_date <= now()") {
			t.Fatalf("%s = %q", name, predicate)
		}
	}
	if !strings.HasPrefix(episodeOrMissingPredicate, "((") || !strings.Contains(episodeOrMissingPredicate, ") OR EXISTS") {
		t.Fatalf("episodeOrMissingPredicate must keep library episodes: %q", episodeOrMissingPredicate)
	}
}

func TestInLibrary(t *testing.T) {
	held := &models.Episode{ContentID: "held"}
	missing := &models.Episode{ContentID: "missing", Availability: models.EpisodeMissing}
	unaired := &models.Episode{ContentID: "unaired", Availability: models.EpisodeUnaired}
	got := InLibrary([]*models.Episode{held, missing, nil, unaired})
	if len(got) != 1 || got[0] != held {
		t.Fatalf("InLibrary = %v, want only the held episode", got)
	}
}
