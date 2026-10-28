package watchlist

import (
	"context"
	"log/slog"
	"time"

	"github.com/Silo-Server/silo-server/internal/watchsync"
)

// WatchSync adapts the service for watch provider sync, which keeps the
// titles a provider's watchlist lists and the library doesn't have as
// watchlist entries.
func (s *Titles) WatchSync() watchsync.WatchlistTitles {
	return syncTitles{titles: s}
}

type syncTitles struct{ titles *Titles }

var _ watchsync.WatchlistTitles = syncTitles{}

func (t syncTitles) LibraryHas(ctx context.Context, title watchsync.WatchlistTitle) (bool, error) {
	matches, err := t.titles.LibraryMatches(ctx, title.Kind, []int{title.TMDBID}, title.IMDbID, title.TVDBID)
	return len(matches) > 0, err
}

// AddWatchlistTitle stores the provider's IDs and display fields. Providers
// send no poster, so a new title is checked on TMDB right away rather than at
// its next scheduled check; a failed check leaves that schedule to retry.
func (t syncTitles) AddWatchlistTitle(ctx context.Context, userID int, profileID string, title watchsync.WatchlistTitle, addedAt time.Time) (bool, error) {
	if addedAt.IsZero() {
		addedAt = t.titles.now()
	}
	entry, inserted, err := t.titles.AddSnapshot(ctx, Viewer{UserID: userID, ProfileID: profileID}, Snapshot{
		MediaType: title.Kind,
		TMDBID:    title.TMDBID,
		IMDbID:    title.IMDbID,
		TVDBID:    title.TVDBID,
		Title:     title.Title,
		Year:      title.Year,
	}, addedAt)
	if err != nil {
		return false, err
	}
	if inserted && entry.Title.PosterPath == "" && t.titles.tmdb != nil {
		if err := t.titles.checkTitle(ctx, entry.Title.ID); err != nil && ctx.Err() == nil {
			slog.WarnContext(ctx, "watchlist title check after a provider import failed",
				"component", "watchlist", "title_id", entry.Title.ID, "error", err)
		}
	}
	return inserted, nil
}

// ListWatchlistTitles returns every entry of the profile outside the library.
func (t syncTitles) ListWatchlistTitles(ctx context.Context, userID int, profileID string) ([]watchsync.WatchlistTitle, error) {
	const pageSize = 500
	var out []watchsync.WatchlistTitle
	var after *PageKey
	for {
		entries, err := t.titles.repo.listPage(ctx, userID, profileID, after, pageSize)
		if err != nil {
			return nil, err
		}
		for _, e := range entries {
			out = append(out, watchsync.WatchlistTitle{
				Kind:    e.Title.MediaType,
				TMDBID:  e.Title.TMDBID,
				IMDbID:  e.Title.IMDbID,
				TVDBID:  e.Title.TVDBID,
				Title:   e.Title.Title,
				Year:    e.Title.Year,
				AddedAt: e.AddedAt,
			})
		}
		if len(entries) < pageSize {
			return out, nil
		}
		key := entries[len(entries)-1].PageKey()
		after = &key
	}
}

func (t syncTitles) RemoveWatchlistTitle(ctx context.Context, userID int, profileID string, title watchsync.WatchlistTitle) error {
	_, _, err := t.titles.Remove(ctx, Viewer{UserID: userID, ProfileID: profileID}, title.Kind, title.TMDBID)
	return err
}

func (t syncTitles) WatchlistTitleListed(ctx context.Context, userID int, profileID string, title watchsync.WatchlistTitle) (bool, error) {
	key := TitleKey{MediaType: title.Kind, TMDBID: title.TMDBID}
	on, err := t.titles.OnWatchlist(ctx, Viewer{UserID: userID, ProfileID: profileID}, []TitleKey{key})
	return on[key], err
}
