package watchsync

import (
	"context"
	"reflect"
	"testing"
	"time"
)

type watchlistBatchProviderStub struct {
	rows []RemoteFavorite
}

func (watchlistBatchProviderStub) Key() string         { return "plugin:4:list" }
func (watchlistBatchProviderStub) DisplayName() string { return "List" }
func (watchlistBatchProviderStub) Capabilities() Capabilities {
	return Capabilities{ImportWatchlist: true}
}
func (p watchlistBatchProviderStub) FetchWatchlist(context.Context, ServerConfig, Connection) ([]RemoteFavorite, error) {
	return p.rows, nil
}

// titlesStub keeps one profile's watchlist entries outside the library by
// TMDB ID, with the TMDB IDs the library carries.
type titlesStub struct {
	library map[int]bool
	entries map[int]time.Time
}

func (s *titlesStub) LibraryHas(_ context.Context, title WatchlistTitle) (bool, error) {
	return s.library[title.TMDBID], nil
}

func (s *titlesStub) AddWatchlistTitle(_ context.Context, _ int, _ string, title WatchlistTitle, addedAt time.Time) (bool, error) {
	if _, ok := s.entries[title.TMDBID]; ok {
		return false, nil
	}
	s.entries[title.TMDBID] = addedAt
	return true, nil
}

func (s *titlesStub) RemoveWatchlistTitle(_ context.Context, _ int, _ string, title WatchlistTitle) error {
	delete(s.entries, title.TMDBID)
	return nil
}

func (s *titlesStub) WatchlistTitleListed(_ context.Context, _ int, _ string, title WatchlistTitle) (bool, error) {
	_, ok := s.entries[title.TMDBID]
	return ok, nil
}

// A provider's watchlist title the library doesn't have becomes a watchlist
// entry outside the library, follows the provider's removals, and a local
// removal or a promotion onto the library watchlist retires its row.
func TestServiceWatchlistImportKeepsTitlesOutsideLibrary(t *testing.T) {
	const titleRow = "watchlist-title:movie:550"
	listedAt := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	fightClub := RemoteFavorite{
		ProviderItemKey: "movie:imdb:tt0137523", Kind: "movie", Title: "Fight Club", Year: 1999,
		IMDbID: "tt0137523", TMDBID: "550", FavoritedAt: listedAt,
	}
	tests := []struct {
		name        string
		library     map[int]bool
		entries     map[int]time.Time
		states      []ListItemState
		rows        []RemoteFavorite
		wantEntry   bool
		wantResult  ImportListResult
		wantLocal   bool
		wantRemote  bool
		wantNoState bool
	}{
		{
			name:       "listed title the library lacks",
			rows:       []RemoteFavorite{fightClub},
			wantEntry:  true,
			wantResult: ImportListResult{Found: 1, Imported: 1, added: 1},
			wantLocal:  true, wantRemote: true,
		},
		{
			name:        "listed title the library has but did not match",
			library:     map[int]bool{550: true},
			rows:        []RemoteFavorite{fightClub},
			wantResult:  ImportListResult{Found: 1, Unmatched: 1},
			wantNoState: true,
		},
		{
			name:        "listed title without a TMDB ID",
			rows:        []RemoteFavorite{{ProviderItemKey: "movie:imdb:tt0137523", Kind: "movie", IMDbID: "tt0137523"}},
			wantResult:  ImportListResult{Found: 1, Unmatched: 1},
			wantNoState: true,
		},
		{
			name:       "title the provider dropped",
			entries:    map[int]time.Time{550: listedAt},
			states:     []ListItemState{{MediaItemID: titleRow, Kind: "movie", ProviderItemKey: fightClub.ProviderItemKey, RemotePresent: true, LocalPresent: true}},
			wantResult: ImportListResult{Removed: 1},
		},
		{
			name:       "title removed locally waits for the provider removal",
			states:     []ListItemState{{MediaItemID: titleRow, Kind: "movie", ProviderItemKey: fightClub.ProviderItemKey, RemotePresent: true, LocalPresent: true}},
			rows:       []RemoteFavorite{fightClub},
			wantResult: ImportListResult{Found: 1},
			wantRemote: true,
		},
		{
			name:       "title promoted onto the library watchlist",
			library:    map[int]bool{550: true},
			states:     []ListItemState{{MediaItemID: titleRow, Kind: "movie", ProviderItemKey: fightClub.ProviderItemKey, RemotePresent: true, LocalPresent: true}},
			rows:       []RemoteFavorite{fightClub},
			wantResult: ImportListResult{Found: 1, Unmatched: 1},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store, repo, conn := favoriteImportFixture(t)
			conn.ImportWatchlistEnabled, conn.SyncWatchlistRemovalsEnabled = true, true
			for _, state := range tt.states {
				state.ConnectionID, state.ListKind = conn.ID, ListKindWatchlist
				repo.listItemStates = append(repo.listItemStates, state)
			}
			titles := &titlesStub{library: tt.library, entries: map[int]time.Time{}}
			for id, at := range tt.entries {
				titles.entries[id] = at
			}
			service := NewService(repo, NewRegistry()).
				WithMatcher(unmatchedMatcherStub{}).
				WithUserStoreProvider(staticStoreProvider{store: store})
			service.SetWatchlistTitles(titles)

			result, err := service.importList(context.Background(), conn, ServerConfig{}, watchlistBatchProviderStub{rows: tt.rows}, service.watchlistBinding())
			if err != nil {
				t.Fatal(err)
			}
			result.Warnings = nil
			if !reflect.DeepEqual(result, tt.wantResult) {
				t.Fatalf("result = %#v, want %#v", result, tt.wantResult)
			}
			if _, ok := titles.entries[550]; ok != tt.wantEntry {
				t.Fatalf("entry present = %v, want %v", ok, tt.wantEntry)
			}
			state := listItemStateOf(repo, titleRow)
			if tt.wantNoState {
				if state != nil {
					t.Fatalf("state = %#v, want none", state)
				}
				return
			}
			if state == nil || state.LocalPresent != tt.wantLocal || state.RemotePresent != tt.wantRemote {
				t.Fatalf("state = %#v, want local %v remote %v", state, tt.wantLocal, tt.wantRemote)
			}
		})
	}
}

func listItemStateOf(repo *serviceFakeRepo, mediaItemID string) *ListItemState {
	for i := range repo.listItemStates {
		if repo.listItemStates[i].MediaItemID == mediaItemID {
			return &repo.listItemStates[i]
		}
	}
	return nil
}
