package newsletter

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/jpeg"
	_ "image/png"
	"io"
	"net/http"
	"net/url"
	"sync/atomic"
	"time"

	"github.com/Silo-Server/silo-server/internal/netguard"
	_ "golang.org/x/image/webp"
)

// ArtworkFetcher accepts only server-resolved artwork belonging to this email.
// Templates cannot turn arbitrary image URLs into server-side requests. Only
// the configured server origin may resolve to a private network address.
func ArtworkFetcher(entries []Entry, serverURL string, logos ...string) func(context.Context, string) ([]byte, string, error) {
	allowed := map[string]bool{}
	for _, logo := range logos {
		if logo != "" {
			allowed[logo] = true
		}
	}
	for _, entry := range entries {
		for _, raw := range []string{entry.Card.PosterURL, entry.Card.BackdropURL} {
			if raw != "" {
				allowed[raw] = true
			}
		}
		for _, detail := range []*Detail{entry.Series, entry.Movie, entry.Episode} {
			if detail != nil {
				for _, raw := range []string{detail.PosterURL, detail.BackdropURL, detail.LogoURL} {
					if raw != "" {
						allowed[raw] = true
					}
				}
			}
		}
	}
	server, _ := url.Parse(serverURL)
	client := netguard.NewClient(5 * time.Second)
	var embeddedBytes atomic.Int64
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	return func(ctx context.Context, raw string) ([]byte, string, error) {
		if !allowed[raw] {
			return nil, "", fmt.Errorf("image is not part of this newsletter")
		}
		u, err := url.Parse(raw)
		if err != nil {
			return nil, "", err
		}
		if server != nil && u.Host == server.Host && u.Scheme == server.Scheme {
			ctx = netguard.WithPrivateAccess(ctx)
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, raw, nil)
		if err != nil {
			return nil, "", err
		}
		resp, err := client.Do(req)
		if err != nil {
			return nil, "", err
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return nil, "", fmt.Errorf("artwork response %d", resp.StatusCode)
		}
		data, err := io.ReadAll(io.LimitReader(resp.Body, 5<<20+1))
		if err != nil {
			return nil, "", err
		}
		if len(data) > 5<<20 {
			return nil, "", fmt.Errorf("artwork too large")
		}
		cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
		if err != nil {
			return nil, "", err
		}
		if cfg.Width <= 0 || cfg.Height <= 0 || int64(cfg.Width)*int64(cfg.Height) > 16_000_000 {
			return nil, "", fmt.Errorf("artwork dimensions too large")
		}
		img, _, err := image.Decode(bytes.NewReader(data))
		if err != nil {
			return nil, "", err
		}
		var out bytes.Buffer
		if err = jpeg.Encode(&out, img, &jpeg.Options{Quality: 85}); err != nil {
			return nil, "", err
		}
		if embeddedBytes.Add(int64(out.Len())) > 20<<20 {
			return nil, "", fmt.Errorf("newsletter artwork budget exceeded")
		}
		return out.Bytes(), "image/jpeg", nil
	}
}
