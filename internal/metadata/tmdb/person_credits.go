package tmdb

import (
	"cmp"
	"context"
	"fmt"
	"log/slog"
	"slices"
	"strconv"
	"strings"
	"time"
)

// personCreditsCacheTTL is how long a process keeps a filmography in memory
// before reading the stored copy again.
const personCreditsCacheTTL = 30 * time.Minute

// PersonCreditsFreshFor is how old a stored filmography may be before a
// person page asks TMDB again. An older copy is still shown at once and
// refreshed in the background, so a new role reaches the page on the visit
// after it lands on TMDB.
const PersonCreditsFreshFor = 3 * 24 * time.Hour

// PersonCredit is one title in a person's filmography: the title as request
// surfaces show it, and what the person did on it. A title the person both
// acted in and worked on appears once, with both.
type PersonCredit struct {
	MediaResult
	// Character is the role played, empty for a crew-only credit. Several
	// roles on one title are joined with " / ".
	Character string
	// Jobs are the crew jobs, such as "Director" or "Screenplay", in TMDB's
	// order without repeats.
	Jobs []string
	// EpisodeCount is how many episodes of a series the person is credited
	// on; zero for movies and when TMDB does not say.
	EpisodeCount int
}

// talkAndNewsGenres are TMDB's TV genres for talk shows and news. Guest spots
// on them fill a filmography with appearances as "Self", so they are left out.
var talkAndNewsGenres = map[int]struct{}{10767: {}, 10763: {}}

type personCreditResponse struct {
	ID           int     `json:"id"`
	MediaType    string  `json:"media_type"`
	Title        string  `json:"title"`
	Name         string  `json:"name"`
	Overview     string  `json:"overview"`
	PosterPath   string  `json:"poster_path"`
	BackdropPath string  `json:"backdrop_path"`
	ReleaseDate  string  `json:"release_date"`
	FirstAirDate string  `json:"first_air_date"`
	Popularity   float64 `json:"popularity"`
	VoteAverage  float64 `json:"vote_average"`
	GenreIDs     []int   `json:"genre_ids"`
	Adult        bool    `json:"adult"`
	Character    string  `json:"character"`
	Job          string  `json:"job"`
	EpisodeCount int     `json:"episode_count"`
}

type personCreditsResponse struct {
	Cast []personCreditResponse `json:"cast"`
	Crew []personCreditResponse `json:"crew"`
}

// SetPersonCreditsStore keeps fetched filmographies in store, so they
// survive restarts and are shared by every node. Call before serving.
func (c *Client) SetPersonCreditsStore(store PersonCreditsStore) {
	c.personCreditsStore = store
}

// GetPersonCredits returns a person's movies and series from TMDB's combined
// credits, newest first with undated (announced) titles leading. Adult titles
// and talk or news shows are left out. It answers from memory, then from the
// PersonCreditsStore, and asks TMDB only when neither holds the person; a
// stored copy older than PersonCreditsFreshFor is returned and refreshed in
// the background. Each caller gets its own copy.
func (c *Client) GetPersonCredits(ctx context.Context, personID int) ([]PersonCredit, error) {
	if personID <= 0 {
		return nil, fmt.Errorf("tmdb: person id must be > 0 (got %d)", personID)
	}
	key := personCreditsKey(personID)
	if cached, ok := c.cachedPersonCredits(key); ok {
		return clonePersonCredits(cached), nil
	}
	resultCh := c.cacheGroup.DoChan(key, func() (any, error) {
		if cached, ok := c.cachedPersonCredits(key); ok {
			return cached, nil
		}
		fetchCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), sharedFetchTimeout)
		defer cancel()
		if stored, fetchedAt, ok := c.loadStoredPersonCredits(fetchCtx, personID); ok {
			c.rememberPersonCredits(key, stored)
			if time.Since(fetchedAt) >= PersonCreditsFreshFor {
				c.refreshPersonCreditsInBackground(personID)
			}
			return stored, nil
		}
		return c.fetchPersonCredits(fetchCtx, personID)
	})
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case result := <-resultCh:
		if result.Err != nil {
			return nil, result.Err
		}
		credits, ok := result.Val.([]PersonCredit)
		if !ok {
			return nil, fmt.Errorf("tmdb: invalid cached person credits")
		}
		return clonePersonCredits(credits), nil
	}
}

// RefreshPersonCredits fetches a person's filmography from TMDB and stores
// it, replacing any cached copy.
func (c *Client) RefreshPersonCredits(ctx context.Context, personID int) error {
	if personID <= 0 {
		return fmt.Errorf("tmdb: person id must be > 0 (got %d)", personID)
	}
	_, err, _ := c.cacheGroup.Do("refresh:"+personCreditsKey(personID), func() (any, error) {
		return c.fetchPersonCredits(ctx, personID)
	})
	return err
}

func personCreditsKey(personID int) string {
	return "person_credits:" + strconv.Itoa(personID)
}

func (c *Client) fetchPersonCredits(ctx context.Context, personID int) ([]PersonCredit, error) {
	var resp personCreditsResponse
	if err := c.doGet(ctx, fmt.Sprintf("/person/%d/combined_credits", personID), &resp); err != nil {
		return nil, err
	}
	credits := normalizePersonCredits(resp)
	c.rememberPersonCredits(personCreditsKey(personID), credits)
	if c.personCreditsStore != nil {
		if err := c.personCreditsStore.SavePersonCredits(ctx, personID, credits, time.Now()); err != nil {
			slog.WarnContext(ctx, "tmdb: storing person credits failed", "component", "tmdb", "person_id", personID, "error", err)
		}
	}
	return credits, nil
}

func (c *Client) loadStoredPersonCredits(ctx context.Context, personID int) ([]PersonCredit, time.Time, bool) {
	if c.personCreditsStore == nil {
		return nil, time.Time{}, false
	}
	credits, fetchedAt, ok, err := c.personCreditsStore.LoadPersonCredits(ctx, personID)
	if err != nil {
		slog.WarnContext(ctx, "tmdb: reading stored person credits failed", "component", "tmdb", "person_id", personID, "error", err)
		return nil, time.Time{}, false
	}
	return credits, fetchedAt, ok
}

func (c *Client) refreshPersonCreditsInBackground(personID int) {
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), sharedFetchTimeout)
		defer cancel()
		if err := c.RefreshPersonCredits(ctx, personID); err != nil {
			slog.Debug("tmdb: background person credits refresh failed", "component", "tmdb", "person_id", personID, "error", err)
		}
	}()
}

func (c *Client) rememberPersonCredits(key string, credits []PersonCredit) {
	if c.personCreditsCache != nil {
		c.personCreditsCache.Set(key, &credits, personCreditsCacheTTL)
	}
}

func (c *Client) cachedPersonCredits(key string) ([]PersonCredit, bool) {
	if c.personCreditsCache == nil {
		return nil, false
	}
	cached, ok := c.personCreditsCache.Get(key)
	if !ok || cached == nil {
		return nil, false
	}
	return *cached, true
}

func clonePersonCredits(credits []PersonCredit) []PersonCredit {
	out := slices.Clone(credits)
	for i := range out {
		out[i].Jobs = slices.Clone(out[i].Jobs)
	}
	return out
}

// normalizePersonCredits merges cast and crew entries by title and orders
// them for a filmography.
func normalizePersonCredits(resp personCreditsResponse) []PersonCredit {
	index := map[string]int{}
	var out []PersonCredit
	add := func(entry personCreditResponse, cast bool) {
		media, ok := personCreditMedia(entry)
		if !ok {
			return
		}
		key := media.MediaType + ":" + strconv.Itoa(media.ID)
		i, seen := index[key]
		if !seen {
			i = len(out)
			index[key] = i
			out = append(out, PersonCredit{MediaResult: media})
		}
		credit := &out[i]
		if cast {
			character := strings.TrimSpace(entry.Character)
			if character != "" && !slices.Contains(strings.Split(credit.Character, " / "), character) {
				if credit.Character != "" {
					credit.Character += " / "
				}
				credit.Character += character
			}
		} else if job := strings.TrimSpace(entry.Job); job != "" && !slices.Contains(credit.Jobs, job) {
			credit.Jobs = append(credit.Jobs, job)
		}
		credit.EpisodeCount = max(credit.EpisodeCount, entry.EpisodeCount)
	}
	for _, entry := range resp.Cast {
		add(entry, true)
	}
	for _, entry := range resp.Crew {
		add(entry, false)
	}
	slices.SortStableFunc(out, func(a, b PersonCredit) int {
		// Undated titles are announced ones and lead; then newest first.
		if (a.ReleaseDate == "") != (b.ReleaseDate == "") {
			if a.ReleaseDate == "" {
				return -1
			}
			return 1
		}
		if byDate := cmp.Compare(b.ReleaseDate, a.ReleaseDate); byDate != 0 {
			return byDate
		}
		return cmp.Compare(b.Popularity, a.Popularity)
	})
	return out
}

func personCreditMedia(entry personCreditResponse) (MediaResult, bool) {
	if entry.ID <= 0 || entry.Adult {
		return MediaResult{}, false
	}
	switch entry.MediaType {
	case "movie":
		return movieResult(mediaMovieResponse{
			ID: entry.ID, Title: entry.Title, Overview: entry.Overview, PosterPath: entry.PosterPath,
			BackdropPath: entry.BackdropPath, ReleaseDate: entry.ReleaseDate, Popularity: entry.Popularity,
			VoteAverage: entry.VoteAverage,
		}), true
	case "tv":
		for _, genre := range entry.GenreIDs {
			if _, skip := talkAndNewsGenres[genre]; skip {
				return MediaResult{}, false
			}
		}
		return tvResult(mediaTVResponse{
			ID: entry.ID, Name: entry.Name, Overview: entry.Overview, PosterPath: entry.PosterPath,
			BackdropPath: entry.BackdropPath, FirstAirDate: entry.FirstAirDate, Popularity: entry.Popularity,
			VoteAverage: entry.VoteAverage,
		}), true
	default:
		return MediaResult{}, false
	}
}
