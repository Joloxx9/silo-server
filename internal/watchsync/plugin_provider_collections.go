package watchsync

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	pluginv1 "github.com/Silo-Server/silo-plugin-sdk/pkg/pluginproto/silo/plugin/v1"

	"github.com/Silo-Server/silo-server/internal/historyimport"
)

// FetchCollections reads the account's named lists. The contract requires a
// complete snapshot, since a list or item missing from a delta would read as
// deleted.
func (p *PluginProvider) FetchCollections(ctx context.Context, _ ServerConfig, conn Connection) (CollectionImportBatch, error) {
	traversal, err := p.listRemoteState(ctx, conn, pluginCollectionsCursorKey,
		pluginv1.WatchSyncRemoteStateKind_WATCH_SYNC_REMOTE_STATE_KIND_COLLECTION)
	if err != nil {
		return CollectionImportBatch{}, err
	}
	if !traversal.completeSnapshot {
		return CollectionImportBatch{}, errors.New("watch sync plugin returned an incremental collection traversal")
	}
	batch := CollectionImportBatch{
		Rows:     make(map[string][]RemoteFavorite, len(traversal.collections)),
		Warnings: traversal.warnings,
	}
	keys := make([]string, 0, len(traversal.collections))
	for key := range traversal.collections {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		collection := traversal.collections[key]
		batch.Collections = append(batch.Collections, RemoteCollection{
			Key:         key,
			Name:        strings.TrimSpace(collection.GetName()),
			Description: strings.TrimSpace(collection.GetDescription()),
		})
	}
	for _, state := range traversal.items {
		listed := state.GetCollection()
		if listed == nil {
			continue
		}
		key := strings.TrimSpace(listed.GetCollectionKey())
		if _, ok := traversal.collections[key]; !ok {
			batch.Warnings = append(batch.Warnings, "watch sync plugin returned an item for an unlisted collection")
			continue
		}
		row, err := remoteFavoriteFromProto(p.Key(), state, &pluginv1.WatchSyncRemoteListState{ListedAt: listed.GetListedAt()})
		if err != nil {
			batch.Warnings = append(batch.Warnings, err.Error())
			continue
		}
		if row.Kind != historyimport.KindMovie && row.Kind != historyimport.KindSeries {
			batch.Warnings = append(batch.Warnings, "watch sync plugin returned a collection item that is not a movie or series")
			continue
		}
		batch.Rows[key] = append(batch.Rows[key], row)
	}
	return batch, nil
}

func (p *PluginProvider) ExportCollectionItems(ctx context.Context, _ ServerConfig, conn Connection, collectionKey string, items []LocalFavorite) (ExportResult, error) {
	return p.applyCollectionEvents(ctx, conn, collectionKey, items, pluginv1.WatchSyncOperation_WATCH_SYNC_OPERATION_ADD_TO_COLLECTION)
}

func (p *PluginProvider) RemoveCollectionItems(ctx context.Context, _ ServerConfig, conn Connection, collectionKey string, items []LocalFavorite) (ExportResult, error) {
	return p.applyCollectionEvents(ctx, conn, collectionKey, items, pluginv1.WatchSyncOperation_WATCH_SYNC_OPERATION_REMOVE_FROM_COLLECTION)
}

func (p *PluginProvider) applyCollectionEvents(
	ctx context.Context,
	conn Connection,
	collectionKey string,
	items []LocalFavorite,
	operation pluginv1.WatchSyncOperation,
) (ExportResult, error) {
	result := ExportResult{Failed: make(map[string]string)}
	events := make([]*pluginv1.WatchSyncEvent, 0, len(items))
	keys := make([]string, 0, len(items))
	for _, item := range items {
		media := mediaFromLocalFavorite(item)
		if !p.supportsMedia(media.GetMediaType()) {
			result.Failed[item.MediaItemID] = unsupportedWatchSyncMediaMessage(media.GetMediaType())
			continue
		}
		events = append(events, &pluginv1.WatchSyncEvent{
			// One item can be on several lists, so the key is part of the ID.
			EventId:         fmt.Sprintf("%s:%s:%s", operation.String(), collectionKey, item.MediaItemID),
			Operation:       operation,
			Origin:          pluginv1.WatchSyncOrigin_WATCH_SYNC_ORIGIN_MANUAL,
			OccurredAt:      timestampOrNil(item.FavoritedAt),
			Media:           media,
			ProviderItemKey: item.ProviderItemKey,
			CollectionKey:   collectionKey,
		})
		keys = append(keys, item.MediaItemID)
	}
	applied, err := p.applyPluginEvents(ctx, conn, events, keys)
	return mergeExportFailures(applied, result.Failed), err
}
