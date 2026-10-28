package watchlist

import (
	"context"
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

// AddWatchlistTitle stores the provider's IDs and display fields. A new
// title has no poster until its first TMDB check fills the snapshot in.
func (t syncTitles) AddWatchlistTitle(ctx context.Context, userID int, profileID string, title watchsync.WatchlistTitle, addedAt time.Time) (bool, error) {
	if addedAt.IsZero() {
		addedAt = t.titles.now()
	}
	_, inserted, err := t.titles.AddSnapshot(ctx, Viewer{UserID: userID, ProfileID: profileID}, Snapshot{
		MediaType: title.Kind,
		TMDBID:    title.TMDBID,
		IMDbID:    title.IMDbID,
		TVDBID:    title.TVDBID,
		Title:     title.Title,
		Year:      title.Year,
	}, addedAt)
	return inserted, err
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
