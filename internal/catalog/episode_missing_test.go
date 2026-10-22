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

// Specials never show what the library lacks.
func TestMissingPredicatesKeepSpecialsLibraryOnly(t *testing.T) {
	if !strings.Contains(episodeOrMissingPredicate, "episodes.season_number > 0 OR") {
		t.Fatalf("episodeOrMissingPredicate = %q", episodeOrMissingPredicate)
	}
	if !strings.HasPrefix(missingEpisodePredicate, "episodes.season_number > 0 AND NOT") {
		t.Fatalf("missingEpisodePredicate = %q", missingEpisodePredicate)
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
