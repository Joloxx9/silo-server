package watchsync

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/Silo-Server/silo-server/internal/historyimport"
	"github.com/Silo-Server/silo-server/internal/userstore"
)

// A provider's watchlist can list titles the library doesn't have. Silo keeps
// those as watchlist entries outside the library (the watchlist's unavailable
// side), so the import keeps them there too instead of skipping them. Their
// shadow rows carry a synthetic media item id naming the title, which no
// library item can hold, so the shared list pipeline tracks them like any
// other row: an entry added in Silo is exported, a provider removal takes the
// entry off, and a local removal goes to the provider when removals sync.

// listTitlePrefix starts the media item id of a shadow row for a watchlist
// entry outside the library: "watchlist-title:<kind>:<tmdb id>".
const listTitlePrefix = "watchlist-title:"

// WatchlistTitle names a movie or series on a watchlist by its TMDB ID, with
// the other IDs and display fields the provider reported.
type WatchlistTitle struct {
	Kind   string // historyimport.KindMovie or historyimport.KindSeries
	TMDBID int
	IMDbID string
	TVDBID int
	Title  string
	Year   int
	// AddedAt is when the entry joined the watchlist, on a listed entry.
	AddedAt time.Time
}

// WatchlistTitles keeps the watchlist entries for titles the library doesn't
// have. *watchlist.Titles provides it through WatchSync.
type WatchlistTitles interface {
	// LibraryHas reports whether any library item carries the title's IDs.
	LibraryHas(ctx context.Context, title WatchlistTitle) (bool, error)
	// AddWatchlistTitle keeps the title as the profile's watchlist entry,
	// keeping an existing entry's added_at. It reports whether it created one.
	AddWatchlistTitle(ctx context.Context, userID int, profileID string, title WatchlistTitle, addedAt time.Time) (bool, error)
	// RemoveWatchlistTitle takes the profile's entry for the title off.
	RemoveWatchlistTitle(ctx context.Context, userID int, profileID string, title WatchlistTitle) error
	// ListWatchlistTitles returns every entry of the profile.
	ListWatchlistTitles(ctx context.Context, userID int, profileID string) ([]WatchlistTitle, error)
	// WatchlistTitleListed reports whether the profile has an entry for the
	// title.
	WatchlistTitleListed(ctx context.Context, userID int, profileID string, title WatchlistTitle) (bool, error)
}

type watchlistTitlesRef struct{ WatchlistTitles }

// SetWatchlistTitles wires the watchlist entries for titles the library
// doesn't have. It may run after syncs started; until then, a watchlist
// import skips those titles as unmatched.
func (s *Service) SetWatchlistTitles(titles WatchlistTitles) {
	if s == nil {
		return
	}
	if titles == nil {
		s.titles.Store(nil)
		return
	}
	s.titles.Store(&watchlistTitlesRef{titles})
}

func (s *Service) watchlistTitles(b listBinding) WatchlistTitles {
	if b.kind != ListKindWatchlist {
		return nil
	}
	if ref := s.titles.Load(); ref != nil {
		return ref.WatchlistTitles
	}
	return nil
}

func (t WatchlistTitle) listItemID() string {
	return listTitlePrefix + t.Kind + ":" + strconv.Itoa(t.TMDBID)
}

// watchlistTitleFromRow returns the title a provider row names when it is a
// movie or series with a TMDB ID.
func watchlistTitleFromRow(row RemoteFavorite) (WatchlistTitle, bool) {
	if row.Kind != historyimport.KindMovie && row.Kind != historyimport.KindSeries {
		return WatchlistTitle{}, false
	}
	tmdbID, err := strconv.Atoi(strings.TrimSpace(row.TMDBID))
	if err != nil || tmdbID <= 0 {
		return WatchlistTitle{}, false
	}
	tvdbID, _ := strconv.Atoi(strings.TrimSpace(row.TVDBID))
	return WatchlistTitle{
		Kind:   row.Kind,
		TMDBID: tmdbID,
		IMDbID: strings.TrimSpace(row.IMDbID),
		TVDBID: max(tvdbID, 0),
		Title:  row.Title,
		Year:   row.Year,
	}, true
}

// watchlistTitleFromListItemID parses a shadow row's synthetic media item id.
func watchlistTitleFromListItemID(id string) (WatchlistTitle, bool) {
	rest, ok := strings.CutPrefix(id, listTitlePrefix)
	if !ok {
		return WatchlistTitle{}, false
	}
	kind, tmdb, ok := strings.Cut(rest, ":")
	if !ok || (kind != historyimport.KindMovie && kind != historyimport.KindSeries) {
		return WatchlistTitle{}, false
	}
	tmdbID, err := strconv.Atoi(tmdb)
	if err != nil || tmdbID <= 0 {
		return WatchlistTitle{}, false
	}
	return WatchlistTitle{Kind: kind, TMDBID: tmdbID}, true
}

// importListTitle keeps a row that matched no library item as a watchlist
// entry outside the library, when it names a title with a TMDB ID that no
// library item carries. A title some library item carries is left unmatched:
// the matcher found it ambiguous, and an entry outside the library would
// show it as missing. It returns the row's shadow state, or nil when the row
// stays unmatched or waits for a local removal to reach the provider.
func (s *Service) importListTitle(
	ctx context.Context,
	conn Connection,
	b listBinding,
	row RemoteFavorite,
	key string,
	removalsPending map[string]time.Time,
) (state *ListItemState, kept, added bool, err error) {
	titles := s.watchlistTitles(b)
	if titles == nil {
		return nil, false, false, nil
	}
	title, ok := watchlistTitleFromRow(row)
	if !ok {
		return nil, false, false, nil
	}
	inLibrary, err := titles.LibraryHas(ctx, title)
	if err != nil || inLibrary {
		return nil, false, false, err
	}
	id := title.listItemID()
	// As for library items: the provider listing the title again after the
	// local removal is a newer add; an older or undated listing is the one
	// the removal will take off.
	if removedAt, pending := removalsPending[id]; pending &&
		(row.FavoritedAtGuessed || !row.FavoritedAt.After(removedAt)) {
		return nil, true, false, nil
	}
	added, err = titles.AddWatchlistTitle(ctx, conn.UserID, conn.ProfileID, title, row.FavoritedAt)
	if err != nil {
		return nil, false, false, err
	}
	listedAt := row.FavoritedAt
	return &ListItemState{
		ConnectionID:     conn.ID,
		ListKind:         b.kind,
		MediaItemID:      id,
		ProviderItemKey:  key,
		Kind:             row.Kind,
		Title:            row.Title,
		Year:             row.Year,
		RemotePresent:    true,
		LocalPresent:     true,
		LastSeenRemoteAt: &listedAt,
		LastSeenLocalAt:  &listedAt,
	}, true, added, nil
}

// removeLocalListItem takes a shadow row's item off the local list: a
// watchlist entry outside the library by its title, a library item through
// the binding.
func (s *Service) removeLocalListItem(ctx context.Context, store userstore.UserStore, conn Connection, b listBinding, mediaItemID string) error {
	if title, ok := watchlistTitleFromListItemID(mediaItemID); ok {
		titles := s.watchlistTitles(b)
		if titles == nil {
			return nil
		}
		return titles.RemoveWatchlistTitle(ctx, conn.UserID, conn.ProfileID, title)
	}
	return b.localRemove(ctx, store, conn.ProfileID, mediaItemID)
}

// reconcileLocalListTitles records which imported watchlist entries outside
// the library left the profile's watchlist. One the profile removed is a
// local removal, which goes to the provider when removals sync. One that
// left because the library got the title moved onto the library watchlist
// and is still listed; its row retires without a removal, and the library
// item's own row takes over at the next import.
func (s *Service) reconcileLocalListTitles(ctx context.Context, conn Connection, b listBinding) error {
	titles := s.watchlistTitles(b)
	if titles == nil {
		return nil
	}
	states, err := s.repo.ListListItemStates(ctx, conn.ID, b.kind)
	if err != nil {
		return err
	}
	now := s.now()
	for _, state := range states {
		if !state.LocalPresent {
			continue
		}
		title, ok := watchlistTitleFromListItemID(state.MediaItemID)
		if !ok {
			continue
		}
		listed, err := titles.WatchlistTitleListed(ctx, conn.UserID, conn.ProfileID, title)
		if err != nil {
			return err
		}
		if listed {
			continue
		}
		promoted, err := titles.LibraryHas(ctx, title)
		if err != nil {
			return err
		}
		if err := s.repo.MarkListItemLocalRemoved(ctx, conn.ID, b.kind, state.MediaItemID, now); err != nil {
			return err
		}
		if promoted {
			if err := s.repo.MarkListItemRemoteRemoved(ctx, conn.ID, b.kind, state.MediaItemID, now); err != nil {
				return err
			}
		}
	}
	return nil
}

// localListTitles returns the profile's watchlist entries outside the library
// as export items, with their shadow states, the way localItemsFromRows does
// for library items.
func (s *Service) localListTitles(ctx context.Context, conn Connection, b listBinding) ([]LocalFavorite, []ListItemState, error) {
	titles := s.watchlistTitles(b)
	if titles == nil {
		return nil, nil, nil
	}
	entries, err := titles.ListWatchlistTitles(ctx, conn.UserID, conn.ProfileID)
	if err != nil {
		return nil, nil, err
	}
	items := make([]LocalFavorite, 0, len(entries))
	states := make([]ListItemState, 0, len(entries))
	for _, title := range entries {
		item := LocalFavorite{
			MediaItemID: title.listItemID(),
			Kind:        title.Kind,
			Title:       title.Title,
			Year:        title.Year,
			IMDbID:      title.IMDbID,
			TMDBID:      strconv.Itoa(title.TMDBID),
			FavoritedAt: title.AddedAt,
		}
		if title.TVDBID > 0 {
			item.TVDBID = strconv.Itoa(title.TVDBID)
		}
		if item.FavoritedAt.IsZero() {
			item.FavoritedAt = s.now()
		}
		item.ProviderItemKey = providerItemKeyForLocalFavorite(item)
		items = append(items, item)
		listedAt := item.FavoritedAt
		states = append(states, ListItemState{
			ConnectionID:    conn.ID,
			ListKind:        b.kind,
			MediaItemID:     item.MediaItemID,
			ProviderItemKey: item.ProviderItemKey,
			Kind:            item.Kind,
			Title:           item.Title,
			Year:            item.Year,
			LocalPresent:    true,
			LastSeenLocalAt: &listedAt,
		})
	}
	return items, states, nil
}
