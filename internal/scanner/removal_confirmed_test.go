package scanner

import (
	"fmt"
	"testing"
	"time"

	"github.com/Silo-Server/silo-server/internal/models"
)

// A file arr confirmed deleting skips the removal grace on the next sweep,
// while a file that merely went missing still waits it out, and a confirmed
// file that is seen on disk again is restored like any other missing file.
func TestConfirmFilesRemovedSkipsGracePostgres(t *testing.T) {
	pool := newDeadRootTestPool(t)
	ctx := t.Context()
	folderID := seedDeadRootTestFolder(t, pool, "series", "Removal confirmed sweep")
	repo := NewFileRepository(pool)
	base := fmt.Sprintf("/removal-confirmed-%d/Show/Season 01", time.Now().UnixNano())

	upsert := func(name string) *models.MediaFile {
		t.Helper()
		row, err := repo.Upsert(ctx, models.MediaFile{
			MediaFolderID: folderID, FilePath: base + "/" + name, FileSize: 1024,
			SubtitleTracks: []models.SubtitleTrack{}, ExternalSubtitles: []models.ExternalSubtitle{},
		})
		if err != nil {
			t.Fatalf("upsert %s: %v", name, err)
		}
		return row
	}
	exists := func(id int) bool {
		t.Helper()
		var ok bool
		if err := pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM media_files WHERE id = $1)`, id).Scan(&ok); err != nil {
			t.Fatal(err)
		}
		return ok
	}

	replaced := upsert("S01E01 old.mkv")
	vanished := upsert("S01E02.mkv")
	returned := upsert("S01E03.mkv")
	if err := repo.MarkMissing(ctx, vanished.ID, time.Now().UTC().Add(-time.Hour)); err != nil {
		t.Fatal(err)
	}

	confirmed, err := repo.ConfirmFilesRemoved(ctx, []string{replaced.FilePath, returned.FilePath, base + "/not-cataloged.mkv"})
	if err != nil {
		t.Fatalf("ConfirmFilesRemoved: %v", err)
	}
	if confirmed != 2 {
		t.Fatalf("confirmed = %d, want 2", confirmed)
	}

	// The confirmed file comes back before the sweep: seeing it clears both
	// the missing mark and the confirmation.
	if row := upsert("S01E03.mkv"); row.MissingSince != nil {
		t.Fatalf("returned file still missing: %v", row.MissingSince)
	}

	deleted, err := repo.DeleteMissingByFolder(ctx, folderID, 24*time.Hour, nil)
	if err != nil {
		t.Fatalf("DeleteMissingByFolder: %v", err)
	}
	if deleted != 1 || exists(replaced.ID) {
		t.Fatalf("deleted = %d, replaced row exists = %v; want only the confirmed row deleted", deleted, exists(replaced.ID))
	}
	if !exists(vanished.ID) {
		t.Fatal("unconfirmed missing row was deleted inside the grace period")
	}
	if !exists(returned.ID) {
		t.Fatal("confirmed row that was seen again was deleted")
	}

	// A later outage marks the returned file missing again; its old
	// confirmation must not let it skip the grace.
	if err := repo.MarkMissing(ctx, returned.ID, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	if deleted, err = repo.DeleteMissingByFolder(ctx, folderID, 24*time.Hour, nil); err != nil || deleted != 0 {
		t.Fatalf("second sweep deleted = %d, err = %v; want 0", deleted, err)
	}

	// A confirmed row holding a removed episode's first-seen date waits for
	// the replacement file to claim it; deleting it would make the episode
	// look newly added.
	holding := upsert("S01E05.mkv")
	if _, err := repo.ConfirmFilesRemoved(ctx, []string{holding.FilePath}); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE media_files SET held_episode_first_seen_at = NOW() - interval '30 days' WHERE id = $1`, holding.ID); err != nil {
		t.Fatal(err)
	}
	if deleted, err = repo.DeleteMissingByFolder(ctx, folderID, 24*time.Hour, nil); err != nil || deleted != 0 {
		t.Fatalf("holding sweep deleted = %d, err = %v; want 0", deleted, err)
	}
	if _, err := pool.Exec(ctx, `UPDATE media_files SET held_episode_first_seen_at = NULL WHERE id = $1`, holding.ID); err != nil {
		t.Fatal(err)
	}
	if deleted, err = repo.DeleteMissingByFolder(ctx, folderID, 24*time.Hour, nil); err != nil || deleted != 1 || exists(holding.ID) {
		t.Fatalf("sweep after the hold was claimed deleted = %d, err = %v; want the row deleted", deleted, err)
	}

	// Protected roots still win over a confirmation.
	protectedFile := upsert("S01E04.mkv")
	if _, err := repo.ConfirmFilesRemoved(ctx, []string{protectedFile.FilePath}); err != nil {
		t.Fatal(err)
	}
	if deleted, err = repo.DeleteMissingByFolder(ctx, folderID, 24*time.Hour, []string{base}); err != nil || deleted != 0 {
		t.Fatalf("protected sweep deleted = %d, err = %v; want 0", deleted, err)
	}
}
