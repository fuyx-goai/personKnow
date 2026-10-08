package library

import (
	"context"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	platformpostgres "knowledge-base/internal/platform/postgres"
)

type PostgresRepository struct {
	pool *pgxpool.Pool
}

func NewPostgresRepository(pool *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{pool: pool}
}

func (repo *PostgresRepository) ListOwned(ctx context.Context, userID uuid.UUID, filter ListFilter) ([]Library, error) {
	return repo.list(ctx, `k.owner_user_id=$1`, userID, filter)
}

func (repo *PostgresRepository) ListPublic(ctx context.Context, userID uuid.UUID, filter ListFilter) ([]Library, error) {
	return repo.list(ctx, `k.owner_user_id<>$1 AND k.visibility='public'`, userID, filter)
}

func (repo *PostgresRepository) list(ctx context.Context, ownership string, userID uuid.UUID, filter ListFilter) ([]Library, error) {
	limit := filter.Limit
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	query := `SELECT k.id,k.owner_user_id,u.nickname,k.name,k.category,k.description,k.visibility,k.status,
        k.document_count,k.chunk_count,k.storage_bytes,k.indexed_tokens,k.created_at,k.updated_at
        FROM knowledge_bases k JOIN users u ON u.id=k.owner_user_id
        WHERE ` + ownership + ` AND k.deleted_at IS NULL AND k.status='active'
        AND ($2='' OR k.name ILIKE '%'||$2||'%' OR k.description ILIKE '%'||$2||'%')
        AND ($3='' OR k.category=$3) ORDER BY k.updated_at DESC,k.id LIMIT $4`
	rows, err := repo.pool.Query(ctx, query, userID, strings.TrimSpace(filter.Keyword), strings.TrimSpace(filter.Category), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var libraries []Library
	for rows.Next() {
		library, err := scanLibrary(rows)
		if err != nil {
			return nil, err
		}
		libraries = append(libraries, library)
	}
	return libraries, rows.Err()
}

func (repo *PostgresRepository) Create(ctx context.Context, library Library, settings RetrievalSettings) error {
	return platformpostgres.WithinTx(ctx, repo.pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `INSERT INTO knowledge_bases
            (id,owner_user_id,name,category,description,visibility,status,created_at,updated_at)
            VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)`, library.ID, library.OwnerUserID, library.Name,
			library.Category, library.Description, library.Visibility, library.Status, library.CreatedAt, library.UpdatedAt)
		if err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `INSERT INTO library_retrieval_settings
            (library_id,chunk_size,chunk_overlap,top_k,similarity_threshold,updated_by)
            VALUES($1,$2,$3,$4,$5,$6)`, settings.LibraryID, settings.ChunkSize, settings.ChunkOverlap,
			settings.TopK, settings.SimilarityThreshold, settings.UpdatedBy)
		return err
	})
}

func (repo *PostgresRepository) Get(ctx context.Context, libraryID uuid.UUID) (Library, error) {
	row := repo.pool.QueryRow(ctx, `SELECT k.id,k.owner_user_id,u.nickname,k.name,k.category,k.description,k.visibility,k.status,
        k.document_count,k.chunk_count,k.storage_bytes,k.indexed_tokens,k.created_at,k.updated_at
        FROM knowledge_bases k JOIN users u ON u.id=k.owner_user_id WHERE k.id=$1 AND k.deleted_at IS NULL`, libraryID)
	library, err := scanLibrary(row)
	if err == pgx.ErrNoRows {
		return Library{}, ErrNotFound
	}
	return library, err
}

func (repo *PostgresRepository) Update(ctx context.Context, library Library) error {
	result, err := repo.pool.Exec(ctx, `UPDATE knowledge_bases SET name=$2,category=$3,description=$4,
        visibility=$5,updated_at=$6 WHERE id=$1 AND deleted_at IS NULL`, library.ID, library.Name,
		library.Category, library.Description, library.Visibility, library.UpdatedAt)
	return affectedOrNotFound(result.RowsAffected(), err)
}

func (repo *PostgresRepository) MarkDeleting(ctx context.Context, libraryID uuid.UUID) error {
	result, err := repo.pool.Exec(ctx, `UPDATE knowledge_bases SET status='deleting',updated_at=now()
        WHERE id=$1 AND deleted_at IS NULL AND status<>'deleting'`, libraryID)
	return affectedOrNotFound(result.RowsAffected(), err)
}

func (repo *PostgresRepository) GetSettings(ctx context.Context, libraryID uuid.UUID) (RetrievalSettings, error) {
	var settings RetrievalSettings
	err := repo.pool.QueryRow(ctx, `SELECT library_id,chunk_size,chunk_overlap,top_k,similarity_threshold,updated_by
        FROM library_retrieval_settings WHERE library_id=$1`, libraryID).
		Scan(&settings.LibraryID, &settings.ChunkSize, &settings.ChunkOverlap, &settings.TopK, &settings.SimilarityThreshold, &settings.UpdatedBy)
	if err == pgx.ErrNoRows {
		return RetrievalSettings{}, ErrNotFound
	}
	return settings, err
}

func (repo *PostgresRepository) UpdateSettings(ctx context.Context, settings RetrievalSettings) error {
	result, err := repo.pool.Exec(ctx, `UPDATE library_retrieval_settings SET chunk_size=$2,chunk_overlap=$3,
        top_k=$4,similarity_threshold=$5,updated_by=$6,updated_at=now() WHERE library_id=$1`,
		settings.LibraryID, settings.ChunkSize, settings.ChunkOverlap, settings.TopK,
		settings.SimilarityThreshold, settings.UpdatedBy)
	return affectedOrNotFound(result.RowsAffected(), err)
}

type rowScanner interface {
	Scan(...any) error
}

func scanLibrary(row rowScanner) (Library, error) {
	var library Library
	err := row.Scan(&library.ID, &library.OwnerUserID, &library.OwnerNickname, &library.Name,
		&library.Category, &library.Description, &library.Visibility, &library.Status,
		&library.DocumentCount, &library.ChunkCount, &library.StorageBytes, &library.IndexedTokens,
		&library.CreatedAt, &library.UpdatedAt)
	return library, err
}

func affectedOrNotFound(affected int64, err error) error {
	if err != nil {
		return err
	}
	if affected == 0 {
		return ErrNotFound
	}
	return nil
}
