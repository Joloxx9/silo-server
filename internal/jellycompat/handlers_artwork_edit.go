package jellycompat

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
)

// Artwork slots the admin image routes accept, named as the native admin
// image API names them.
const (
	artworkSlotPoster   = "poster"
	artworkSlotBackdrop = "backdrop"
	artworkSlotLogo     = "logo"
)

// maxCompatArtworkUploadBytes bounds a POST /Items/{id}/Images body. Jellyfin
// sends the image base64 encoded, so a 10 MiB image arrives as about 13.4 MiB.
const maxCompatArtworkUploadBytes = 15 << 20

// RemoteArtwork is one provider image GET /Items/{id}/RemoteImages lists.
// URL is the full-size image, fetchable without Silo credentials.
type RemoteArtwork struct {
	ProviderName string
	URL          string
	Language     string
	Width        int
	Height       int
	Rating       float64
}

// ArtworkEditError carries the status and message a failed artwork change
// answers with.
type ArtworkEditError struct {
	Status  int
	Message string
}

func (e *ArtworkEditError) Error() string { return e.Message }

// ArtworkEditor changes catalog artwork for the Jellyfin image write routes,
// through the same services the native admin image API uses. Slots are
// poster, backdrop and logo.
type ArtworkEditor interface {
	// UploadItemImage stores data as the item's artwork in slot and locks the
	// item's images, as an upload in the admin image picker does.
	UploadItemImage(ctx context.Context, contentID, slot string, data []byte) error
	// ResetItemImage shows the image the item's providers offer first in
	// slot again, in place of an uploaded one.
	ResetItemImage(ctx context.Context, contentID, slot string) error
	// ProviderImages lists the images the item's providers offer in slot.
	ProviderImages(ctx context.Context, contentID, slot string) ([]RemoteArtwork, error)
	// UploadCollectionImage stores data as a collection's poster or backdrop.
	UploadCollectionImage(ctx context.Context, collectionID, slot string, data []byte) error
	// DeleteCollectionImage removes a collection's poster or backdrop.
	DeleteCollectionImage(ctx context.Context, collectionID, slot string) error
}

// ArtworkEditHandler serves the Jellyfin routes that change artwork:
// POST and DELETE /Items/{id}/Images/{type}, POST /Items/{id}/Refresh and
// GET /Items/{id}/RemoteImages. Poster managers written for Jellyfin use
// them to replace posters. The router lets only admin API keys reach them.
type ArtworkEditHandler struct {
	editor ArtworkEditor
	codec  *ResourceIDCodec
	images *ImageCache
}

// NewArtworkEditHandler builds the handler; a nil editor answers 503.
func NewArtworkEditHandler(editor ArtworkEditor, codec *ResourceIDCodec, images *ImageCache) *ArtworkEditHandler {
	return &ArtworkEditHandler{editor: editor, codec: codec, images: images}
}

// artworkTarget is the catalog object a route ID names.
type artworkTarget struct {
	contentID    string
	collectionID string
}

func (h *ArtworkEditHandler) target(routeID string) (artworkTarget, bool) {
	if collectionID, err := h.codec.DecodeStringID(EncodedIDCollection, routeID); err == nil && collectionID != "" {
		return artworkTarget{collectionID: collectionID}, true
	}
	if contentID, err := decodeContentID(h.codec, routeID); err == nil && contentID != "" {
		return artworkTarget{contentID: contentID}, true
	}
	return artworkTarget{}, false
}

// compatArtworkSlot maps a Jellyfin image type to the slot it fills.
func compatArtworkSlot(imageType string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(imageType)) {
	case "primary":
		return artworkSlotPoster, true
	case "backdrop":
		return artworkSlotBackdrop, true
	case "logo":
		return artworkSlotLogo, true
	}
	return "", false
}

// decodeCompatImageBody reads the image a client posted. Jellyfin expects the
// body base64 encoded; a body that is not base64 is taken as the image bytes
// themselves, which some clients send.
func decodeCompatImageBody(body []byte) []byte {
	trimmed := bytes.TrimSpace(body)
	if _, rest, ok := bytes.Cut(trimmed, []byte(";base64,")); ok && bytes.HasPrefix(trimmed, []byte("data:")) {
		trimmed = rest
	}
	decoded := make([]byte, base64.StdEncoding.DecodedLen(len(trimmed)))
	if n, err := base64.StdEncoding.Decode(decoded, trimmed); err == nil && n > 0 {
		return decoded[:n]
	}
	return body
}

func (h *ArtworkEditHandler) ready(w http.ResponseWriter) bool {
	if h == nil || h.editor == nil {
		writeError(w, http.StatusServiceUnavailable, "Unavailable", "Artwork editing is not available")
		return false
	}
	return true
}

// HandleUploadImage serves POST /Items/{id}/Images/{type}[/{index}].
func (h *ArtworkEditHandler) HandleUploadImage(w http.ResponseWriter, r *http.Request) {
	if !h.ready(w) {
		return
	}
	routeID := chiURLParam(r, "id")
	target, ok := h.target(routeID)
	if !ok {
		writeError(w, http.StatusNotFound, "NotFound", "Item not found")
		return
	}
	slot, ok := compatArtworkSlot(chiURLParam(r, "imageType"))
	if !ok {
		writeError(w, http.StatusBadRequest, "BadRequest", "Upload a Primary, Backdrop or Logo image")
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxCompatArtworkUploadBytes))
	if err != nil {
		if _, tooLarge := errors.AsType[*http.MaxBytesError](err); tooLarge {
			writeError(w, http.StatusRequestEntityTooLarge, "PayloadTooLarge", "Images can be at most 10 MiB")
			return
		}
		writeError(w, http.StatusBadRequest, "BadRequest", "Could not read the image")
		return
	}
	data := decodeCompatImageBody(body)
	if len(data) == 0 {
		writeError(w, http.StatusBadRequest, "BadRequest", "Choose an image to upload")
		return
	}
	if target.collectionID != "" {
		err = h.editor.UploadCollectionImage(r.Context(), target.collectionID, slot, data)
	} else {
		err = h.editor.UploadItemImage(r.Context(), target.contentID, slot, data)
	}
	h.finish(w, r, routeID, err)
}

// HandleDeleteImage serves DELETE /Items/{id}/Images/{type}[/{index}]. Silo
// keeps one image per slot, so deleting it brings back the provider's choice
// (a collection's poster falls back to its collage), and a backdrop index
// above zero names nothing to delete.
func (h *ArtworkEditHandler) HandleDeleteImage(w http.ResponseWriter, r *http.Request) {
	if !h.ready(w) {
		return
	}
	routeID := chiURLParam(r, "id")
	target, ok := h.target(routeID)
	if !ok {
		writeError(w, http.StatusNotFound, "NotFound", "Item not found")
		return
	}
	slot, ok := compatArtworkSlot(chiURLParam(r, "imageType"))
	if !ok {
		writeError(w, http.StatusBadRequest, "BadRequest", "Delete a Primary, Backdrop or Logo image")
		return
	}
	if raw := chiURLParam(r, "index"); raw != "" {
		if index, err := strconv.Atoi(raw); err != nil || index < 0 {
			writeError(w, http.StatusBadRequest, "BadRequest", "Invalid image index")
			return
		} else if index > 0 {
			w.WriteHeader(http.StatusNoContent)
			return
		}
	}
	var err error
	if target.collectionID != "" {
		err = h.editor.DeleteCollectionImage(r.Context(), target.collectionID, slot)
	} else {
		err = h.editor.ResetItemImage(r.Context(), target.contentID, slot)
	}
	h.finish(w, r, routeID, err)
}

// HandleRefresh serves POST /Items/{id}/Refresh. Jellyfin clients call it
// after deleting images so the server downloads new ones; in Silo the delete
// already brought the provider's image back, so there is nothing left to do.
func (h *ArtworkEditHandler) HandleRefresh(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.target(chiURLParam(r, "id")); !ok {
		writeError(w, http.StatusNotFound, "NotFound", "Item not found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type remoteImageDTO struct {
	ProviderName    string  `json:"ProviderName"`
	URL             string  `json:"Url"`
	Height          int     `json:"Height,omitempty"`
	Width           int     `json:"Width,omitempty"`
	CommunityRating float64 `json:"CommunityRating,omitempty"`
	Type            string  `json:"Type"`
	Language        string  `json:"Language,omitempty"`
	RatingType      string  `json:"RatingType"`
}

type remoteImageResultDTO struct {
	Images           []remoteImageDTO `json:"Images"`
	TotalRecordCount int              `json:"TotalRecordCount"`
	Providers        []string         `json:"Providers"`
}

// HandleRemoteImages serves GET /Items/{id}/RemoteImages: the provider
// images of one type (Primary when Type is absent), in the providers' order.
// Collections have no provider images.
func (h *ArtworkEditHandler) HandleRemoteImages(w http.ResponseWriter, r *http.Request) {
	if !h.ready(w) {
		return
	}
	target, ok := h.target(chiURLParam(r, "id"))
	if !ok {
		writeError(w, http.StatusNotFound, "NotFound", "Item not found")
		return
	}
	q := newCaseInsensitiveQuery(r.URL.Query())
	imageType := strings.TrimSpace(q.Get("Type"))
	if imageType == "" {
		imageType = compatImagePrimary
	}
	result := remoteImageResultDTO{Images: []remoteImageDTO{}, Providers: []string{}}
	slot, ok := compatArtworkSlot(imageType)
	if !ok || target.collectionID != "" {
		writeJSON(w, http.StatusOK, result)
		return
	}
	images, err := h.editor.ProviderImages(r.Context(), target.contentID, slot)
	if err != nil {
		writeArtworkEditError(w, r, err)
		return
	}
	providers := map[string]bool{}
	for _, image := range images {
		result.Images = append(result.Images, remoteImageDTO{
			ProviderName:    image.ProviderName,
			URL:             image.URL,
			Height:          image.Height,
			Width:           image.Width,
			CommunityRating: image.Rating,
			Type:            imageType,
			Language:        image.Language,
			RatingType:      "Score",
		})
		if image.ProviderName != "" && !providers[image.ProviderName] {
			providers[image.ProviderName] = true
			result.Providers = append(result.Providers, image.ProviderName)
		}
	}
	if limit := parsePositiveInt(q.Get("Limit"), 0); limit > 0 {
		start := min(parsePositiveInt(q.Get("StartIndex"), 0), len(result.Images))
		result.TotalRecordCount = len(result.Images)
		result.Images = result.Images[start:min(start+limit, len(result.Images))]
	} else {
		result.TotalRecordCount = len(result.Images)
	}
	writeJSON(w, http.StatusOK, result)
}

// finish answers an artwork change. A change that took drops this node's
// cached image URLs for the route, so the next untagged image request reads
// the new artwork.
func (h *ArtworkEditHandler) finish(w http.ResponseWriter, r *http.Request, routeID string, err error) {
	if err != nil {
		writeArtworkEditError(w, r, err)
		return
	}
	h.images.ForgetRoute(routeID)
	w.WriteHeader(http.StatusNoContent)
}

func writeArtworkEditError(w http.ResponseWriter, r *http.Request, err error) {
	if editErr, ok := errors.AsType[*ArtworkEditError](err); ok {
		switch {
		case editErr.Status == http.StatusNotFound:
			writeError(w, http.StatusNotFound, "NotFound", editErr.Message)
		case editErr.Status >= 400 && editErr.Status < 500:
			writeError(w, editErr.Status, "BadRequest", editErr.Message)
		default:
			writeError(w, editErr.Status, "Error", editErr.Message)
		}
		return
	}
	slog.ErrorContext(r.Context(), "jellycompat artwork edit failed", "component", "jellycompat", "error", err)
	writeError(w, http.StatusInternalServerError, "Error", "Could not change the artwork")
}
