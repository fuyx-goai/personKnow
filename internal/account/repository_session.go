package account

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func (repo *PostgresRepository) CreateSession(ctx context.Context, session Session) error {
	_, err := repo.pool.Exec(ctx, `INSERT INTO auth_sessions
        (id,user_id,refresh_token_hash,client_type,device_label,expires_at,created_at)
        VALUES($1,$2,$3,$4,$5,$6,$7)`, session.ID, session.UserID, session.RefreshTokenHash,
		session.ClientType, nullIfEmpty(session.DeviceLabel), session.ExpiresAt, session.CreatedAt)
	return err
}

func (repo *PostgresRepository) RotateSession(ctx context.Context, oldHash, newHash string, expiresAt time.Time) (Session, error) {
	var session Session
	err := repo.pool.QueryRow(ctx, `UPDATE auth_sessions SET refresh_token_hash=$2,expires_at=$3,last_used_at=now()
        WHERE refresh_token_hash=$1 AND revoked_at IS NULL AND expires_at>now()
        RETURNING id,user_id,refresh_token_hash,client_type,COALESCE(device_label,''),expires_at,revoked_at,last_used_at,created_at`,
		oldHash, newHash, expiresAt).Scan(&session.ID, &session.UserID, &session.RefreshTokenHash,
		&session.ClientType, &session.DeviceLabel, &session.ExpiresAt, &session.RevokedAt, &session.LastUsedAt, &session.CreatedAt)
	if err == pgx.ErrNoRows {
		return Session{}, ErrSessionInvalid
	}
	return session, err
}

func (repo *PostgresRepository) RevokeSession(ctx context.Context, userID, sessionID uuid.UUID) error {
	result, err := repo.pool.Exec(ctx, `UPDATE auth_sessions SET revoked_at=now()
        WHERE id=$1 AND user_id=$2 AND revoked_at IS NULL`, sessionID, userID)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return ErrSessionInvalid
	}
	return nil
}

func (repo *PostgresRepository) RevokeOtherSessions(ctx context.Context, userID, currentSessionID uuid.UUID) error {
	_, err := repo.pool.Exec(ctx, `UPDATE auth_sessions SET revoked_at=now()
        WHERE user_id=$1 AND id<>$2 AND revoked_at IS NULL`, userID, currentSessionID)
	return err
}

func (repo *PostgresRepository) ListSessions(ctx context.Context, userID uuid.UUID) ([]Session, error) {
	rows, err := repo.pool.Query(ctx, `SELECT id,user_id,client_type,COALESCE(device_label,''),expires_at,last_used_at,created_at
        FROM auth_sessions WHERE user_id=$1 AND revoked_at IS NULL AND expires_at>now() ORDER BY created_at DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var sessions []Session
	for rows.Next() {
		var session Session
		if err := rows.Scan(&session.ID, &session.UserID, &session.ClientType, &session.DeviceLabel,
			&session.ExpiresAt, &session.LastUsedAt, &session.CreatedAt); err != nil {
			return nil, err
		}
		sessions = append(sessions, session)
	}
	return sessions, rows.Err()
}
