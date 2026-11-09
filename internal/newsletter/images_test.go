package newsletter

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

func TestArtworkFetcherLimitsRequestsToResolvedArtwork(t *testing.T) {
	var hits atomic.Int32
	var data bytes.Buffer
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	img.Set(0, 0, color.White)
	if err := png.Encode(&data, img); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		if r.URL.Path == "/redirect" {
			http.Redirect(w, r, "/private", http.StatusFound)
			return
		}
		_, _ = w.Write(data.Bytes())
	}))
	defer server.Close()
	entries := []Entry{{Card: Card{PosterURL: server.URL + "/poster", BackdropURL: server.URL + "/redirect"}}}
	fetch := ArtworkFetcher(entries, server.URL)
	if _, _, err := fetch(context.Background(), server.URL+"/private"); err == nil || hits.Load() != 0 {
		t.Fatal("template URL outside the resolved artwork was fetched")
	}
	if _, _, err := fetch(context.Background(), server.URL+"/redirect"); err == nil || hits.Load() != 1 {
		t.Fatal("artwork redirect was followed")
	}
	result, mime, err := fetch(context.Background(), server.URL+"/poster")
	if err != nil || mime != "image/jpeg" {
		t.Fatalf("resolved server artwork: %s %v", mime, err)
	}
	if _, format, err := image.Decode(bytes.NewReader(result)); err != nil || format != "jpeg" {
		t.Fatalf("invalid image conversion: %s %v", format, err)
	}
	// Even an allowlisted image cannot grant private-network access to a
	// different origin: only the configured server has that permission.
	fetch = ArtworkFetcher(entries, "https://example.test")
	if _, _, err := fetch(context.Background(), server.URL+"/poster"); err == nil || hits.Load() != 2 {
		t.Fatal("non-server private artwork origin was fetched")
	}
}
