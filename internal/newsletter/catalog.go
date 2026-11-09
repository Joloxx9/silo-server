package newsletter

import "time"

type Card struct {
	ContentID              string   `json:"content_id"`
	Type                   string   `json:"type"`
	Title                  string   `json:"title"`
	Year                   int      `json:"year"`
	Overview               string   `json:"overview"`
	AddedAt                string   `json:"added_at"`
	ReleaseDate            string   `json:"release_date"`
	Runtime                int      `json:"runtime"`
	DurationSeconds        float64  `json:"duration_seconds"`
	ContentRating          string   `json:"content_rating"`
	Genres                 []string `json:"genres"`
	Studios                []string `json:"studios"`
	Networks               []string `json:"networks"`
	RatingTMDB             float64  `json:"rating_tmdb"`
	RatingIMDB             float64  `json:"rating_imdb"`
	RatingRTCritic         int      `json:"rating_rt_critic"`
	RatingRTAudience       int      `json:"rating_rt_audience"`
	PosterURL              string   `json:"poster_url"`
	BackdropURL            string   `json:"backdrop_url"`
	PosterIsEpisodeStill   bool     `json:"poster_is_episode_still"`
	BackdropIsEpisodeStill bool     `json:"backdrop_is_episode_still"`
	SeriesID               string   `json:"series_id"`
	SeriesTitle            string   `json:"series_title"`
	SeasonNumber           *int     `json:"season_number"`
	EpisodeNumber          *int     `json:"episode_number"`
	ShowStatus             string   `json:"show_status"`
}

// Detail is the subset of a catalog item detail the plugin reads.
type Detail struct {
	Card
	Tagline      string       `json:"tagline"`
	FirstAirDate string       `json:"first_air_date"`
	AirDate      string       `json:"air_date"`
	SeasonCount  int          `json:"season_count"`
	EpisodeCount int          `json:"episode_count"`
	LogoURL      string       `json:"logo_url"`
	Cast         []CastMember `json:"cast"`
}

type CastMember struct {
	Name      string `json:"name"`
	Character string `json:"character"`
}

func (c Card) added() time.Time {
	t, err := time.Parse(time.RFC3339Nano, c.AddedAt)
	if err != nil {
		return time.Time{}
	}
	return t
}
