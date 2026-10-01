// Package scrob implements the Scrob watch provider. Scrob
// (https://github.com/ellite/scrob) is self-hosted, so unlike Silo's other
// watch providers there is no fixed API host: each connection supplies its
// own server URL alongside the API key, both captured on connect through
// ConnectionConfigSchema/ConnectWithAPIKeyConfig and persisted on the
// connection (the URL in SecretAttributes, the key as AccessToken).
package scrob

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/Silo-Server/silo-server/internal/historyimport"
	"github.com/Silo-Server/silo-server/internal/logredact"
	"github.com/Silo-Server/silo-server/internal/plugins"
	"github.com/Silo-Server/silo-server/internal/userstore"
	"github.com/Silo-Server/silo-server/internal/watchsync"
)

const (
	// historyPageSize matches Scrob's GET /history page_size cap (1-100).
	historyPageSize   = 100
	maxErrorBodyBytes = 4 << 10

	connectionConfigKey = "scrob_server"
	serverURLFieldKey   = "server_url"
)

// errNotFound marks a 404 response so callers can tell "nothing to remove"
// apart from a real failure.
var errNotFound = errors.New("scrob resource not found")

func isNotFound(err error) bool {
	return errors.Is(err, errNotFound)
}

type Provider struct {
	client *http.Client
}

// NewProvider builds a Scrob provider. Scrob has no default host, so unlike
// Silo's other built-in providers this takes no base URL: every call reads
// the host from the connection it is given.
func NewProvider(client *http.Client) *Provider {
	if client == nil {
		client = &http.Client{Timeout: 20 * time.Second}
	}
	return &Provider{client: client}
}

func (p *Provider) Key() string {
	return "scrob"
}

func (p *Provider) DisplayName() string {
	return "Scrob"
}

func (p *Provider) Capabilities() watchsync.Capabilities {
	return watchsync.Capabilities{
		ImportWatched:   true,
		ExportWatched:   true,
		ExportUnwatched: true,
		ImportRatings:   true,
		ExportRatings:   true,
	}
}

func (p *Provider) HistorySource() userstore.WatchHistorySource {
	return userstore.WatchHistorySourceScrob
}

// ConnectionConfigSchema asks the connect UI for the self-hosted server URL
// alongside the API key. Required is set so the field always renders, not
// only once a value is entered.
func (p *Provider) ConnectionConfigSchema() []plugins.ConfigSchemaView {
	return []plugins.ConfigSchemaView{
		{
			Key:         connectionConfigKey,
			Title:       "Scrob server",
			Description: "Your self-hosted Scrob instance.",
			JSONSchema:  `{"type":"object","properties":{"` + serverURLFieldKey + `":{"type":"string"}},"required":["` + serverURLFieldKey + `"]}`,
			Required:    true,
			AdminForm: &plugins.AdminFormView{
				Fields: []plugins.AdminFormFieldView{
					{
						Key:         serverURLFieldKey,
						Label:       "Server URL",
						Description: "The base URL of your Scrob instance, e.g. https://scrob.example.com.",
						Control:     "TEXT",
						Placeholder: "https://scrob.example.com",
						Required:    true,
					},
				},
			},
		},
	}
}

// ConnectWithAPIKey satisfies APIKeyAuthProvider's gate check. The service
// always prefers ConnectWithAPIKeyConfig when a provider implements it (as
// this one does), so in practice this path is only reached with no
// configuration, which Scrob cannot work without.
func (p *Provider) ConnectWithAPIKey(ctx context.Context, apiKey string) (watchsync.TokenSet, watchsync.ProviderAccount, error) {
	return p.ConnectWithAPIKeyConfig(ctx, apiKey, nil)
}

func (p *Provider) ConnectWithAPIKeyConfig(ctx context.Context, apiKey string, config watchsync.ConnectionConfigValues) (watchsync.TokenSet, watchsync.ProviderAccount, error) {
	apiKey = strings.TrimSpace(apiKey)
	if apiKey == "" {
		return watchsync.TokenSet{}, watchsync.ProviderAccount{}, errors.New("scrob api key is required")
	}
	serverURL, err := serverURLFromConfig(config)
	if err != nil {
		return watchsync.TokenSet{}, watchsync.ProviderAccount{}, err
	}
	if _, err := p.verify(ctx, serverURL, apiKey); err != nil {
		return watchsync.TokenSet{}, watchsync.ProviderAccount{}, err
	}
	account := watchsync.ProviderAccount{ID: serverURL, Username: accountLabel(serverURL)}
	return watchsync.TokenSet{
		AccessToken:      apiKey,
		SecretAttributes: map[string]string{serverURLFieldKey: serverURL},
	}, account, nil
}

func (p *Provider) LookupAccount(ctx context.Context, _ watchsync.ServerConfig, conn watchsync.Connection) (watchsync.ProviderAccount, error) {
	serverURL, err := p.connectionServerURL(conn)
	if err != nil {
		return watchsync.ProviderAccount{}, err
	}
	if _, err := p.verify(ctx, serverURL, conn.AccessToken); err != nil {
		return watchsync.ProviderAccount{}, err
	}
	return watchsync.ProviderAccount{ID: serverURL, Username: accountLabel(serverURL)}, nil
}

// RefreshToken is a no-op: Scrob API keys don't expire on their own.
func (p *Provider) RefreshToken(_ context.Context, _ watchsync.ServerConfig, conn watchsync.Connection) (watchsync.TokenSet, error) {
	return watchsync.TokenSet{AccessToken: conn.AccessToken, SecretAttributes: conn.SecretAttributes}, nil
}

// verify confirms the server URL and API key work together by hitting the
// lightest authenticated read Scrob exposes. Scrob has no api-key-friendly
// whoami endpoint (GET /auth/me requires a session JWT), so ratings doubles
// as the connectivity check.
func (p *Provider) verify(ctx context.Context, serverURL, apiKey string) (scrobRatingsResponse, error) {
	var payload scrobRatingsResponse
	if err := p.do(ctx, http.MethodGet, serverURL, apiKey, "/ratings", nil, &payload); err != nil {
		return scrobRatingsResponse{}, err
	}
	return payload, nil
}

func accountLabel(serverURL string) string {
	parsed, err := url.Parse(serverURL)
	if err != nil || parsed.Host == "" {
		return serverURL
	}
	return parsed.Host
}

func serverURLFromConfig(config watchsync.ConnectionConfigValues) (string, error) {
	fields := config[connectionConfigKey]
	raw, _ := fields[serverURLFieldKey].(string)
	return normalizeServerURL(raw)
}

func normalizeServerURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", errors.New("scrob server URL is required")
	}
	if !strings.Contains(raw, "://") {
		raw = "https://" + raw
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return "", fmt.Errorf("scrob server URL %q is not a valid http(s) URL", raw)
	}
	parsed.Path = ""
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return strings.TrimRight(parsed.String(), "/"), nil
}

func (p *Provider) connectionServerURL(conn watchsync.Connection) (string, error) {
	raw := conn.SecretAttributes[serverURLFieldKey]
	if strings.TrimSpace(raw) == "" {
		return "", errors.New("scrob connection is missing its server URL; reconnect to set one")
	}
	return normalizeServerURL(raw)
}

// --- watched history ---

type scrobMedia struct {
	ID            int    `json:"id"`
	TMDBID        int    `json:"tmdb_id"`
	TVDBID        int    `json:"tvdb_id"`
	IMDbID        string `json:"imdb_id"`
	Type          string `json:"type"`
	Title         string `json:"title"`
	SeasonNumber  *int   `json:"season_number"`
	EpisodeNumber *int   `json:"episode_number"`
	ShowTitle     string `json:"show_title"`
	ShowTMDBID    int    `json:"show_tmdb_id"`
	ShowTVDBID    int    `json:"show_tvdb_id"`
}

type scrobHistoryEvent struct {
	Media     scrobMedia `json:"media"`
	WatchedAt *time.Time `json:"watched_at"`
}

type scrobHistoryResponse struct {
	Page       int                 `json:"page"`
	TotalPages int                 `json:"total_pages"`
	Results    []scrobHistoryEvent `json:"results"`
}

// FetchWatched reads every completed movie and episode watch event and
// aggregates them per item: PlayCount is the number of distinct events Scrob
// returned, LastWatchedAt the most recent one.
func (p *Provider) FetchWatched(ctx context.Context, _ watchsync.ServerConfig, conn watchsync.Connection) ([]watchsync.RemoteWatch, error) {
	plays, err := p.fetchHistoryEvents(ctx, conn)
	if err != nil {
		return nil, err
	}
	aggregated := make(map[string]*watchsync.RemoteWatch, len(plays))
	order := make([]string, 0, len(plays))
	for _, play := range plays {
		key := play.Kind + "\x00" + play.ProviderItemKey
		existing, ok := aggregated[key]
		if !ok {
			row := watchsync.RemoteWatch{
				Provider:        play.Provider,
				ProviderItemKey: play.ProviderItemKey,
				Kind:            play.Kind,
				Title:           play.Title,
				Year:            play.Year,
				IMDbID:          play.IMDbID,
				TMDBID:          play.TMDBID,
				TVDBID:          play.TVDBID,
				SeriesTitle:     play.SeriesTitle,
				SeriesYear:      play.SeriesYear,
				SeriesIMDbID:    play.SeriesIMDbID,
				SeriesTMDBID:    play.SeriesTMDBID,
				SeriesTVDBID:    play.SeriesTVDBID,
				SeasonNumber:    play.SeasonNumber,
				EpisodeNumber:   play.EpisodeNumber,
				PlayCount:       1,
			}
			watchedAt := play.WatchedAt
			row.LastWatchedAt = &watchedAt
			aggregated[key] = &row
			order = append(order, key)
			continue
		}
		existing.PlayCount++
		if existing.LastWatchedAt == nil || play.WatchedAt.After(*existing.LastWatchedAt) {
			watchedAt := play.WatchedAt
			existing.LastWatchedAt = &watchedAt
		}
	}
	rows := make([]watchsync.RemoteWatch, 0, len(order))
	for _, key := range order {
		rows = append(rows, *aggregated[key])
	}
	return rows, nil
}

func (p *Provider) FetchHistory(ctx context.Context, _ watchsync.ServerConfig, conn watchsync.Connection) ([]watchsync.RemotePlay, error) {
	return p.fetchHistoryEvents(ctx, conn)
}

func (p *Provider) fetchHistoryEvents(ctx context.Context, conn watchsync.Connection) ([]watchsync.RemotePlay, error) {
	serverURL, err := p.connectionServerURL(conn)
	if err != nil {
		return nil, err
	}
	var rows []watchsync.RemotePlay
	for page := 1; ; page++ {
		path := fmt.Sprintf("/history?page=%d&page_size=%d", page, historyPageSize)
		var payload scrobHistoryResponse
		if err := p.do(ctx, http.MethodGet, serverURL, conn.AccessToken, path, nil, &payload); err != nil {
			return nil, err
		}
		for _, event := range payload.Results {
			row, ok := historyPlayFromEvent(p.Key(), event)
			if !ok {
				continue
			}
			rows = append(rows, row)
		}
		if page >= payload.TotalPages || len(payload.Results) == 0 {
			break
		}
	}
	return rows, nil
}

func historyPlayFromEvent(providerKey string, event scrobHistoryEvent) (watchsync.RemotePlay, bool) {
	if event.WatchedAt == nil {
		return watchsync.RemotePlay{}, false
	}
	media := event.Media
	switch media.Type {
	case "movie":
		key := movieKey(media.TMDBID, media.TVDBID, media.IMDbID)
		if key == "" {
			return watchsync.RemotePlay{}, false
		}
		return watchsync.RemotePlay{
			Provider:        providerKey,
			ProviderItemKey: key,
			Kind:            historyimport.KindMovie,
			Title:           media.Title,
			IMDbID:          media.IMDbID,
			TMDBID:          intString(media.TMDBID),
			TVDBID:          intString(media.TVDBID),
			WatchedAt:       *event.WatchedAt,
		}, true
	case "episode":
		if media.SeasonNumber == nil || media.EpisodeNumber == nil {
			return watchsync.RemotePlay{}, false
		}
		key := episodeKey(media.ShowTMDBID, media.ShowTVDBID, *media.SeasonNumber, *media.EpisodeNumber, media.TMDBID, media.TVDBID, media.IMDbID)
		if key == "" {
			return watchsync.RemotePlay{}, false
		}
		return watchsync.RemotePlay{
			Provider:        providerKey,
			ProviderItemKey: key,
			Kind:            historyimport.KindEpisode,
			Title:           media.Title,
			IMDbID:          media.IMDbID,
			TMDBID:          intString(media.TMDBID),
			TVDBID:          intString(media.TVDBID),
			SeriesTitle:     media.ShowTitle,
			SeriesTMDBID:    intString(media.ShowTMDBID),
			SeriesTVDBID:    intString(media.ShowTVDBID),
			SeasonNumber:    *media.SeasonNumber,
			EpisodeNumber:   *media.EpisodeNumber,
			WatchedAt:       *event.WatchedAt,
		}, true
	default:
		return watchsync.RemotePlay{}, false
	}
}

type scrobWatchEventCreate struct {
	TMDBID        int    `json:"tmdb_id,omitempty"`
	TVDBID        int    `json:"tvdb_id,omitempty"`
	MediaType     string `json:"media_type"`
	WatchedAt     string `json:"watched_at,omitempty"`
	Completed     bool   `json:"completed"`
	SeriesTMDBID  int    `json:"series_tmdb_id,omitempty"`
	SeriesTVDBID  int    `json:"series_tvdb_id,omitempty"`
	SeasonNumber  *int   `json:"season_number,omitempty"`
	EpisodeNumber *int   `json:"episode_number,omitempty"`
}

// ExportHistory sends one POST /history per play: Scrob has no bulk watched
// endpoint. A per-item failure is recorded against that item and does not
// abort the rest of the batch.
func (p *Provider) ExportHistory(ctx context.Context, _ watchsync.ServerConfig, conn watchsync.Connection, plays []watchsync.LocalPlay) (watchsync.ExportResult, error) {
	serverURL, err := p.connectionServerURL(conn)
	if err != nil {
		return watchsync.ExportResult{}, err
	}
	result := watchsync.ExportResult{Failed: make(map[string]string)}
	for _, play := range plays {
		if play.HistoryID == "" {
			continue
		}
		body, ok := watchEventBody(play)
		if !ok {
			result.Failed[play.HistoryID] = "Scrob watched export requires a movie or episode with a TMDB or TVDB id"
			continue
		}
		var encoded bytes.Buffer
		if err := json.NewEncoder(&encoded).Encode(body); err != nil {
			return watchsync.ExportResult{}, fmt.Errorf("encode scrob watch event: %w", err)
		}
		if err := p.do(ctx, http.MethodPost, serverURL, conn.AccessToken, "/history", &encoded, nil); err != nil {
			result.Failed[play.HistoryID] = err.Error()
			continue
		}
		result.Sent = append(result.Sent, play.HistoryID)
	}
	if len(result.Failed) == 0 {
		result.Failed = nil
	}
	return result, nil
}

// RemoveHistory clears every watch event Scrob holds for each item. A title
// Scrob doesn't know about reports zero removed rather than an error, which
// Scrob folds into the same "ok" response, so a missing item just sends clean.
func (p *Provider) RemoveHistory(ctx context.Context, _ watchsync.ServerConfig, conn watchsync.Connection, plays []watchsync.LocalPlay) (watchsync.ExportResult, error) {
	serverURL, err := p.connectionServerURL(conn)
	if err != nil {
		return watchsync.ExportResult{}, err
	}
	result := watchsync.ExportResult{Failed: make(map[string]string)}
	for _, play := range plays {
		if play.HistoryID == "" {
			continue
		}
		path, ok := unwatchItemPath(play.Kind, play.TMDBID, play.TVDBID)
		if !ok {
			result.Failed[play.HistoryID] = "Scrob watched export requires a movie or episode with a TMDB or TVDB id"
			continue
		}
		if err := p.do(ctx, http.MethodDelete, serverURL, conn.AccessToken, path, nil, nil); err != nil {
			result.Failed[play.HistoryID] = err.Error()
			continue
		}
		result.Sent = append(result.Sent, play.HistoryID)
	}
	if len(result.Failed) == 0 {
		result.Failed = nil
	}
	return result, nil
}

func watchEventBody(play watchsync.LocalPlay) (scrobWatchEventCreate, bool) {
	if play.Kind != historyimport.KindMovie && play.Kind != historyimport.KindEpisode {
		return scrobWatchEventCreate{}, false
	}
	tmdbID := parseInt(play.TMDBID)
	tvdbID := parseInt(play.TVDBID)
	if tmdbID <= 0 && tvdbID <= 0 {
		return scrobWatchEventCreate{}, false
	}
	body := scrobWatchEventCreate{
		TMDBID:    tmdbID,
		TVDBID:    tvdbID,
		MediaType: play.Kind,
		Completed: true,
	}
	if !play.WatchedAt.IsZero() {
		body.WatchedAt = play.WatchedAt.UTC().Format(time.RFC3339)
	}
	if play.Kind == historyimport.KindEpisode {
		seriesTMDBID := parseInt(play.SeriesTMDBID)
		seriesTVDBID := parseInt(play.SeriesTVDBID)
		if seriesTMDBID <= 0 && seriesTVDBID <= 0 {
			return scrobWatchEventCreate{}, false
		}
		body.SeriesTMDBID = seriesTMDBID
		body.SeriesTVDBID = seriesTVDBID
		season, episode := play.SeasonNumber, play.EpisodeNumber
		body.SeasonNumber = &season
		body.EpisodeNumber = &episode
	}
	return body, true
}

func unwatchItemPath(kind, tmdbID, tvdbID string) (string, bool) {
	if kind != historyimport.KindMovie && kind != historyimport.KindEpisode {
		return "", false
	}
	tmdb, tvdb := parseInt(tmdbID), parseInt(tvdbID)
	if tmdb <= 0 && tvdb <= 0 {
		return "", false
	}
	query := url.Values{"media_type": {kind}}
	if tmdb > 0 {
		query.Set("tmdb_id", strconv.Itoa(tmdb))
	}
	if tvdb > 0 {
		query.Set("tvdb_id", strconv.Itoa(tvdb))
	}
	return "/history/item?" + query.Encode(), true
}

// --- HTTP plumbing ---

// do issues one request against a connection's Scrob server. The API key
// travels as the X-Api-Key header, never in the URL or query string, so
// unlike providers authenticated by URL parameter there is no credential to
// redact from request errors.
func (p *Provider) do(ctx context.Context, method, serverURL, apiKey, path string, body io.Reader, out any) error {
	if strings.TrimSpace(apiKey) == "" {
		return errors.New("scrob api key is missing")
	}
	req, err := http.NewRequestWithContext(ctx, method, serverURL+path, body)
	if err != nil {
		return fmt.Errorf("create scrob request: %w", logredact.SanitizeURLError(err))
	}
	req.Header.Set("X-Api-Key", apiKey)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := p.client.Do(req)
	if err != nil {
		return fmt.Errorf("send scrob request: %w", logredact.SanitizeURLError(err))
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return fmt.Errorf("scrob request %s %s rejected: status %d (check api key): %w", method, path, resp.StatusCode, watchsync.ErrInvalidCredential)
	}
	if resp.StatusCode == http.StatusNotFound {
		return fmt.Errorf("scrob request %s %s: status 404: %w", method, path, errNotFound)
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		if detail := responseErrorDetail(resp.Body); detail != "" {
			return fmt.Errorf("scrob request %s %s failed: status %d: %s", method, path, resp.StatusCode, detail)
		}
		return fmt.Errorf("scrob request %s %s failed: status %d", method, path, resp.StatusCode)
	}
	if out == nil || resp.StatusCode == http.StatusNoContent {
		return nil
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("decode scrob response: %w", err)
	}
	return nil
}

func responseErrorDetail(body io.Reader) string {
	raw, err := io.ReadAll(io.LimitReader(body, maxErrorBodyBytes+1))
	if err != nil {
		return ""
	}
	if len(raw) > maxErrorBodyBytes {
		raw = raw[:maxErrorBodyBytes]
	}
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 {
		return ""
	}
	var envelope struct {
		Detail json.RawMessage `json:"detail"`
	}
	if json.Unmarshal(raw, &envelope) == nil && len(envelope.Detail) > 0 {
		raw = bytes.TrimSpace(envelope.Detail)
	}
	var compact bytes.Buffer
	if json.Compact(&compact, raw) == nil {
		return compact.String()
	}
	return strings.Join(strings.Fields(string(raw)), " ")
}

// --- identity helpers ---

// movieKey and episodeKey mirror the id-priority convention Silo's other
// watch providers use for ProviderItemKey: prefer IMDb, then TMDB, then TVDB
// for movies; TVDB-first for shows/episodes, since TheTVDB episode ids stay
// stable across TheTVDB's alternate orderings while (season, number) do not.
func movieKey(tmdbID, tvdbID int, imdbID string) string {
	switch {
	case imdbID != "":
		return "imdb:" + imdbID
	case tmdbID > 0:
		return "tmdb:" + strconv.Itoa(tmdbID)
	case tvdbID > 0:
		return "tvdb:" + strconv.Itoa(tvdbID)
	default:
		return ""
	}
}

func episodeKey(showTMDBID, showTVDBID, season, episode, episodeTMDBID, episodeTVDBID int, episodeIMDbID string) string {
	switch {
	case episodeTVDBID > 0:
		return "tvdb:" + strconv.Itoa(episodeTVDBID)
	case episodeTMDBID > 0:
		return "tmdb:" + strconv.Itoa(episodeTMDBID)
	case episodeIMDbID != "":
		return "imdb:" + episodeIMDbID
	case showTVDBID > 0:
		return fmt.Sprintf("show:tvdb:%d:s%d:e%d", showTVDBID, season, episode)
	case showTMDBID > 0:
		return fmt.Sprintf("show:tmdb:%d:s%d:e%d", showTMDBID, season, episode)
	default:
		return ""
	}
}

func intString(value int) string {
	if value <= 0 {
		return ""
	}
	return strconv.Itoa(value)
}

func parseInt(value string) int {
	parsed, _ := strconv.Atoi(strings.TrimSpace(value))
	return parsed
}
