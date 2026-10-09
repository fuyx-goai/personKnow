package repo

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	platformpostgres "knowledge-base/internal/platform/postgres"
	. "knowledge-base/internal/usage/entity"
)

type PostgresRepository struct {
	pool *pgxpool.Pool
}

func NewPostgresRepository(pool *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{pool: pool}
}

func (repository *PostgresRepository) UpdateMonthly(ctx context.Context, userID uuid.UUID, month time.Time, mutate MonthlyMutation) error {
	return platformpostgres.WithinTx(ctx, repository.pool, func(tx pgx.Tx) error {
		limits, err := quotaLimits(ctx, tx, userID)
		if err != nil {
			return err
		}
		if err := ensureMonthlyRow(ctx, tx, userID, month); err != nil {
			return err
		}
		current, err := lockMonthlyRow(ctx, tx, userID, month)
		if err != nil {
			return err
		}
		next, records, err := mutate(current, limits)
		if err != nil {
			return err
		}
		if err := validateMonthly(next); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE monthly_usage SET storage_bytes=$3,embedding_tokens=$4,
            input_tokens=$5,output_tokens=$6,total_tokens=$7,updated_at=now()
            WHERE user_id=$1 AND month_start=$2`, userID, month, next.StorageBytes,
			next.EmbeddingTokens, next.InputTokens, next.OutputTokens, next.TotalTokens); err != nil {
			return err
		}
		return insertUsageRecords(ctx, tx, records)
	})
}

func (repository *PostgresRepository) Summary(ctx context.Context, userID uuid.UUID, month time.Time) (MonthlyUsage, Limits, error) {
	var current MonthlyUsage
	var limits Limits
	err := repository.pool.QueryRow(ctx, `SELECT p.storage_quota_bytes,p.monthly_token_quota,
        COALESCE(m.storage_bytes,(SELECT previous.storage_bytes FROM monthly_usage previous
            WHERE previous.user_id=$1 AND previous.month_start<$2 ORDER BY previous.month_start DESC LIMIT 1),0),
        COALESCE(m.embedding_tokens,0),COALESCE(m.input_tokens,0),COALESCE(m.output_tokens,0),COALESCE(m.total_tokens,0)
        FROM user_plans up JOIN plans p ON p.id=up.plan_id
        LEFT JOIN monthly_usage m ON m.user_id=up.user_id AND m.month_start=$2
        WHERE up.user_id=$1 AND up.starts_at<=now() AND (up.ends_at IS NULL OR up.ends_at>now())
        ORDER BY up.starts_at DESC LIMIT 1`, userID, month).Scan(
		&limits.StorageBytes, &limits.MonthlyTokens, &current.StorageBytes,
		&current.EmbeddingTokens, &current.InputTokens, &current.OutputTokens, &current.TotalTokens)
	return current, limits, err
}

func (repository *PostgresRepository) ListRecords(ctx context.Context, userID uuid.UUID, filter RecordFilter) ([]Record, error) {
	rows, err := repository.pool.Query(ctx, `SELECT id,user_id,usage_type,quantity,unit,resource_type,resource_id,occurred_at,metadata
        FROM usage_records WHERE user_id=$1 AND ($2='' OR usage_type=$2)
        AND ($3::timestamptz IS NULL OR occurred_at<$3)
        ORDER BY occurred_at DESC,id DESC LIMIT $4`, userID, filter.Type, filter.Before, filter.Limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	records := make([]Record, 0, filter.Limit)
	for rows.Next() {
		var record Record
		if err := rows.Scan(&record.ID, &record.UserID, &record.Type, &record.Quantity, &record.Unit,
			&record.ResourceType, &record.ResourceID, &record.OccurredAt, &record.Metadata); err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	return records, rows.Err()
}

func (repository *PostgresRepository) AppendAudit(ctx context.Context, entry AuditEntry) error {
	metadata, err := json.Marshal(entry.Metadata)
	if err != nil {
		return err
	}
	_, err = repository.pool.Exec(ctx, `INSERT INTO audit_logs
        (actor_user_id,action,resource_type,resource_id,result,request_id,client_type,ip_hash,metadata,occurred_at)
        VALUES($1,$2,$3,$4,$5,NULLIF($6,''),NULLIF($7,''),NULLIF($8,''),$9,$10)`,
		entry.ActorUserID, entry.Action, entry.ResourceType, entry.ResourceID, entry.Result,
		entry.RequestID, entry.ClientType, entry.IPHash, metadata, entry.OccurredAt)
	return err
}

func (repository *PostgresRepository) ListAudit(ctx context.Context, actorID uuid.UUID, filter AuditFilter) ([]AuditEntry, error) {
	rows, err := repository.pool.Query(ctx, `SELECT id,actor_user_id,action,resource_type,resource_id,result,
        COALESCE(request_id,''),COALESCE(client_type,''),metadata,occurred_at
        FROM audit_logs WHERE actor_user_id=$1 AND ($2='' OR action=$2) AND ($3='' OR resource_type=$3)
        AND ($4::timestamptz IS NULL OR occurred_at<$4)
        ORDER BY occurred_at DESC,id DESC LIMIT $5`, actorID, filter.Action, filter.ResourceType, filter.Before, filter.Limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	entries := make([]AuditEntry, 0, filter.Limit)
	for rows.Next() {
		var entry AuditEntry
		if err := rows.Scan(&entry.ID, &entry.ActorUserID, &entry.Action, &entry.ResourceType, &entry.ResourceID,
			&entry.Result, &entry.RequestID, &entry.ClientType, &entry.Metadata, &entry.OccurredAt); err != nil {
			return nil, err
		}
		entries = append(entries, entry)
	}
	return entries, rows.Err()
}

func quotaLimits(ctx context.Context, tx pgx.Tx, userID uuid.UUID) (Limits, error) {
	var limits Limits
	err := tx.QueryRow(ctx, `SELECT p.storage_quota_bytes,p.monthly_token_quota
        FROM user_plans up JOIN plans p ON p.id=up.plan_id
        WHERE up.user_id=$1 AND up.starts_at<=now() AND (up.ends_at IS NULL OR up.ends_at>now())
        ORDER BY up.starts_at DESC LIMIT 1`, userID).Scan(&limits.StorageBytes, &limits.MonthlyTokens)
	return limits, err
}

func ensureMonthlyRow(ctx context.Context, tx pgx.Tx, userID uuid.UUID, month time.Time) error {
	_, err := tx.Exec(ctx, `INSERT INTO monthly_usage(user_id,month_start,storage_bytes)
        VALUES($1,$2,COALESCE((SELECT storage_bytes FROM monthly_usage
            WHERE user_id=$1 AND month_start<$2 ORDER BY month_start DESC LIMIT 1),0))
        ON CONFLICT(user_id,month_start) DO NOTHING`, userID, month)
	return err
}

func lockMonthlyRow(ctx context.Context, tx pgx.Tx, userID uuid.UUID, month time.Time) (MonthlyUsage, error) {
	var current MonthlyUsage
	err := tx.QueryRow(ctx, `SELECT storage_bytes,embedding_tokens,input_tokens,output_tokens,total_tokens
        FROM monthly_usage WHERE user_id=$1 AND month_start=$2 FOR UPDATE`, userID, month).Scan(
		&current.StorageBytes, &current.EmbeddingTokens, &current.InputTokens, &current.OutputTokens, &current.TotalTokens)
	return current, err
}

func validateMonthly(usage MonthlyUsage) error {
	if usage.StorageBytes < 0 || usage.EmbeddingTokens < 0 || usage.InputTokens < 0 || usage.OutputTokens < 0 || usage.TotalTokens < 0 {
		return fmt.Errorf("monthly usage cannot be negative")
	}
	return nil
}

func insertUsageRecords(ctx context.Context, tx pgx.Tx, records []Record) error {
	for _, record := range records {
		metadata, err := json.Marshal(record.Metadata)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO usage_records
            (id,user_id,usage_type,quantity,unit,resource_type,resource_id,occurred_at,metadata)
            VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)`, record.ID, record.UserID, record.Type,
			record.Quantity, record.Unit, record.ResourceType, record.ResourceID, record.OccurredAt, metadata); err != nil {
			return err
		}
	}
	return nil
}
