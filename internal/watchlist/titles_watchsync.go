package watchlist

import (
	"context"
	"log/slog"
	"slices"
	"time"

	"github.com/Silo-Server/silo-server/internal/watchsync"
)

// SyncRequests applies watchlist requests to the entries watch provider sync
// adds and removes, as adding or removing a title by hand does: a new entry
// requests the title or follows its request, and a removed one withdraws the
// request it made while nothing has been sent. Failures are the
// implementation's to log; the entry stays either way.
type SyncRequests interface {
	RequestWatchlistTitle(ctx context.Context, userID int, profileID string, title Title)
	WithdrawWatchlistTitle(ctx context.Context, userID int, profileID string, mediaType string, tmdbIDs []int)
	// RequestQueuedWatchlistTitles requests, in order, the titles no request
	// was ever made for, while the profile's quota has room.
	RequestQueuedWatchlistTitles(ctx context.Context, userID int, profileID string, titles []Title)
}

// WatchSync adapts the service for watch provider sync, which keeps the
// titles a provider's watchlist lists and the library doesn't have as
// watchlist entries. requests may be nil, which leaves requests alone.
func (s *Titles) WatchSync(requests SyncRequests) watchsync.WatchlistTitles {
	return syncTitles{titles: s, requests: requests}
}

type syncTitles struct {
	titles   *Titles
	requests SyncRequests
}

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
	if !inserted {
		return false, nil
	}
	if entry.Title.PosterPath == "" && t.titles.tmdb != nil {
		if err := t.titles.checkTitle(ctx, entry.Title.ID); err != nil && ctx.Err() == nil {
			slog.WarnContext(ctx, "watchlist title check after a provider import failed",
				"component", "watchlist", "title_id", entry.Title.ID, "error", err)
		}
	}
	if t.requests != nil {
		// Request with the snapshot the check just refreshed.
		stored := &entry.Title
		if found, err := t.titles.Find(ctx, title.Kind, title.TMDBID); err == nil && found != nil {
			stored = found
		}
		t.requests.RequestWatchlistTitle(ctx, userID, profileID, *stored)
	}
	return true, nil
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

// RequestQueuedWatchlistTitles hands the stored titles, with the snapshots
// their checks keep current, to the request service.
func (t syncTitles) RequestQueuedWatchlistTitles(ctx context.Context, userID int, profileID string, titles []watchsync.WatchlistTitle) error {
	if t.requests == nil || len(titles) == 0 {
		return nil
	}
	stored := make([]Title, 0, len(titles))
	for _, title := range titles {
		found, err := t.titles.Find(ctx, title.Kind, title.TMDBID)
		if err != nil {
			return err
		}
		if found != nil {
			stored = append(stored, *found)
		}
	}
	t.requests.RequestQueuedWatchlistTitles(ctx, userID, profileID, stored)
	return nil
}

// RemoveWatchlistTitle withdraws the entry's request after removing it, under
// every TMDB ID the title has held, since a request keeps the ID it was made
// under.
func (t syncTitles) RemoveWatchlistTitle(ctx context.Context, userID int, profileID string, title watchsync.WatchlistTitle) error {
	removed, _, err := t.titles.Remove(ctx, Viewer{UserID: userID, ProfileID: profileID}, title.Kind, title.TMDBID)
	if err != nil {
		return err
	}
	if t.requests != nil {
		ids := []int{title.TMDBID}
		if removed != nil {
			for _, id := range append([]int{removed.TMDBID}, removed.FormerTMDBIDs...) {
				if !slices.Contains(ids, id) {
					ids = append(ids, id)
				}
			}
		}
		t.requests.WithdrawWatchlistTitle(ctx, userID, profileID, title.Kind, ids)
	}
	return nil
}

func (t syncTitles) WatchlistTitleListed(ctx context.Context, userID int, profileID string, title watchsync.WatchlistTitle) (bool, error) {
	key := TitleKey{MediaType: title.Kind, TMDBID: title.TMDBID}
	on, err := t.titles.OnWatchlist(ctx, Viewer{UserID: userID, ProfileID: profileID}, []TitleKey{key})
	return on[key], err
}
