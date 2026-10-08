package indexing

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	documentdomain "knowledge-base/internal/document"
	platformpostgres "knowledge-base/internal/platform/postgres"
)

type PostgresRepository struct {
	pool *pgxpool.Pool
}

func NewPostgresRepository(pool *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{pool: pool}
}

func (repository *PostgresRepository) ScheduleDocument(ctx context.Context, request documentdomain.JobRequest) (uuid.UUID, error) {
	jobID := uuid.New()
	err := repository.pool.QueryRow(ctx, `INSERT INTO index_jobs
        (id,document_id,content_version_id,job_type,status,progress,stage,next_run_at)
        VALUES($1,$2,$3,$4,'queued',0,'queued',now()) ON CONFLICT DO NOTHING RETURNING id`,
		jobID, request.DocumentID, request.ContentVersionID, JobType(request.Type)).Scan(&jobID)
	if err == nil {
		return jobID, nil
	}
	if err != pgx.ErrNoRows {
		return uuid.Nil, err
	}
	err = repository.pool.QueryRow(ctx, `SELECT id FROM index_jobs
        WHERE document_id=$1 AND status IN ('queued','running') ORDER BY created_at DESC LIMIT 1`, request.DocumentID).Scan(&jobID)
	return jobID, err
}

func (repository *PostgresRepository) ScheduleLibrary(ctx context.Context, actorID, libraryID uuid.UUID) ([]uuid.UUID, error) {
	rows, err := repository.pool.Query(ctx, `SELECT d.id,d.active_content_version_id FROM documents d
        JOIN knowledge_bases k ON k.id=d.library_id
        WHERE d.library_id=$1 AND k.owner_user_id=$2 AND d.deleted_at IS NULL AND d.status<>'deleting'`, libraryID, actorID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	type target struct {
		documentID uuid.UUID
		versionID  *uuid.UUID
	}
	var targets []target
	for rows.Next() {
		var item target
		if err := rows.Scan(&item.documentID, &item.versionID); err != nil {
			return nil, err
		}
		targets = append(targets, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	jobIDs := make([]uuid.UUID, 0, len(targets))
	for _, item := range targets {
		jobID, err := repository.ScheduleDocument(ctx, documentdomain.JobRequest{
			ActorID: actorID, LibraryID: libraryID, DocumentID: item.documentID,
			ContentVersionID: item.versionID, Type: documentdomain.JobReindex,
		})
		if err != nil {
			return nil, err
		}
		jobIDs = append(jobIDs, jobID)
	}
	return jobIDs, nil
}

func (repository *PostgresRepository) Claim(ctx context.Context, workerID string, now time.Time, lease time.Duration) (*Job, error) {
	row := repository.pool.QueryRow(ctx, `WITH candidate AS (
        SELECT id FROM index_jobs WHERE status='queued' AND next_run_at<=$2
        ORDER BY next_run_at,created_at FOR UPDATE SKIP LOCKED LIMIT 1)
        UPDATE index_jobs j SET status='running',stage='starting',attempt_count=j.attempt_count+1,
        worker_id=$1,lease_expires_at=$3,started_at=COALESCE(j.started_at,$2),updated_at=$2
        FROM candidate WHERE j.id=candidate.id RETURNING `+jobColumns, workerID, now, now.Add(lease))
	job, err := scanJob(row)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	return &job, err
}

func (repository *PostgresRepository) Progress(ctx context.Context, jobID uuid.UUID, stage string, progress int, leaseExpiresAt time.Time) error {
	_, err := repository.pool.Exec(ctx, `UPDATE index_jobs SET stage=$2,progress=$3,lease_expires_at=$4,updated_at=now()
        WHERE id=$1 AND status='running'`, jobID, stage, progress, leaseExpiresAt)
	return err
}

func (repository *PostgresRepository) Complete(ctx context.Context, job Job, result ProcessResult, now time.Time) (*uuid.UUID, error) {
	var oldVersion *uuid.UUID
	err := platformpostgres.WithinTx(ctx, repository.pool, func(tx pgx.Tx) error {
		var libraryID uuid.UUID
		if err := tx.QueryRow(ctx, `SELECT active_content_version_id,library_id FROM documents WHERE id=$1 FOR UPDATE`, job.DocumentID).
			Scan(&oldVersion, &libraryID); err != nil {
			return err
		}
		if job.Type == JobDelete {
			if _, err := tx.Exec(ctx, `UPDATE documents SET deleted_at=$2,status='deleting',updated_at=$2 WHERE id=$1`, job.DocumentID, now); err != nil {
				return err
			}
		} else {
			if _, err := tx.Exec(ctx, `UPDATE documents SET active_content_version_id=$2,status='ready',chunk_count=$3,
                indexed_tokens=$4,last_indexed_at=$5,updated_at=$5 WHERE id=$1`, job.DocumentID, result.ContentVersionID,
				result.ChunkCount, result.IndexedTokens, now); err != nil {
				return err
			}
			if _, err := tx.Exec(ctx, `UPDATE document_contents SET token_estimate=$2 WHERE id=$1`, result.ContentVersionID, result.IndexedTokens); err != nil {
				return err
			}
		}
		if err := refreshLibraryStats(ctx, tx, libraryID); err != nil {
			return err
		}
		action := "document.index"
		if job.Type == JobDelete {
			action = "document.delete.complete"
		}
		if _, err := tx.Exec(ctx, `INSERT INTO audit_logs
            (actor_user_id,action,resource_type,resource_id,result,client_type,metadata,occurred_at)
            SELECT k.owner_user_id,$2,'document',$1,'success','worker','{"status":"ready"}'::jsonb,$3
            FROM documents d JOIN knowledge_bases k ON k.id=d.library_id WHERE d.id=$1`, job.DocumentID, action, now); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `UPDATE index_jobs SET status='ready',stage='complete',progress=100,
            content_version_id=COALESCE(content_version_id,$2),lease_expires_at=NULL,worker_id=NULL,
            error_code=NULL,error_message=NULL,finished_at=$3,updated_at=$3 WHERE id=$1`, job.ID, result.ContentVersionID, now)
		return err
	})
	return oldVersion, err
}

func (repository *PostgresRepository) Fail(ctx context.Context, job Job, failure JobFailure, now time.Time) error {
	if failure.NextRunAt != nil {
		_, err := repository.pool.Exec(ctx, `UPDATE index_jobs SET status='queued',stage='retry_wait',progress=0,
            next_run_at=$2,lease_expires_at=NULL,worker_id=NULL,error_code=$3,error_message=$4,updated_at=$5 WHERE id=$1`,
			job.ID, failure.NextRunAt, failure.Code, failure.Message, now)
		return err
	}
	_, err := repository.pool.Exec(ctx, `UPDATE index_jobs SET status='failed',stage='failed',lease_expires_at=NULL,
        worker_id=NULL,error_code=$2,error_message=$3,finished_at=$4,updated_at=$4 WHERE id=$1`, job.ID, failure.Code, failure.Message, now)
	return err
}

func (repository *PostgresRepository) RecoverExpired(ctx context.Context, now time.Time) (int, error) {
	result, err := repository.pool.Exec(ctx, `UPDATE index_jobs SET status='queued',stage='recovered',worker_id=NULL,
        lease_expires_at=NULL,next_run_at=$1,updated_at=$1 WHERE status='running' AND lease_expires_at<$1`, now)
	return int(result.RowsAffected()), err
}

func (repository *PostgresRepository) Get(ctx context.Context, actorID, jobID uuid.UUID) (Job, error) {
	row := repository.pool.QueryRow(ctx, `SELECT `+jobColumns+` FROM index_jobs j
        JOIN documents d ON d.id=j.document_id JOIN knowledge_bases k ON k.id=d.library_id
        WHERE j.id=$1 AND k.owner_user_id=$2`, jobID, actorID)
	job, err := scanJob(row)
	if err == pgx.ErrNoRows {
		return Job{}, ErrJobNotFound
	}
	return job, err
}

func (repository *PostgresRepository) Retry(ctx context.Context, actorID, jobID uuid.UUID) error {
	result, err := repository.pool.Exec(ctx, `UPDATE index_jobs j SET status='queued',stage='queued',progress=0,
        next_run_at=now(),finished_at=NULL,error_code=NULL,error_message=NULL,updated_at=now()
        FROM documents d JOIN knowledge_bases k ON k.id=d.library_id
        WHERE j.id=$1 AND j.document_id=d.id AND k.owner_user_id=$2 AND j.status='failed'`, jobID, actorID)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return ErrJobNotRetryable
	}
	return nil
}

const jobColumns = `j.id,j.document_id,j.content_version_id,j.job_type,j.status,j.progress,j.stage,
    j.attempt_count,j.max_attempts,j.next_run_at,j.lease_expires_at,COALESCE(j.worker_id,''),
    COALESCE(j.error_code,''),COALESCE(j.error_message,''),j.created_at,j.updated_at`

type jobScanner interface{ Scan(...any) error }

func scanJob(row jobScanner) (Job, error) {
	var job Job
	err := row.Scan(&job.ID, &job.DocumentID, &job.ContentVersionID, &job.Type, &job.Status, &job.Progress,
		&job.Stage, &job.AttemptCount, &job.MaxAttempts, &job.NextRunAt, &job.LeaseExpiresAt,
		&job.WorkerID, &job.ErrorCode, &job.ErrorMessage, &job.CreatedAt, &job.UpdatedAt)
	return job, err
}

func refreshLibraryStats(ctx context.Context, tx pgx.Tx, libraryID uuid.UUID) error {
	_, err := tx.Exec(ctx, `UPDATE knowledge_bases k SET
        document_count=(SELECT count(*) FROM documents d WHERE d.library_id=k.id AND d.deleted_at IS NULL),
        chunk_count=COALESCE((SELECT sum(d.chunk_count) FROM documents d WHERE d.library_id=k.id AND d.deleted_at IS NULL),0),
        indexed_tokens=COALESCE((SELECT sum(d.indexed_tokens) FROM documents d WHERE d.library_id=k.id AND d.deleted_at IS NULL),0),
        updated_at=now() WHERE k.id=$1`, libraryID)
	return err
}
