package main

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/Silo-Server/silo-server/internal/api/handlers"
	"github.com/Silo-Server/silo-server/internal/catalog"
	"github.com/Silo-Server/silo-server/internal/jellycompat"
	"github.com/Silo-Server/silo-server/internal/metadata"
)

// compatItemImages is the part of the native admin image handler the
// Jellyfin artwork routes use.
type compatItemImages interface {
	GetAdminItemImages(context.Context, string) (handlers.AdminItemImagesView, error)
	ApplyAdminItemImage(context.Context, string, handlers.AdminItemImageRequest) (handlers.AdminItemImageResult, error)
	UploadAdminItemImage(context.Context, string, string, []byte) (handlers.AdminItemImageResult, error)
}

// compatCollectionArtwork is the part of the native collection handler the
// Jellyfin artwork routes use.
type compatCollectionArtwork interface {
	UploadAdminCollectionArtwork(ctx context.Context, id, kind string, data []byte) error
	DeleteAdminCollectionArtwork(ctx context.Context, id, kind string) error
}

// compatImageURLResolver turns a provider's stored image reference into a
// URL anyone can fetch.
type compatImageURLResolver interface {
	ResolveImageURLs(ctx context.Context, paths []string, variant string) map[string]string
}

// compatArtworkEditor lets the Jellyfin listener change artwork through the
// services the native admin image and collection APIs use, so an image a
// poster manager uploads is stored, locked and published the same way.
type compatArtworkEditor struct {
	items       compatItemImages
	collections compatCollectionArtwork
	resolver    compatImageURLResolver
}

var _ jellycompat.ArtworkEditor = (*compatArtworkEditor)(nil)

func (e *compatArtworkEditor) UploadItemImage(ctx context.Context, contentID, slot string, data []byte) error {
	_, err := e.items.UploadAdminItemImage(ctx, contentID, slot, data)
	return compatArtworkError(err)
}

// ResetItemImage applies the first image the item's list offers in slot that
// is not an upload: its local sidecar artwork when it has some, else the
// providers' first choice.
func (e *compatArtworkEditor) ResetItemImage(ctx context.Context, contentID, slot string) error {
	view, err := e.items.GetAdminItemImages(ctx, contentID)
	if err != nil {
		return compatArtworkError(err)
	}
	for _, image := range view.Images {
		if image.Type != slot || image.OriginalURL == "" || metadata.IsUploadedImageSource(image.OriginalURL) {
			continue
		}
		_, err := e.items.ApplyAdminItemImage(ctx, contentID, handlers.AdminItemImageRequest{
			OriginalURL: image.OriginalURL,
			Type:        slot,
			ProviderID:  image.ProviderID,
		})
		return compatArtworkError(err)
	}
	return &jellycompat.ArtworkEditError{Status: http.StatusNotFound, Message: "No provider offers a " + slot + " for this item"}
}

// ProviderImages lists the slot's provider images with full-size URLs. Local
// sidecar files and uploads are left out: they have no public URL.
func (e *compatArtworkEditor) ProviderImages(ctx context.Context, contentID, slot string) ([]jellycompat.RemoteArtwork, error) {
	view, err := e.items.GetAdminItemImages(ctx, contentID)
	if err != nil {
		return nil, compatArtworkError(err)
	}
	var unresolved []string
	for _, image := range view.Images {
		if image.Type == slot && !isPublicImageURL(image.OriginalURL) && !metadata.IsLocalImageSource(image.OriginalURL) && !metadata.IsUploadedImageSource(image.OriginalURL) {
			unresolved = append(unresolved, image.OriginalURL)
		}
	}
	var resolved map[string]string
	if len(unresolved) > 0 && e.resolver != nil {
		resolved = e.resolver.ResolveImageURLs(ctx, unresolved, "original")
	}
	out := []jellycompat.RemoteArtwork{}
	for _, image := range view.Images {
		if image.Type != slot {
			continue
		}
		url := image.OriginalURL
		if !isPublicImageURL(url) {
			url = resolved[url]
		}
		if !isPublicImageURL(url) {
			continue
		}
		out = append(out, jellycompat.RemoteArtwork{
			ProviderName: image.ProviderID,
			URL:          url,
			Language:     image.Language,
			Width:        image.Width,
			Height:       image.Height,
			Rating:       image.Rating,
		})
	}
	return out, nil
}

func (e *compatArtworkEditor) UploadCollectionImage(ctx context.Context, collectionID, slot string, data []byte) error {
	if e.collections == nil {
		return &jellycompat.ArtworkEditError{Status: http.StatusServiceUnavailable, Message: "Collection artwork is not available"}
	}
	if slot == "logo" {
		return &jellycompat.ArtworkEditError{Status: http.StatusBadRequest, Message: "Collections take a Primary or Backdrop image"}
	}
	return compatArtworkError(e.collections.UploadAdminCollectionArtwork(ctx, collectionID, slot, data))
}

func (e *compatArtworkEditor) DeleteCollectionImage(ctx context.Context, collectionID, slot string) error {
	if e.collections == nil {
		return &jellycompat.ArtworkEditError{Status: http.StatusServiceUnavailable, Message: "Collection artwork is not available"}
	}
	if slot == "logo" {
		return &jellycompat.ArtworkEditError{Status: http.StatusBadRequest, Message: "Collections take a Primary or Backdrop image"}
	}
	return compatArtworkError(e.collections.DeleteAdminCollectionArtwork(ctx, collectionID, slot))
}

func isPublicImageURL(url string) bool {
	return strings.HasPrefix(url, "https://") || strings.HasPrefix(url, "http://")
}

// compatArtworkError keeps the status and message of a native handler error
// and maps a missing collection to 404; anything else stays an internal
// error.
func compatArtworkError(err error) error {
	if err == nil {
		return nil
	}
	if apiErr, ok := errors.AsType[*handlers.APIError](err); ok {
		return &jellycompat.ArtworkEditError{Status: apiErr.Status, Message: apiErr.Message}
	}
	if errors.Is(err, catalog.ErrLibraryCollectionNotFound) {
		return &jellycompat.ArtworkEditError{Status: http.StatusNotFound, Message: "Collection not found"}
	}
	return err
}
