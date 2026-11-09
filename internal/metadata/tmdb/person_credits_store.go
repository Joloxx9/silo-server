package tmdb

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// personCreditsStoreFormat versions the stored JSON. A row in another format
// reads as a miss and is replaced by the next fetch.
const personCreditsStoreFormat = 1

// personCreditsStoreRetention is how long a filmography nobody opens stays
// stored before a later save prunes it.
const personCreditsStoreRetention = 60 * 24 * time.Hour

// PersonCreditsStore keeps fetched filmographies across restarts and nodes.
type PersonCreditsStore interface {
	// LoadPersonCredits returns the stored filmography and when TMDB
	// returned it; ok is false when none is stored.
	LoadPersonCredits(ctx context.Context, personID int) (credits []PersonCredit, fetchedAt time.Time, ok bool, err error)
	SavePersonCredits(ctx context.Context, personID int, credits []PersonCredit, fetchedAt time.Time) error
}

// PGPersonCreditsStore is the Postgres PersonCreditsStore.
type PGPersonCreditsStore struct {
	pool *pgxpool.Pool
}

// NewPGPersonCreditsStore stores filmographies in tmdb_person_credits_cache.
func NewPGPersonCreditsStore(pool *pgxpool.Pool) *PGPersonCreditsStore {
	return &PGPersonCreditsStore{pool: pool}
}

func (s *PGPersonCreditsStore) LoadPersonCredits(ctx context.Context, personID int) ([]PersonCredit, time.Time, bool, error) {
	var raw []byte
	var fetchedAt time.Time
	err := s.pool.QueryRow(ctx, `
		SELECT credits, fetched_at FROM tmdb_person_credits_cache
		WHERE person_id = $1 AND format = $2`, personID, personCreditsStoreFormat).Scan(&raw, &fetchedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, time.Time{}, false, nil
	}
	if err != nil {
		return nil, time.Time{}, false, fmt.Errorf("loading person credits: %w", err)
	}
	var credits []PersonCredit
	if err := json.Unmarshal(raw, &credits); err != nil {
		return nil, time.Time{}, false, nil
	}
	return credits, fetchedAt, true, nil
}

func (s *PGPersonCreditsStore) SavePersonCredits(ctx context.Context, personID int, credits []PersonCredit, fetchedAt time.Time) error {
	if credits == nil {
		credits = []PersonCredit{}
	}
	raw, err := json.Marshal(credits)
	if err != nil {
		return fmt.Errorf("encoding person credits: %w", err)
	}
	if _, err := s.pool.Exec(ctx, `
		INSERT INTO tmdb_person_credits_cache (person_id, format, credits, fetched_at)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (person_id) DO UPDATE
		SET format = EXCLUDED.format, credits = EXCLUDED.credits, fetched_at = EXCLUDED.fetched_at
		WHERE tmdb_person_credits_cache.fetched_at <= EXCLUDED.fetched_at`,
		personID, personCreditsStoreFormat, raw, fetchedAt); err != nil {
		return fmt.Errorf("saving person credits: %w", err)
	}
	if _, err := s.pool.Exec(ctx, `DELETE FROM tmdb_person_credits_cache WHERE fetched_at < $1`,
		fetchedAt.Add(-personCreditsStoreRetention)); err != nil {
		return fmt.Errorf("pruning person credits: %w", err)
	}
	return nil
}

// PeopleDueForRefresh returns the TMDB IDs of people credited in the library
// whose stored filmography is missing or was fetched before staleBefore,
// those credited on the most titles first.
func (s *PGPersonCreditsStore) PeopleDueForRefresh(ctx context.Context, staleBefore time.Time, limit int) ([]int, error) {
	rows, err := s.pool.Query(ctx, `
		WITH credited AS (
			SELECT p.tmdb_id::integer AS person_id, count(DISTINCT ip.content_id) AS titles
			FROM people p
			JOIN item_people ip ON ip.person_id = p.id
			WHERE p.tmdb_id ~ '^[1-9][0-9]{0,8}$'
			GROUP BY 1
		)
		SELECT credited.person_id
		FROM credited
		LEFT JOIN tmdb_person_credits_cache c ON c.person_id = credited.person_id
		WHERE c.person_id IS NULL OR c.format <> $1 OR c.fetched_at < $2
		ORDER BY credited.titles DESC, credited.person_id
		LIMIT $3`, personCreditsStoreFormat, staleBefore, limit)
	if err != nil {
		return nil, fmt.Errorf("listing people due for credits refresh: %w", err)
	}
	defer rows.Close()
	var ids []int
	for rows.Next() {
		var id int
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scanning person due for credits refresh: %w", err)
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}
