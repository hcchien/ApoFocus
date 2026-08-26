package folders

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

var ErrNotFound = errors.New("collection not found")

type PostgresRepository struct{ db *sql.DB }

func NewPostgresRepository(db *sql.DB) *PostgresRepository { return &PostgresRepository{db: db} }

func (r *PostgresRepository) List(ctx context.Context) ([]Collection, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT c.id::text, COALESCE(c.parent_id::text,''), c.name, c.kind, c.filter::text,
		       count(cp.photo_id), c.created_at
		FROM collections c LEFT JOIN collection_photos cp ON cp.collection_id=c.id
		GROUP BY c.id ORDER BY c.parent_id NULLS FIRST, lower(c.name)`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []Collection{}
	for rows.Next() {
		var collection Collection
		var filterJSON string
		if err := rows.Scan(&collection.ID, &collection.ParentID, &collection.Name, &collection.Kind, &filterJSON, &collection.Count, &collection.CreatedAt); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(filterJSON), &collection.Filter); err != nil {
			return nil, err
		}
		result = append(result, collection)
	}
	return result, rows.Err()
}

func (r *PostgresRepository) Create(ctx context.Context, input CreateInput) (Collection, error) {
	input.Name = strings.TrimSpace(input.Name)
	if input.Name == "" {
		return Collection{}, errors.New("collection name is required")
	}
	if input.Kind != "manual" && input.Kind != "smart" {
		return Collection{}, errors.New("collection kind must be manual or smart")
	}
	filter, err := json.Marshal(input.Filter)
	if err != nil {
		return Collection{}, err
	}
	var parent any
	if input.ParentID != "" {
		parent = input.ParentID
	}
	var result Collection
	var filterJSON string
	err = r.db.QueryRowContext(ctx, `
		INSERT INTO collections(parent_id,name,kind,filter) VALUES($1,$2,$3,$4::jsonb)
		RETURNING id::text, COALESCE(parent_id::text,''), name, kind, filter::text, created_at`,
		parent, input.Name, input.Kind, filter).Scan(&result.ID, &result.ParentID, &result.Name, &result.Kind, &filterJSON, &result.CreatedAt)
	if err != nil {
		return Collection{}, fmt.Errorf("create collection: %w", err)
	}
	if err := json.Unmarshal([]byte(filterJSON), &result.Filter); err != nil {
		return Collection{}, err
	}
	return result, nil
}

func (r *PostgresRepository) AddPhotos(ctx context.Context, collectionID string, photoIDs []string) error {
	var kind string
	if err := r.db.QueryRowContext(ctx, `SELECT kind FROM collections WHERE id=$1`, collectionID).Scan(&kind); errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	} else if err != nil {
		return err
	}
	if kind != "manual" {
		return errors.New("photos can only be added directly to a manual collection")
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	for index, photoID := range photoIDs {
		if _, err := tx.ExecContext(ctx, `INSERT INTO collection_photos(collection_id,photo_id,position) VALUES($1,$2,$3) ON CONFLICT DO NOTHING`, collectionID, photoID, index); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (r *PostgresRepository) PhotoIDs(ctx context.Context, collectionID string) ([]string, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT photo_id::text FROM collection_photos WHERE collection_id=$1 ORDER BY position,added_at`, collectionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		result = append(result, id)
	}
	return result, rows.Err()
}
