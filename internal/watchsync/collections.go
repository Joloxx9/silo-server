package watchsync

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/Silo-Server/silo-server/internal/historyimport"
	"github.com/Silo-Server/silo-server/internal/userstore"
)

// Named-list sync mirrors each of a provider account's own lists (other than
// its watchlist and favorites) into a manual collection on the connected
// profile, both ways. Every linked list runs through the shared list pipeline
// in lists.go as its own listBinding, with the list kind
// "collection:<provider key>", so item state, pending removals, and remote
// reconciliation work exactly as they do for the watchlist.
//
// The provider's lists are the source of truth for which collections exist:
// a list new on the provider gets a new collection, and a list gone from the
// provider is unlinked while its collection stays. A collection made in Silo
// is not created on the provider.

const collectionListKindPrefix = "collection:"

// collectionManagementModeManual matches the collection type the collections
// API edits by hand.
const collectionManagementModeManual = "manual"

func collectionListKind(key string) ListKind {
	return ListKind(collectionListKindPrefix + key)
}

// RemoteCollection is one of the provider account's named lists.
type RemoteCollection struct {
	Key         string
	Name        string
	Description string
}

// CollectionImportBatch is a complete snapshot of the account's named lists
// and their items, keyed by collection key. Every key in Rows is listed in
// Collections.
type CollectionImportBatch struct {
	Collections []RemoteCollection
	Rows        map[string][]RemoteFavorite
	Warnings    []string
}

// CollectionSyncer is implemented by providers that sync named lists.
type CollectionSyncer interface {
	FetchCollections(ctx context.Context, cfg ServerConfig, conn Connection) (CollectionImportBatch, error)
	ExportCollectionItems(ctx context.Context, cfg ServerConfig, conn Connection, collectionKey string, items []LocalFavorite) (ExportResult, error)
	RemoveCollectionItems(ctx context.Context, cfg ServerConfig, conn Connection, collectionKey string, items []LocalFavorite) (ExportResult, error)
}

// CollectionLink ties a provider list to the local collection that mirrors it.
type CollectionLink struct {
	ConnectionID  string
	CollectionKey string
	CollectionID  string
	Name          string
}

// collectionLinkStore persists CollectionLinks. PostgresRepository implements
// it; it is kept out of Repository so fakes that do not exercise named lists
// need not implement it.
type collectionLinkStore interface {
	ListCollectionLinks(ctx context.Context, connectionID string) ([]CollectionLink, error)
	ListCollectionLinksForCollection(ctx context.Context, collectionID string) ([]CollectionLink, error)
	UpsertCollectionLink(ctx context.Context, link CollectionLink) error
	// DeleteCollectionLink removes the link and the item state of its list.
	DeleteCollectionLink(ctx context.Context, connectionID, collectionKey string) error
	// DeleteCollectionLinks removes every link of the connection and the item
	// state of their lists.
	DeleteCollectionLinks(ctx context.Context, connectionID string) error
}

func (s *Service) collectionLinkStore() (collectionLinkStore, error) {
	links, ok := s.repo.(collectionLinkStore)
	if !ok {
		return nil, errors.New("collection link store is not configured")
	}
	return links, nil
}

// SyncCollectionsResult summarizes one named-list sync.
type SyncCollectionsResult struct {
	Collections int
	Created     int
	Unlinked    int
	Imported    int
	Removed     int
	Sent        int
	Warnings    []string
}

func (s *Service) syncCollections(ctx context.Context, conn Connection, cfg ServerConfig, provider Provider) (SyncCollectionsResult, error) {
	var result SyncCollectionsResult
	syncer, ok := provider.(CollectionSyncer)
	if !ok {
		return result, fmt.Errorf("provider %q does not implement collection sync", conn.Provider)
	}
	links, err := s.collectionLinkStore()
	if err != nil {
		return result, err
	}
	if s.storeProvider == nil {
		return result, errors.New("user store provider is not configured")
	}
	store, err := s.storeProvider.ForUser(ctx, conn.UserID)
	if err != nil {
		return result, fmt.Errorf("open user store: %w", err)
	}
	batch, err := syncer.FetchCollections(ctx, cfg, conn)
	if err != nil {
		return result, err
	}
	result.Warnings = append(result.Warnings, batch.Warnings...)
	existing, err := links.ListCollectionLinks(ctx, conn.ID)
	if err != nil {
		return result, err
	}
	linkByKey := make(map[string]CollectionLink, len(existing))
	for _, link := range existing {
		linkByKey[link.CollectionKey] = link
	}

	listed := make(map[string]bool, len(batch.Collections))
	for _, remote := range batch.Collections {
		listed[remote.Key] = true
		link, err := s.ensureCollectionLink(ctx, links, store, conn, remote, linkByKey[remote.Key], &result)
		if err != nil {
			return result, err
		}
		result.Collections++
		b := s.collectionBinding(syncer, store, link, batch.Rows[remote.Key])
		if err := s.markLocalCollectionRemovals(ctx, conn, store, b, link); err != nil {
			return result, err
		}
		imported, err := s.importList(ctx, conn, cfg, provider, b)
		result.Imported += imported.Imported
		result.Removed += imported.Removed
		if imported.Unmatched > 0 {
			result.Warnings = append(result.Warnings, fmt.Sprintf(
				"%d item(s) on list %q are not in the library", imported.Unmatched, link.Name))
		}
		if err != nil {
			return result, err
		}
		exported, err := s.exportList(ctx, conn, cfg, provider, b)
		result.Sent += exported.Sent
		result.Warnings = append(result.Warnings, exported.Warnings...)
		if err != nil {
			return result, err
		}
		removed, err := s.removePendingListItems(ctx, conn, cfg, provider, b)
		result.Sent += removed
		if err != nil {
			return result, err
		}
	}

	// A list gone from the provider is unlinked; its collection stays as an
	// ordinary manual collection.
	for key := range linkByKey {
		if listed[key] {
			continue
		}
		if err := links.DeleteCollectionLink(ctx, conn.ID, key); err != nil {
			return result, err
		}
		result.Unlinked++
	}
	return result, nil
}

// ensureCollectionLink returns the link for a provider list, creating the
// local collection when the list is new or its collection was deleted in Silo.
// A recreated collection starts with fresh item state, so the next steps add
// the provider's items again instead of reading them as local removals.
func (s *Service) ensureCollectionLink(
	ctx context.Context,
	links collectionLinkStore,
	store userstore.UserStore,
	conn Connection,
	remote RemoteCollection,
	link CollectionLink,
	result *SyncCollectionsResult,
) (CollectionLink, error) {
	if link.CollectionID != "" {
		collection, err := store.GetCollection(ctx, link.CollectionID)
		switch {
		case err == nil && collection.CreatorProfileID == conn.ProfileID &&
			collection.CollectionType == collectionManagementModeManual:
			return link, nil
		case err != nil && !errors.Is(err, userstore.ErrCollectionNotFound):
			return CollectionLink{}, err
		}
		if err := links.DeleteCollectionLink(ctx, conn.ID, remote.Key); err != nil {
			return CollectionLink{}, err
		}
	}
	name := strings.TrimSpace(remote.Name)
	if name == "" {
		name = remote.Key
	}
	created, err := store.CreateCollection(ctx, userstore.CreateCollectionInput{
		CreatorProfileID: conn.ProfileID,
		Name:             name,
		Description:      remote.Description,
		CollectionType:   collectionManagementModeManual,
	})
	if err != nil {
		return CollectionLink{}, fmt.Errorf("create collection for list %q: %w", name, err)
	}
	link = CollectionLink{
		ConnectionID:  conn.ID,
		CollectionKey: remote.Key,
		CollectionID:  created.ID,
		Name:          name,
	}
	if err := links.UpsertCollectionLink(ctx, link); err != nil {
		return CollectionLink{}, err
	}
	result.Created++
	return link, nil
}

// collectionBinding runs one linked list through the shared list pipeline.
// rows is the list's part of an already fetched snapshot; a binding built for
// a local change event passes nil and never imports.
func (s *Service) collectionBinding(syncer CollectionSyncer, store userstore.UserStore, link CollectionLink, rows []RemoteFavorite) listBinding {
	kind := collectionListKind(link.CollectionKey)
	enabled := func(c Connection) bool { return c.SyncCollectionsEnabled }
	capable := func(c Capabilities) bool { return c.SyncCollections }
	collectionID := link.CollectionID
	return listBinding{
		kind:            kind,
		importEnabled:   enabled,
		exportEnabled:   enabled,
		removalsEnabled: enabled,
		capImport:       capable,
		capExport:       capable,
		capRemove:       capable,
		fetchBatch: func(context.Context, ServerConfig, Connection, Provider) (FavoriteImportBatch, bool, error) {
			return FavoriteImportBatch{Rows: rows}, true, nil
		},
		exportItems: func(ctx context.Context, cfg ServerConfig, conn Connection, _ Provider, items []LocalFavorite) (ExportResult, bool, error) {
			res, err := syncer.ExportCollectionItems(ctx, cfg, conn, link.CollectionKey, items)
			return res, true, err
		},
		removeItems: func(ctx context.Context, cfg ServerConfig, conn Connection, _ Provider, items []LocalFavorite) (ExportResult, bool, error) {
			res, err := syncer.RemoveCollectionItems(ctx, cfg, conn, link.CollectionKey, items)
			return res, true, err
		},
		localAdd: func(ctx context.Context, _ userstore.UserStore, _, mediaItemID string, _ time.Time) (bool, error) {
			current, err := store.ListCollectionItems(ctx, collectionID)
			if err != nil {
				return false, err
			}
			for _, item := range current {
				if item.MediaItemID == mediaItemID {
					return false, nil
				}
			}
			if err := store.AddCollectionItem(ctx, collectionID, mediaItemID, len(current)); err != nil {
				return false, err
			}
			return true, nil
		},
		localRemove: func(ctx context.Context, _ userstore.UserStore, _, mediaItemID string) error {
			return store.RemoveCollectionItem(ctx, collectionID, mediaItemID)
		},
		localList: func(ctx context.Context, _ userstore.UserStore, _ string, _, offset int) ([]listRow, error) {
			if offset > 0 {
				return nil, nil
			}
			items, err := store.ListCollectionItems(ctx, collectionID)
			if err != nil {
				return nil, err
			}
			rows := make([]listRow, 0, len(items))
			for _, item := range items {
				rows = append(rows, listRow{MediaItemID: item.MediaItemID, AddedAt: item.AddedAt})
			}
			return rows, nil
		},
		setLastSync:     func(*ConnectionSyncUpdate, time.Time) {},
		setImportCounts: func(*SyncRun, int, int) {},
		setExportCounts: func(*SyncRun, int, int) {},
		setRemovalCount: func(*SyncRun, int) {},
	}
}

// markLocalCollectionRemovals records items that left the collection without
// a change event, such as through a bulk edit, so the removal step takes them
// off the provider list instead of the import putting them back.
func (s *Service) markLocalCollectionRemovals(ctx context.Context, conn Connection, store userstore.UserStore, b listBinding, link CollectionLink) error {
	states, err := s.repo.ListListItemStates(ctx, conn.ID, b.kind)
	if err != nil {
		return err
	}
	if len(states) == 0 {
		return nil
	}
	items, err := store.ListCollectionItems(ctx, link.CollectionID)
	if err != nil {
		return err
	}
	present := make(map[string]bool, len(items))
	for _, item := range items {
		present[item.MediaItemID] = true
	}
	now := s.now()
	for _, state := range states {
		if state.LocalPresent && !present[state.MediaItemID] {
			if err := s.repo.MarkListItemLocalRemoved(ctx, conn.ID, b.kind, state.MediaItemID, now); err != nil {
				return err
			}
		}
	}
	return nil
}

// LocalCollectionEvent is a real-time add to or removal from a collection.
type LocalCollectionEvent struct {
	CollectionID string
	Change       ListChange
	UserID       int
	ProfileID    string
	Items        []LocalFavorite
}

// HandleLocalCollectionEvent mirrors a collection edit to the provider lists
// linked to the collection. Like HandleLocalListEvent it is fire-and-forget;
// the next scheduled sync reconciles anything that fails here.
func (s *Service) HandleLocalCollectionEvent(ctx context.Context, event LocalCollectionEvent) error {
	if event.UserID == 0 || event.ProfileID == "" || event.CollectionID == "" || len(event.Items) == 0 {
		return nil
	}
	go func() {
		bg, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		if err := s.processLocalCollectionEvent(bg, event); err != nil {
			slog.WarnContext(ctx, "failed to dispatch local collection provider event", "component", "watchsync",
				"collection_id", event.CollectionID, "change", event.Change, "user_id", event.UserID, "error", err)
		}
	}()
	return nil
}

func (s *Service) processLocalCollectionEvent(ctx context.Context, event LocalCollectionEvent) error {
	links, err := s.collectionLinkStore()
	if err != nil {
		return err
	}
	linked, err := links.ListCollectionLinksForCollection(ctx, event.CollectionID)
	if err != nil || len(linked) == 0 {
		return err
	}
	if s.storeProvider == nil {
		return errors.New("user store provider is not configured")
	}
	store, err := s.storeProvider.ForUser(ctx, event.UserID)
	if err != nil {
		return fmt.Errorf("open user store: %w", err)
	}
	items := make([]LocalFavorite, 0, len(event.Items))
	for _, item := range event.Items {
		if item.Kind == historyimport.KindMovie || item.Kind == historyimport.KindSeries {
			items = append(items, item)
		}
	}
	if len(items) == 0 {
		return nil
	}
	now := s.now()
	for _, link := range linked {
		conn, ok, err := s.repo.GetConnectionByID(ctx, link.ConnectionID)
		if err != nil {
			return err
		}
		if !ok || conn.UserID != event.UserID || conn.ProfileID != event.ProfileID || !conn.SyncCollectionsEnabled ||
			(conn.RateLimitedUntil != nil && conn.RateLimitedUntil.After(now)) {
			continue
		}
		provider, ok := s.registry.Get(conn.Provider)
		if !ok {
			continue
		}
		syncer, ok := provider.(CollectionSyncer)
		if !ok || !provider.Capabilities().SyncCollections {
			continue
		}
		b := s.collectionBinding(syncer, store, link, nil)
		if err := s.dispatchLocalListChange(ctx, conn, provider, b, event.Change, items); err != nil {
			return err
		}
	}
	return nil
}
