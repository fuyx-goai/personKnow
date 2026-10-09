package repo

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	. "knowledge-base/internal/document/entity"
)

type PostgresRepository struct {
	pool *pgxpool.Pool
}

func NewPostgresRepository(pool *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{pool: pool}
}

func (repo *PostgresRepository) Create(ctx context.Context, document Document) error {
	if document.Tags == nil {
		document.Tags = []string{}
	}
	tags, err := json.Marshal(document.Tags)
	if err != nil {
		return err
	}
	_, err = repo.pool.Exec(ctx, `INSERT INTO documents
        (id,library_id,uploaded_by,original_name,display_name,extension,mime_type,original_bytes,
         original_sha256,original_path,status,summary,tags,created_at,updated_at)
        VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15)`,
		document.ID, document.LibraryID, document.UploadedBy, document.OriginalName, document.DisplayName,
		string(document.Format), document.MIMEType, document.OriginalBytes, document.OriginalSHA256,
		document.OriginalPath, document.Status, document.Summary, string(tags), document.CreatedAt, document.UpdatedAt)
	return err
}

func (repo *PostgresRepository) List(ctx context.Context, actorID uuid.UUID, filter ListFilter) ([]Document, error) {
	limit := filter.Limit
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	rows, err := repo.pool.Query(ctx, `SELECT `+documentColumns+`
        FROM documents d JOIN knowledge_bases k ON k.id=d.library_id
        WHERE d.deleted_at IS NULL AND k.deleted_at IS NULL AND k.status='active'
          AND (k.owner_user_id=$1 OR k.visibility='public')
          AND ($2::uuid IS NULL OR d.library_id=$2)
          AND ($3='' OR d.display_name ILIKE '%'||$3||'%' OR d.summary ILIKE '%'||$3||'%' OR d.tags::text ILIKE '%'||$3||'%')
          AND ($4='' OR d.status=$4)
        ORDER BY d.updated_at DESC,d.id LIMIT $5`, actorID, filter.LibraryID, strings.TrimSpace(filter.Keyword), filter.Status, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var documents []Document
	for rows.Next() {
		document, err := scanDocument(rows)
		if err != nil {
			return nil, err
		}
		documents = append(documents, document)
	}
	return documents, rows.Err()
}

func (repo *PostgresRepository) Get(ctx context.Context, actorID, documentID uuid.UUID) (Document, error) {
	row := repo.pool.QueryRow(ctx, `SELECT `+documentColumns+`
        FROM documents d JOIN knowledge_bases k ON k.id=d.library_id
        WHERE d.id=$1 AND d.deleted_at IS NULL AND k.deleted_at IS NULL
          AND (k.owner_user_id=$2 OR (k.visibility='public' AND k.status='active'))`, documentID, actorID)
	document, err := scanDocument(row)
	if err == pgx.ErrNoRows {
		return Document{}, ErrDocumentNotFound
	}
	return document, err
}

func (repo *PostgresRepository) UpdateMetadata(ctx context.Context, document Document) error {
	if document.Tags == nil {
		document.Tags = []string{}
	}
	tags, err := json.Marshal(document.Tags)
	if err != nil {
		return err
	}
	result, err := repo.pool.Exec(ctx, `UPDATE documents SET display_name=$2,tags=$3,updated_at=$4
		WHERE id=$1 AND deleted_at IS NULL`, document.ID, document.DisplayName, string(tags), document.UpdatedAt)
	return documentAffected(result.RowsAffected(), err)
}

func (repo *PostgresRepository) NextContentVersion(ctx context.Context, documentID uuid.UUID) (int, error) {
	var version int
	err := repo.pool.QueryRow(ctx, `SELECT COALESCE(MAX(version),0)+1 FROM document_contents WHERE document_id=$1`, documentID).Scan(&version)
	return version, err
}

func (repo *PostgresRepository) CreateContentVersion(ctx context.Context, content ContentVersion) error {
	if content.StructureMap == nil {
		content.StructureMap = map[string]any{}
	}
	structure, err := json.Marshal(content.StructureMap)
	if err != nil {
		return err
	}
	_, err = repo.pool.Exec(ctx, `INSERT INTO document_contents
        (id,document_id,version,source_type,content_path,content_hash,text_bytes,token_estimate,structure_map,created_by,created_at)
        VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)`, content.ID, content.DocumentID, content.Version,
		content.SourceType, content.ContentPath, content.ContentHash, content.TextBytes, content.TokenEstimate,
		string(structure), content.CreatedBy, content.CreatedAt)
	return err
}

func (repo *PostgresRepository) GetContent(ctx context.Context, actorID, documentID uuid.UUID) (ContentVersion, error) {
	row := repo.pool.QueryRow(ctx, `SELECT dc.id,dc.document_id,dc.version,dc.source_type,dc.content_path,
        dc.content_hash,dc.text_bytes,dc.token_estimate,dc.structure_map,dc.created_by,dc.created_at
        FROM documents d JOIN knowledge_bases k ON k.id=d.library_id
        JOIN document_contents dc ON dc.id=COALESCE(d.active_content_version_id,
            (SELECT id FROM document_contents WHERE document_id=d.id ORDER BY version DESC LIMIT 1))
        WHERE d.id=$1 AND d.deleted_at IS NULL AND (k.owner_user_id=$2 OR (k.visibility='public' AND k.status='active'))`,
		documentID, actorID)
	content, err := scanContent(row)
	if err == pgx.ErrNoRows {
		return ContentVersion{}, ErrDocumentNotFound
	}
	return content, err
}

func (repo *PostgresRepository) MarkDeleting(ctx context.Context, documentID uuid.UUID) error {
	result, err := repo.pool.Exec(ctx, `UPDATE documents SET status='deleting',updated_at=now()
        WHERE id=$1 AND deleted_at IS NULL AND status<>'deleting'`, documentID)
	return documentAffected(result.RowsAffected(), err)
}

const documentColumns = `d.id,d.library_id,d.uploaded_by,d.original_name,d.display_name,d.extension,d.mime_type,
    d.original_bytes,d.original_sha256,d.original_path,d.status,d.summary,d.tags,d.active_content_version_id,
    d.chunk_count,d.indexed_tokens,d.last_indexed_at,d.created_at,d.updated_at`

type rowScanner interface {
	Scan(...any) error
}

func scanDocument(row rowScanner) (Document, error) {
	var document Document
	var tags []byte
	err := row.Scan(&document.ID, &document.LibraryID, &document.UploadedBy, &document.OriginalName,
		&document.DisplayName, &document.Format, &document.MIMEType, &document.OriginalBytes,
		&document.OriginalSHA256, &document.OriginalPath, &document.Status, &document.Summary, &tags,
		&document.ActiveContentVersionID, &document.ChunkCount, &document.IndexedTokens, &document.LastIndexedAt,
		&document.CreatedAt, &document.UpdatedAt)
	if err == nil {
		err = json.Unmarshal(tags, &document.Tags)
	}
	return document, err
}

func scanContent(row rowScanner) (ContentVersion, error) {
	var content ContentVersion
	var structure []byte
	err := row.Scan(&content.ID, &content.DocumentID, &content.Version, &content.SourceType, &content.ContentPath,
		&content.ContentHash, &content.TextBytes, &content.TokenEstimate, &structure, &content.CreatedBy, &content.CreatedAt)
	if err == nil {
		err = json.Unmarshal(structure, &content.StructureMap)
	}
	return content, err
}

func documentAffected(affected int64, err error) error {
	if err != nil {
		return err
	}
	if affected == 0 {
		return ErrDocumentNotFound
	}
	return nil
}
