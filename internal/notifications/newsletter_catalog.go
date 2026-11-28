package notifications

import (
	"context"
	"strconv"
	"time"

	"github.com/Silo-Server/silo-server/internal/newsletter"
	"github.com/jackc/pgx/v5"
)

// newsletterEntries reads committed availability events, not metadata dates:
// an episode can exist in the catalog months before its file arrives.
// Titles and synopses come in the newsletter's language when the catalog has
// them, and each entry is marked with the recipient's reason to care.
func (s *System) newsletterEntries(ctx context.Context, tx pgx.Tx, k profileKey, p DeliveryPreferences, language string, from, to time.Time) ([]newsletter.Entry, error) {
	rows, err := tx.Query(ctx, `SELECT r.kind,COALESCE(r.item_id,''),COALESCE(r.series_id,''),COALESCE(r.episode_id,''),r.library_id,
 COALESCE(f.name,''),COALESCE(r.season_number,0),COALESCE(r.episode_number,0),r.created_at,
 COALESCE(ml.title,m.title),COALESCE(m.year,0),COALESCE(ml.overview,m.overview,''),COALESCE(m.rating_imdb,0),COALESCE(m.rating_tmdb,0),
 COALESCE(m.rating_rt_critic,0),COALESCE(m.rating_rt_audience,0),COALESCE(m.poster_path,''),
 COALESCE(el.title,e.title,''),COALESCE(el.overview,e.overview,''),COALESCE(e.still_path,'')
 FROM release_events r JOIN media_folders f ON f.id=r.library_id
 JOIN media_items m ON m.content_id=CASE WHEN r.kind='episode' THEN r.series_id ELSE r.item_id END
 LEFT JOIN episodes e ON e.content_id=r.episode_id
 LEFT JOIN LATERAL (SELECT NULLIF(l.title,'') AS title,NULLIF(l.overview,'') AS overview FROM media_item_localizations l
  WHERE l.content_id=m.content_id AND (lower(l.language)=$5::text OR lower(l.language) LIKE $5::text||'-%')
  ORDER BY lower(l.language)=$5::text DESC LIMIT 1) ml ON true
 LEFT JOIN LATERAL (SELECT NULLIF(l.title,'') AS title,NULLIF(l.overview,'') AS overview FROM episode_localizations l
  WHERE l.episode_content_id=e.content_id AND (lower(l.language)=$5::text OR lower(l.language) LIKE $5::text||'-%')
  ORDER BY lower(l.language)=$5::text DESC LIMIT 1) el ON true
 WHERE r.created_at>$1 AND r.created_at<=$2 AND ((r.kind='movie' AND $3) OR (r.kind='episode' AND $4))
 ORDER BY r.created_at DESC,r.id DESC LIMIT 2000`, from, to, p.Movies, p.Episodes, language)
	if err != nil {
		return nil, err
	}
	type candidate struct {
		entry         newsletter.Entry
		id            string
		library       int
		poster, still string
	}
	var candidates []candidate
	for rows.Next() {
		var c candidate
		var item, series, episode, seriesTitle, seriesOverview, episodeTitle, episodeOverview string
		var season, number int
		var at time.Time
		card := &c.entry.Card
		if err = rows.Scan(&card.Type, &item, &series, &episode, &c.library, &c.entry.Library.Name, &season, &number, &at, &seriesTitle, &card.Year, &seriesOverview, &card.RatingIMDB, &card.RatingTMDB, &card.RatingRTCritic, &card.RatingRTAudience, &c.poster, &episodeTitle, &episodeOverview, &c.still); err != nil {
			rows.Close()
			return nil, err
		}
		card.Title, card.Overview, card.ContentID = seriesTitle, seriesOverview, item
		c.id = item
		card.AddedAt = at.UTC().Format(time.RFC3339Nano)
		c.entry.Library.ID = strconv.Itoa(c.library)
		if card.Type == "episode" {
			c.id = series
			seriesCard := *card
			seriesCard.Type = "series"
			seriesCard.ContentID = series
			c.entry.Series = &newsletter.Detail{Card: seriesCard}
			card.ContentID, card.SeriesID, card.SeriesTitle = episode, series, seriesTitle
			card.Title, card.Overview = episodeTitle, episodeOverview
			card.SeasonNumber = &season
			card.EpisodeNumber = &number
		}
		candidates = append(candidates, c)
	}
	rows.Close()
	if err = rows.Err(); err != nil {
		return nil, err
	}
	access := newRecipientAccess(s.scopes)
	out := []newsletter.Entry{}
	seen := map[string]bool{}
	for _, c := range candidates {
		ok, err := access.canOpen(ctx, tx, k.userID, k.profileID, c.id, c.library)
		if err != nil {
			return nil, err
		}
		if !ok || seen[c.entry.Card.ContentID] {
			continue
		}
		seen[c.entry.Card.ContentID] = true
		if s.images != nil {
			c.entry.Card.PosterURL = s.absoluteArtworkURL(ctx, s.images.PresignImageURL(ctx, c.poster, "poster", ""))
			if c.entry.Series != nil {
				c.entry.Series.PosterURL = c.entry.Card.PosterURL
			}
			if c.still != "" {
				c.entry.Card.PosterURL = s.absoluteArtworkURL(ctx, s.images.PresignImageURL(ctx, c.still, "still", ""))
			}
		}
		out = append(out, c.entry)
	}
	if out, err = s.dropWatchedEntries(ctx, tx, k, out); err != nil {
		return nil, err
	}
	if err = s.markNewsletterPicks(ctx, tx, k, out); err != nil {
		return nil, err
	}
	return out, nil
}

// dropWatchedEntries leaves out the movies and episodes the recipient has
// already watched to the end, from the history or marked as watched.
func (s *System) dropWatchedEntries(ctx context.Context, tx pgx.Tx, k profileKey, entries []newsletter.Entry) ([]newsletter.Entry, error) {
	if len(entries) == 0 {
		return entries, nil
	}
	ids := make([]string, 0, len(entries))
	for _, e := range entries {
		ids = append(ids, e.Card.ContentID)
	}
	rows, err := tx.Query(ctx, `SELECT media_item_id FROM user_watch_progress WHERE user_id=$1 AND profile_id=$2 AND completed AND media_item_id=ANY($3)
 UNION SELECT media_item_id FROM user_watch_history WHERE user_id=$1 AND profile_id=$2 AND completed AND media_item_id=ANY($3)`, k.userID, k.profileID, ids)
	if err != nil {
		return nil, err
	}
	watched := map[string]bool{}
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			rows.Close()
			return nil, err
		}
		watched[id] = true
	}
	rows.Close()
	if err = rows.Err(); err != nil {
		return nil, err
	}
	kept := entries[:0]
	for _, e := range entries {
		if !watched[e.Card.ContentID] {
			kept = append(kept, e)
		}
	}
	return kept, nil
}

// markNewsletterPicks sets Entry.Pick: movies and shows on the recipient's
// watchlist, and shows the recipient has watched, started or finished.
func (s *System) markNewsletterPicks(ctx context.Context, tx pgx.Tx, k profileKey, entries []newsletter.Entry) error {
	var movieIDs, seriesIDs []string
	for _, e := range entries {
		if e.Card.Type == "episode" {
			seriesIDs = append(seriesIDs, e.Card.SeriesID)
		} else {
			movieIDs = append(movieIDs, e.Card.ContentID)
		}
	}
	if len(movieIDs) == 0 && len(seriesIDs) == 0 {
		return nil
	}
	picks := map[string]string{}
	rows, err := tx.Query(ctx, `SELECT media_item_id FROM user_watchlist WHERE user_id=$1 AND profile_id=$2 AND media_item_id=ANY($3)`,
		k.userID, k.profileID, append(append([]string{}, movieIDs...), seriesIDs...))
	if err != nil {
		return err
	}
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		picks[id] = newsletter.PickWatchlist
	}
	rows.Close()
	if err = rows.Err(); err != nil {
		return err
	}
	if len(seriesIDs) > 0 {
		// A show counts as watched once any of its episodes, or the show
		// itself, is in the history or has progress (Continue Watching).
		rows, err = tx.Query(ctx, `WITH seen AS (
 SELECT media_item_id FROM user_watch_history WHERE user_id=$1 AND profile_id=$2
 UNION SELECT media_item_id FROM user_watch_progress WHERE user_id=$1 AND profile_id=$2 AND (completed OR position_seconds>0))
 SELECT DISTINCT COALESCE(e.series_id,seen.media_item_id) FROM seen LEFT JOIN episodes e ON e.content_id=seen.media_item_id
 WHERE COALESCE(e.series_id,seen.media_item_id)=ANY($3)`, k.userID, k.profileID, seriesIDs)
		if err != nil {
			return err
		}
		for rows.Next() {
			var id string
			if err = rows.Scan(&id); err != nil {
				rows.Close()
				return err
			}
			picks[id] = newsletter.PickWatching
		}
		rows.Close()
		if err = rows.Err(); err != nil {
			return err
		}
	}
	for i := range entries {
		id := entries[i].Card.ContentID
		if entries[i].Card.Type == "episode" {
			id = entries[i].Card.SeriesID
		}
		entries[i].Pick = picks[id]
	}
	return nil
}

func (s *System) newsletterRecap(ctx context.Context, tx pgx.Tx, k profileKey, p DeliveryPreferences, settings newsletter.Settings, from, to time.Time) (*newsletter.RecapView, error) {
	plays := []newsletter.Play{}
	if p.Personal && settings.Recap.Personal {
		rows, err := tx.Query(ctx, `SELECT h.media_item_id,COALESCE(m.title,e.title,''),CASE WHEN e.content_id IS NOT NULL THEN 'episode' ELSE COALESCE(m.type,'') END,
 COALESCE(series.title,''),h.watched_seconds,h.completed,h.played_seconds,h.ended_at,COALESCE(e.series_id,m.content_id,'')
 FROM admin_playback_history h LEFT JOIN media_items m ON m.content_id=h.media_item_id
 LEFT JOIN episodes e ON e.content_id=h.media_item_id LEFT JOIN media_items series ON series.content_id=e.series_id
 WHERE h.user_id=$1 AND h.profile_id=$2 AND h.ended_at>=$3 AND h.ended_at<$4 ORDER BY h.ended_at DESC LIMIT 10000`, k.userID, k.profileID, from, to)
		if err != nil {
			return nil, err
		}
		type playback struct {
			play      newsletter.Play
			visibleID string
		}
		var history []playback
		for rows.Next() {
			var item playback
			play := &item.play
			var ended time.Time
			if err = rows.Scan(&play.MediaItemID, &play.MediaTitle, &play.MediaType, &play.SeriesTitle, &play.WatchedSeconds, &play.Completed, &play.PlayedSeconds, &ended, &item.visibleID); err != nil {
				rows.Close()
				return nil, err
			}
			play.EndedAt = ended.UTC().Format(time.RFC3339Nano)
			history = append(history, item)
		}
		rows.Close()
		if err = rows.Err(); err != nil {
			return nil, err
		}
		access := newRecipientAccess(s.scopes)
		for _, item := range history {
			allowed, err := access.canOpen(ctx, tx, k.userID, k.profileID, item.visibleID, 0)
			if err != nil {
				return nil, err
			}
			if allowed {
				plays = append(plays, item.play)
			}
		}
	}
	popular := []newsletter.PopularTitle{}
	if p.Popular && settings.Recap.Popular {
		// Viewers are profiles, so household members on one account count
		// separately; a title still needs at least two of them.
		rows, err := tx.Query(ctx, `SELECT COALESCE(e.series_id,m.content_id),COALESCE(series.title,m.title),COUNT(DISTINCT (h.user_id,h.profile_id))
 FROM admin_playback_history h LEFT JOIN episodes e ON e.content_id=h.media_item_id
 LEFT JOIN media_items m ON m.content_id=h.media_item_id LEFT JOIN media_items series ON series.content_id=e.series_id
 WHERE h.ended_at>=$1 AND h.ended_at<$2 AND (h.watched_seconds>=300 OR h.completed)
 AND COALESCE(e.series_id,m.content_id) IS NOT NULL
 GROUP BY 1,2 HAVING COUNT(DISTINCT (h.user_id,h.profile_id))>=$3 ORDER BY 3 DESC,1 LIMIT 100`, from, to, max(2, settings.Recap.PopularMinViewers))
		if err != nil {
			return nil, err
		}
		type title struct {
			id, name string
			viewers  int
		}
		var titles []title
		for rows.Next() {
			var t title
			if err = rows.Scan(&t.id, &t.name, &t.viewers); err != nil {
				rows.Close()
				return nil, err
			}
			titles = append(titles, t)
		}
		rows.Close()
		if err = rows.Err(); err != nil {
			return nil, err
		}
		access := newRecipientAccess(s.scopes)
		for _, t := range titles {
			ok, err := access.canOpen(ctx, tx, k.userID, k.profileID, t.id, 0)
			if err != nil {
				return nil, err
			}
			if ok {
				popular = append(popular, newsletter.PopularTitle{Name: t.name, Viewers: t.viewers})
			}
			if len(popular) >= settings.Recap.PopularCount {
				break
			}
		}
	}
	return newsletter.RecapFor(from, to, plays, popular, p.Personal && settings.Recap.Personal), nil
}
