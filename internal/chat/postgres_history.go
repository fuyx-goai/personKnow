package chat

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	platformpostgres "knowledge-base/internal/platform/postgres"
)

func (repository *PostgresRepository) ListMessages(ctx context.Context, userID, sessionID uuid.UUID, filter SessionFilter) ([]Message, error) {
	rows, err := repository.pool.Query(ctx, `SELECT m.id,m.session_id,m.role,m.content,m.status,m.input_tokens,
        m.output_tokens,m.total_tokens,COALESCE(m.request_id,''),m.created_at,m.updated_at
        FROM chat_messages m JOIN chat_sessions s ON s.id=m.session_id
        WHERE m.session_id=$1 AND s.user_id=$2 AND s.deleted_at IS NULL
        AND ($3::timestamptz IS NULL OR m.created_at<$3) ORDER BY m.created_at DESC,m.id DESC LIMIT $4`,
		sessionID, userID, filter.Before, filter.Limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	messages := make([]Message, 0, filter.Limit)
	for rows.Next() {
		var message Message
		if err := rows.Scan(&message.ID, &message.SessionID, &message.Role, &message.Content, &message.Status,
			&message.InputTokens, &message.OutputTokens, &message.TotalTokens, &message.RequestID,
			&message.CreatedAt, &message.UpdatedAt); err != nil {
			return nil, err
		}
		messages = append(messages, message)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for index := range messages {
		references, err := repository.references(ctx, messages[index].ID)
		if err != nil {
			return nil, err
		}
		messages[index].References = references
	}
	return messages, nil
}

func (repository *PostgresRepository) references(ctx context.Context, messageID uuid.UUID) ([]Reference, error) {
	rows, err := repository.pool.Query(ctx, `SELECT document_id,content_version_id,chunk_id,source_name_snapshot,
        location_label,excerpt,similarity,rank FROM message_references WHERE message_id=$1 ORDER BY rank`, messageID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var references []Reference
	for rows.Next() {
		var reference Reference
		if err := rows.Scan(&reference.DocumentID, &reference.ContentVersionID, &reference.ChunkID,
			&reference.SourceName, &reference.LocationLabel, &reference.Excerpt, &reference.Similarity, &reference.Rank); err != nil {
			return nil, err
		}
		references = append(references, reference)
	}
	return references, rows.Err()
}

func (repository *PostgresRepository) DeleteSession(ctx context.Context, userID, sessionID uuid.UUID, now time.Time) error {
	return platformpostgres.WithinTx(ctx, repository.pool, func(tx pgx.Tx) error {
		result, err := tx.Exec(ctx, `UPDATE chat_sessions SET deleted_at=$3,updated_at=$3
            WHERE id=$1 AND user_id=$2 AND deleted_at IS NULL`, sessionID, userID, now)
		if err != nil {
			return err
		}
		if result.RowsAffected() == 0 {
			return ErrSessionNotFound
		}
		_, err = tx.Exec(ctx, `INSERT INTO audit_logs
            (actor_user_id,action,resource_type,resource_id,result,metadata,occurred_at)
            VALUES($1,'chat.session.delete','chat_session',$2,'success','{"status":"deleted"}'::jsonb,$3)`, userID, sessionID, now)
		return err
	})
}
