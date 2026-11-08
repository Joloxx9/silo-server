package tmdb

import (
	"cmp"
	"context"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"
)

// personCreditsCacheTTL keeps a person's filmography long enough that paging
// through a person page, or several profiles opening it, share one fetch,
// while a new role reaches the page within the hour.
const personCreditsCacheTTL = 30 * time.Minute

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

// GetPersonCredits returns a person's movies and series from TMDB's combined
// credits, newest first with undated (announced) titles leading. Adult titles
// and talk or news shows are left out. The list is cached for
// personCreditsCacheTTL; each caller gets its own copy.
func (c *Client) GetPersonCredits(ctx context.Context, personID int) ([]PersonCredit, error) {
	if personID <= 0 {
		return nil, fmt.Errorf("tmdb: person id must be > 0 (got %d)", personID)
	}
	key := "person_credits:" + strconv.Itoa(personID)
	if cached, ok := c.cachedPersonCredits(key); ok {
		return clonePersonCredits(cached), nil
	}
	resultCh := c.cacheGroup.DoChan(key, func() (any, error) {
		if cached, ok := c.cachedPersonCredits(key); ok {
			return cached, nil
		}
		fetchCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), sharedFetchTimeout)
		defer cancel()
		var resp personCreditsResponse
		if err := c.doGet(fetchCtx, fmt.Sprintf("/person/%d/combined_credits", personID), &resp); err != nil {
			return nil, err
		}
		credits := normalizePersonCredits(resp)
		if c.personCreditsCache != nil {
			c.personCreditsCache.Set(key, &credits, personCreditsCacheTTL)
		}
		return credits, nil
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
