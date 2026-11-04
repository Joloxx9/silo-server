package watchsync

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
)

func (r *PostgresRepository) ListCollectionLinks(ctx context.Context, connectionID string) ([]CollectionLink, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT connection_id::text, collection_key, collection_id, name
		FROM watch_provider_collection_links
		WHERE connection_id = $1::uuid
		ORDER BY collection_key
	`, connectionID)
	if err != nil {
		return nil, fmt.Errorf("list collection links: %w", err)
	}
	return scanCollectionLinks(rows)
}

func (r *PostgresRepository) ListCollectionLinksForCollection(ctx context.Context, collectionID string) ([]CollectionLink, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT connection_id::text, collection_key, collection_id, name
		FROM watch_provider_collection_links
		WHERE collection_id = $1
		ORDER BY connection_id, collection_key
	`, collectionID)
	if err != nil {
		return nil, fmt.Errorf("list collection links for collection: %w", err)
	}
	return scanCollectionLinks(rows)
}

func (r *PostgresRepository) UpsertCollectionLink(ctx context.Context, link CollectionLink) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO watch_provider_collection_links (connection_id, collection_key, collection_id, name)
		VALUES ($1::uuid, $2, $3, $4)
		ON CONFLICT (connection_id, collection_key) DO UPDATE SET
			collection_id = EXCLUDED.collection_id,
			name = EXCLUDED.name,
			updated_at = now()
	`, link.ConnectionID, link.CollectionKey, link.CollectionID, link.Name)
	if err != nil {
		return fmt.Errorf("upsert collection link: %w", err)
	}
	return nil
}

func (r *PostgresRepository) DeleteCollectionLink(ctx context.Context, connectionID, collectionKey string) error {
	return pgx.BeginFunc(ctx, r.pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `
			DELETE FROM watch_provider_list_items
			WHERE connection_id = $1::uuid AND list_kind = $2
		`, connectionID, string(collectionListKind(collectionKey))); err != nil {
			return fmt.Errorf("delete collection list items: %w", err)
		}
		if _, err := tx.Exec(ctx, `
			DELETE FROM watch_provider_collection_links
			WHERE connection_id = $1::uuid AND collection_key = $2
		`, connectionID, collectionKey); err != nil {
			return fmt.Errorf("delete collection link: %w", err)
		}
		return nil
	})
}

func (r *PostgresRepository) DeleteCollectionLinks(ctx context.Context, connectionID string) error {
	return pgx.BeginFunc(ctx, r.pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `
			DELETE FROM watch_provider_list_items
			WHERE connection_id = $1::uuid AND list_kind LIKE $2
		`, connectionID, collectionListKindPrefix+"%"); err != nil {
			return fmt.Errorf("delete collection list items: %w", err)
		}
		if _, err := tx.Exec(ctx, `
			DELETE FROM watch_provider_collection_links WHERE connection_id = $1::uuid
		`, connectionID); err != nil {
			return fmt.Errorf("delete collection links: %w", err)
		}
		return nil
	})
}

func scanCollectionLinks(rows pgx.Rows) ([]CollectionLink, error) {
	defer rows.Close()
	var links []CollectionLink
	for rows.Next() {
		var link CollectionLink
		if err := rows.Scan(&link.ConnectionID, &link.CollectionKey, &link.CollectionID, &link.Name); err != nil {
			return nil, fmt.Errorf("scan collection link: %w", err)
		}
		links = append(links, link)
	}
	return links, rows.Err()
}
