package indexing

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	documentdomain "knowledge-base/internal/document"
)

type PostgresSourceLoader struct {
	pool    *pgxpool.Pool
	files   *documentdomain.LocalFileStore
	parsers *documentdomain.ParserRegistry
	quota   documentdomain.StorageQuota
	now     func() time.Time
}

func NewPostgresSourceLoader(pool *pgxpool.Pool, files *documentdomain.LocalFileStore, parsers *documentdomain.ParserRegistry, quota documentdomain.StorageQuota) *PostgresSourceLoader {
	return &PostgresSourceLoader{pool: pool, files: files, parsers: parsers, quota: quota, now: time.Now}
}

func (loader *PostgresSourceLoader) Load(ctx context.Context, job Job) (IndexSource, error) {
	var source IndexSource
	var format documentdomain.Format
	var originalPath string
	var uploadedBy uuid.UUID
	var activeVersion *uuid.UUID
	err := loader.pool.QueryRow(ctx, `SELECT k.owner_user_id,d.library_id,d.id,d.display_name,d.extension,d.original_path,
        d.uploaded_by,d.active_content_version_id,k.visibility,s.chunk_size,s.chunk_overlap,s.top_k,s.similarity_threshold
        FROM documents d JOIN knowledge_bases k ON k.id=d.library_id
        JOIN library_retrieval_settings s ON s.library_id=k.id WHERE d.id=$1`, job.DocumentID).Scan(
		&source.OwnerUserID, &source.LibraryID, &source.DocumentID, &source.SourceName, &format, &originalPath,
		&uploadedBy, &activeVersion, &source.Visibility, &source.Settings.ChunkSize, &source.Settings.ChunkOverlap,
		&source.Settings.TopK, &source.Settings.SimilarityThreshold)
	if err != nil {
		return IndexSource{}, err
	}
	source.Settings.LibraryID = source.LibraryID
	versionID := job.ContentVersionID
	if versionID == nil {
		versionID = activeVersion
	}
	if job.Type == JobDelete {
		if versionID != nil {
			source.ContentVersionID = *versionID
		}
		return source, nil
	}
	if versionID != nil {
		return loader.loadContent(ctx, source, *versionID)
	}
	return loader.parseOriginal(ctx, source, uploadedBy, originalPath, format)
}

func (loader *PostgresSourceLoader) loadContent(ctx context.Context, source IndexSource, versionID uuid.UUID) (IndexSource, error) {
	var contentPath string
	if err := loader.pool.QueryRow(ctx, `SELECT content_path FROM document_contents WHERE id=$1 AND document_id=$2`, versionID, source.DocumentID).Scan(&contentPath); err != nil {
		return IndexSource{}, err
	}
	data, err := loader.files.Read(contentPath)
	if err != nil {
		return IndexSource{}, err
	}
	source.ContentVersionID = versionID
	source.Content = string(data)
	return source, nil
}

func (loader *PostgresSourceLoader) parseOriginal(ctx context.Context, source IndexSource, createdBy uuid.UUID, originalPath string, format documentdomain.Format) (IndexSource, error) {
	path := loader.files.Absolute(originalPath)
	if path == "" {
		return IndexSource{}, fmt.Errorf("original path is invalid")
	}
	parsed, err := loader.parsers.Parse(ctx, path, format)
	if err != nil {
		return IndexSource{}, err
	}
	var version int
	if err := loader.pool.QueryRow(ctx, `SELECT COALESCE(max(version),0)+1 FROM document_contents WHERE document_id=$1`, source.DocumentID).Scan(&version); err != nil {
		return IndexSource{}, err
	}
	stored, err := loader.files.WriteContent(documentdomain.Location{
		UserID: source.OwnerUserID, LibraryID: source.LibraryID, DocumentID: source.DocumentID,
	}, version, parsed.Text)
	if err != nil {
		return IndexSource{}, err
	}
	if loader.quota != nil {
		if err := loader.quota.ApplyStorage(ctx, source.OwnerUserID, source.DocumentID, stored.Size); err != nil {
			return IndexSource{}, err
		}
	}
	versionID := uuid.New()
	structure, err := json.Marshal(parsed.StructureMap)
	if err != nil {
		return IndexSource{}, err
	}
	_, err = loader.pool.Exec(ctx, `INSERT INTO document_contents
        (id,document_id,version,source_type,content_path,content_hash,text_bytes,token_estimate,structure_map,created_by,created_at)
        VALUES($1,$2,$3,'parsed',$4,$5,$6,0,$7,$8,$9)`, versionID, source.DocumentID, version,
		stored.RelativePath, stored.SHA256, stored.Size, structure, createdBy, loader.now())
	if err != nil {
		if loader.quota != nil {
			_ = loader.quota.ApplyStorage(ctx, source.OwnerUserID, source.DocumentID, -stored.Size)
		}
		return IndexSource{}, err
	}
	source.ContentVersionID = versionID
	source.Content = parsed.Text
	return source, nil
}
