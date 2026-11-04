package watchsync

import (
	"context"
	"slices"
	"sort"
	"strings"
	"testing"
)

// collectionRepo adds the collection link store to the service fake.
type collectionRepo struct {
	*serviceFakeRepo
	links map[string]CollectionLink
}

func (r *collectionRepo) ListCollectionLinks(_ context.Context, connectionID string) ([]CollectionLink, error) {
	var links []CollectionLink
	for _, link := range r.links {
		if link.ConnectionID == connectionID {
			links = append(links, link)
		}
	}
	return links, nil
}

func (r *collectionRepo) ListCollectionLinksForCollection(_ context.Context, collectionID string) ([]CollectionLink, error) {
	var links []CollectionLink
	for _, link := range r.links {
		if link.CollectionID == collectionID {
			links = append(links, link)
		}
	}
	return links, nil
}

func (r *collectionRepo) UpsertCollectionLink(_ context.Context, link CollectionLink) error {
	r.links[link.ConnectionID+"/"+link.CollectionKey] = link
	return nil
}

func (r *collectionRepo) DeleteCollectionLink(_ context.Context, connectionID, collectionKey string) error {
	delete(r.links, connectionID+"/"+collectionKey)
	kind := collectionListKind(collectionKey)
	r.listItemStates = slices.DeleteFunc(r.listItemStates, func(s ListItemState) bool {
		return s.ConnectionID == connectionID && s.ListKind == kind
	})
	return nil
}

func (r *collectionRepo) DeleteCollectionLinks(ctx context.Context, connectionID string) error {
	for _, link := range r.links {
		if link.ConnectionID == connectionID {
			_ = r.DeleteCollectionLink(ctx, connectionID, link.CollectionKey)
		}
	}
	return nil
}

// collectionProviderStub holds the provider's lists as TMDB movie ids.
type collectionProviderStub struct {
	names map[string]string
	items map[string][]string
	sent  []string
}

func (*collectionProviderStub) Key() string         { return "plugin:4:list" }
func (*collectionProviderStub) DisplayName() string { return "List" }
func (*collectionProviderStub) Capabilities() Capabilities {
	return Capabilities{SyncCollections: true}
}

func (p *collectionProviderStub) FetchCollections(context.Context, ServerConfig, Connection) (CollectionImportBatch, error) {
	batch := CollectionImportBatch{Rows: map[string][]RemoteFavorite{}}
	for key, name := range p.names {
		batch.Collections = append(batch.Collections, RemoteCollection{Key: key, Name: name})
		for _, tmdbID := range p.items[key] {
			batch.Rows[key] = append(batch.Rows[key], RemoteFavorite{
				ProviderItemKey: "tmdb:" + tmdbID, Kind: "movie", TMDBID: tmdbID,
			})
		}
	}
	return batch, nil
}

func (p *collectionProviderStub) ExportCollectionItems(_ context.Context, _ ServerConfig, _ Connection, key string, items []LocalFavorite) (ExportResult, error) {
	result := ExportResult{}
	for _, item := range items {
		if !slices.Contains(p.items[key], item.TMDBID) {
			p.items[key] = append(p.items[key], item.TMDBID)
		}
		p.sent = append(p.sent, "add "+key+" "+item.TMDBID)
		result.Sent = append(result.Sent, item.MediaItemID)
	}
	return result, nil
}

func (p *collectionProviderStub) RemoveCollectionItems(_ context.Context, _ ServerConfig, _ Connection, key string, items []LocalFavorite) (ExportResult, error) {
	result := ExportResult{}
	for _, item := range items {
		tmdbID := strings.TrimPrefix(item.ProviderItemKey, "tmdb:")
		p.items[key] = slices.DeleteFunc(p.items[key], func(id string) bool { return id == tmdbID })
		p.sent = append(p.sent, "remove "+key+" "+tmdbID)
		result.Sent = append(result.Sent, item.MediaItemID)
	}
	return result, nil
}

func TestServiceSyncsCollectionsBothWays(t *testing.T) {
	ctx := context.Background()
	store, fake, conn := favoriteImportFixture(t)
	conn.SyncCollectionsEnabled = true
	repo := &collectionRepo{serviceFakeRepo: fake, links: map[string]CollectionLink{}}
	media := map[string]LocalFavorite{
		"movie-1": {MediaItemID: "movie-1", ProviderItemKey: "tmdb:603", Kind: "movie", Title: "The Matrix", TMDBID: "603"},
		"movie-2": {MediaItemID: "movie-2", ProviderItemKey: "tmdb:550", Kind: "movie", Title: "Fight Club", TMDBID: "550"},
	}
	fake.listMedia = media
	service := NewService(repo, NewRegistry()).
		WithMatcher(ratingMatcherStub{media: media}).
		WithUserStoreProvider(staticStoreProvider{store: store})
	provider := &collectionProviderStub{
		names: map[string]string{"list-7": "Mine"},
		items: map[string][]string{"list-7": {"603"}},
	}
	sync := func() SyncCollectionsResult {
		t.Helper()
		result, err := service.syncCollections(ctx, conn, ServerConfig{}, provider)
		if err != nil {
			t.Fatal(err)
		}
		return result
	}
	local := func(collectionID string) []string {
		t.Helper()
		items, err := store.ListCollectionItems(ctx, collectionID)
		if err != nil {
			t.Fatal(err)
		}
		ids := make([]string, 0, len(items))
		for _, item := range items {
			ids = append(ids, item.MediaItemID)
		}
		sort.Strings(ids)
		return ids
	}

	// A new provider list becomes a collection holding its items.
	if result := sync(); result.Created != 1 || result.Imported != 1 {
		t.Fatalf("first sync = %#v", result)
	}
	link := repo.links["conn-1/list-7"]
	collection, err := store.GetCollection(ctx, link.CollectionID)
	if err != nil || collection.Name != "Mine" || collection.CollectionType != "manual" || collection.CreatorProfileID != conn.ProfileID {
		t.Fatalf("collection = %#v, %v", collection, err)
	}
	if got := local(link.CollectionID); !slices.Equal(got, []string{"movie-1"}) {
		t.Fatalf("local after first sync = %v", got)
	}

	// Removed on the provider, added in Silo without an event.
	provider.items["list-7"] = nil
	if err := store.AddCollectionItem(ctx, link.CollectionID, "movie-2", 1); err != nil {
		t.Fatal(err)
	}
	sync()
	if got := local(link.CollectionID); !slices.Equal(got, []string{"movie-2"}) {
		t.Fatalf("local after second sync = %v", got)
	}
	if !slices.Equal(provider.items["list-7"], []string{"550"}) {
		t.Fatalf("remote after second sync = %v", provider.items["list-7"])
	}

	// Removed in Silo without an event: the provider follows, and the
	// provider still listing it does not bring it back.
	if err := store.RemoveCollectionItem(ctx, link.CollectionID, "movie-2"); err != nil {
		t.Fatal(err)
	}
	sync()
	if got := local(link.CollectionID); len(got) != 0 {
		t.Fatalf("local after third sync = %v", got)
	}
	if len(provider.items["list-7"]) != 0 {
		t.Fatalf("remote after third sync = %v", provider.items["list-7"])
	}

	// A collection deleted in Silo comes back while the list exists.
	if err := store.DeleteCollection(ctx, link.CollectionID); err != nil {
		t.Fatal(err)
	}
	provider.items["list-7"] = []string{"603"}
	if result := sync(); result.Created != 1 {
		t.Fatalf("recreate sync = %#v", result)
	}
	recreated := repo.links["conn-1/list-7"]
	if recreated.CollectionID == link.CollectionID || !slices.Equal(local(recreated.CollectionID), []string{"movie-1"}) {
		t.Fatalf("recreated link = %#v", recreated)
	}

	// A list deleted on the provider is unlinked, and its collection stays.
	provider.names = map[string]string{}
	if result := sync(); result.Unlinked != 1 {
		t.Fatalf("unlink sync = %#v", result)
	}
	if len(repo.links) != 0 {
		t.Fatalf("links = %#v", repo.links)
	}
	if got := local(recreated.CollectionID); !slices.Equal(got, []string{"movie-1"}) {
		t.Fatalf("local after unlink = %v", got)
	}
}
