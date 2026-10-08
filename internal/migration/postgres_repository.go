package migration

import (
	"context"
	"encoding/json"
	"fmt"

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

func (repository *PostgresRepository) EnsureDefaultLibrary(ctx context.Context, userID uuid.UUID) (uuid.UUID, error) {
	var libraryID uuid.UUID
	err := platformpostgres.WithinTx(ctx, repository.pool, func(tx pgx.Tx) error {
		if err := ensureMigrationUser(ctx, tx, userID); err != nil {
			return err
		}
		var err error
		libraryID, err = findDefaultLibrary(ctx, tx, userID)
		if err != nil && err != pgx.ErrNoRows {
			return err
		}
		if err == pgx.ErrNoRows {
			libraryID = uuid.NewSHA1(legacyNamespace, []byte(userID.String()+"\x00default-library"))
			if err := insertDefaultLibrary(ctx, tx, userID, libraryID); err != nil {
				return err
			}
		}
		return ensureDefaultSettings(ctx, tx, userID, libraryID)
	})
	return libraryID, err
}

func (repository *PostgresRepository) PrepareDocument(ctx context.Context, item ImportDocument) (DocumentState, error) {
	state := DocumentState{}
	err := platformpostgres.WithinTx(ctx, repository.pool, func(tx pgx.Tx) error {
		ready, exists, err := migrationDocumentState(ctx, tx, item.DocumentID, item.TargetUser)
		if err != nil || exists {
			state.Ready = ready
			return err
		}
		created, err := insertMigrationDocument(ctx, tx, item)
		if err != nil {
			return err
		}
		if !created {
			state.Ready, _, err = migrationDocumentState(ctx, tx, item.DocumentID, item.TargetUser)
			return err
		}
		if err := insertMigrationContent(ctx, tx, item); err != nil {
			return err
		}
		state.Created = true
		return nil
	})
	return state, err
}

func (repository *PostgresRepository) CompleteDocument(ctx context.Context, documentID uuid.UUID, chunkCount int) (bool, error) {
	completed := false
	err := platformpostgres.WithinTx(ctx, repository.pool, func(tx pgx.Tx) error {
		var libraryID, userID uuid.UUID
		var originalBytes int64
		err := tx.QueryRow(ctx, `UPDATE documents SET status='ready',chunk_count=$2,last_indexed_at=now(),updated_at=now()
            WHERE id=$1 AND status<>'ready' AND deleted_at IS NULL
            RETURNING library_id,uploaded_by,original_bytes`, documentID, chunkCount).
			Scan(&libraryID, &userID, &originalBytes)
		if err == pgx.ErrNoRows {
			return ensureCompletedDocumentExists(ctx, tx, documentID)
		}
		if err != nil {
			return err
		}
		completed = true
		if err := refreshLibraryStats(ctx, tx, libraryID); err != nil {
			return err
		}
		return recordMigratedStorage(ctx, tx, userID, documentID, originalBytes)
	})
	return completed, err
}

func (repository *PostgresRepository) RecordAudit(ctx context.Context, record AuditRecord) error {
	metadata, err := json.Marshal(map[string]any{
		"documents_added": record.Documents, "chunks_added": record.Chunks,
		"failure_count": record.FailureCount,
	})
	if err != nil {
		return err
	}
	result := "success"
	if record.FailureCount > 0 {
		result = "failure"
	}
	_, err = repository.pool.Exec(ctx, `INSERT INTO audit_logs
        (actor_user_id,action,resource_type,resource_id,result,client_type,metadata,occurred_at)
        VALUES($1,'migration.legacy','knowledge_base',$2,$3,'worker',$4,$5)`,
		record.TargetUser, record.LibraryID, result, metadata, record.OccurredAt)
	return err
}

func ensureMigrationUser(ctx context.Context, tx pgx.Tx, userID uuid.UUID) error {
	var exists bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM users WHERE id=$1 AND status='active' AND deleted_at IS NULL)`, userID).Scan(&exists); err != nil {
		return err
	}
	if !exists {
		return fmt.Errorf("目标用户不存在或已停用")
	}
	return nil
}

func findDefaultLibrary(ctx context.Context, tx pgx.Tx, userID uuid.UUID) (uuid.UUID, error) {
	var libraryID uuid.UUID
	err := tx.QueryRow(ctx, `SELECT id FROM knowledge_bases
        WHERE owner_user_id=$1 AND name='默认知识库' AND deleted_at IS NULL
        ORDER BY created_at LIMIT 1`, userID).Scan(&libraryID)
	return libraryID, err
}

func insertDefaultLibrary(ctx context.Context, tx pgx.Tx, userID, libraryID uuid.UUID) error {
	_, err := tx.Exec(ctx, `INSERT INTO knowledge_bases
        (id,owner_user_id,name,category,description,visibility,status)
        VALUES($1,$2,'默认知识库','uncategorized','从旧 knowledge.json 迁移','private','active')
        ON CONFLICT(id) DO UPDATE SET status='active',deleted_at=NULL,updated_at=now()`, libraryID, userID)
	return err
}

func ensureDefaultSettings(ctx context.Context, tx pgx.Tx, userID, libraryID uuid.UUID) error {
	_, err := tx.Exec(ctx, `INSERT INTO library_retrieval_settings
        (library_id,chunk_size,chunk_overlap,top_k,similarity_threshold,updated_by)
        VALUES($1,800,100,5,0.3,$2) ON CONFLICT(library_id) DO NOTHING`, libraryID, userID)
	return err
}

func migrationDocumentState(ctx context.Context, tx pgx.Tx, documentID, userID uuid.UUID) (bool, bool, error) {
	var status string
	err := tx.QueryRow(ctx, `SELECT d.status FROM documents d JOIN knowledge_bases k ON k.id=d.library_id
        WHERE d.id=$1 AND k.owner_user_id=$2 AND d.deleted_at IS NULL`, documentID, userID).Scan(&status)
	if err == pgx.ErrNoRows {
		return false, false, nil
	}
	return status == "ready", err == nil, err
}

func insertMigrationDocument(ctx context.Context, tx pgx.Tx, item ImportDocument) (bool, error) {
	result, err := tx.Exec(ctx, `INSERT INTO documents
        (id,library_id,uploaded_by,original_name,display_name,extension,mime_type,original_bytes,
         original_sha256,original_path,status,summary,tags,created_at,updated_at)
        VALUES($1,$2,$3,$4,$5,$6,'text/plain',$7,$8,$9,'processing','旧知识库迁移','[]'::jsonb,$10,$10)
        ON CONFLICT(id) DO NOTHING`, item.DocumentID, item.LibraryID, item.TargetUser,
		item.DisplayName, item.DisplayName, item.Extension, item.OriginalBytes, item.OriginalHash,
		"legacy://sha256/"+item.OriginalHash, item.CreatedAt)
	return result.RowsAffected() == 1, err
}

func insertMigrationContent(ctx context.Context, tx pgx.Tx, item ImportDocument) error {
	_, err := tx.Exec(ctx, `INSERT INTO document_contents
        (id,document_id,version,source_type,content_path,content_hash,text_bytes,token_estimate,structure_map,created_by,created_at)
        VALUES($1,$2,1,'parsed',$3,$4,$5,0,$6,$7,$8)
        ON CONFLICT(id) DO NOTHING`, item.ContentVersionID, item.DocumentID, item.ContentPath,
		item.ContentHash, item.OriginalBytes, map[string]any{"legacy_source": item.SourceName}, item.TargetUser, item.CreatedAt)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `UPDATE documents SET active_content_version_id=$2 WHERE id=$1`, item.DocumentID, item.ContentVersionID)
	return err
}

func ensureCompletedDocumentExists(ctx context.Context, tx pgx.Tx, documentID uuid.UUID) error {
	var ready bool
	err := tx.QueryRow(ctx, `SELECT status='ready' FROM documents WHERE id=$1 AND deleted_at IS NULL`, documentID).Scan(&ready)
	if err != nil {
		return err
	}
	if !ready {
		return fmt.Errorf("文档未能完成迁移")
	}
	return nil
}

func refreshLibraryStats(ctx context.Context, tx pgx.Tx, libraryID uuid.UUID) error {
	_, err := tx.Exec(ctx, `UPDATE knowledge_bases SET
        document_count=(SELECT count(*) FROM documents WHERE library_id=$1 AND deleted_at IS NULL),
        chunk_count=(SELECT COALESCE(sum(chunk_count),0) FROM documents WHERE library_id=$1 AND deleted_at IS NULL),
        storage_bytes=(SELECT COALESCE(sum(original_bytes),0) FROM documents WHERE library_id=$1 AND deleted_at IS NULL),
        indexed_tokens=(SELECT COALESCE(sum(indexed_tokens),0) FROM documents WHERE library_id=$1 AND deleted_at IS NULL),
        updated_at=now() WHERE id=$1`, libraryID)
	return err
}

func recordMigratedStorage(ctx context.Context, tx pgx.Tx, userID, documentID uuid.UUID, originalBytes int64) error {
	_, err := tx.Exec(ctx, `INSERT INTO monthly_usage(user_id,month_start,storage_bytes)
        VALUES($1,date_trunc('month',now())::date,(SELECT COALESCE(sum(d.original_bytes),0)
        FROM documents d JOIN knowledge_bases k ON k.id=d.library_id
        WHERE k.owner_user_id=$1 AND d.deleted_at IS NULL))
        ON CONFLICT(user_id,month_start) DO UPDATE SET storage_bytes=excluded.storage_bytes,updated_at=now()`, userID)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `INSERT INTO usage_records
        (id,user_id,usage_type,quantity,unit,resource_type,resource_id,metadata)
        VALUES($1,$2,'storage',$3,'bytes','document',$4,'{"source":"legacy_migration"}'::jsonb)`,
		uuid.New(), userID, originalBytes, documentID)
	return err
}
