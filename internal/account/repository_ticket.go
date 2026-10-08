package account

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	platformpostgres "knowledge-base/internal/platform/postgres"
)

func (repo *PostgresRepository) CreateWebTicket(ctx context.Context, ticket WebLoginTicket) error {
	_, err := repo.pool.Exec(ctx, `INSERT INTO web_login_tickets(id,secret_hash,status,expires_at)
        VALUES($1,$2,$3,$4)`, ticket.ID, ticket.SecretHash, ticket.Status, ticket.ExpiresAt)
	return err
}

func (repo *PostgresRepository) ConfirmWebTicket(ctx context.Context, id uuid.UUID, secretHash string, userID uuid.UUID, now time.Time) error {
	result, err := repo.pool.Exec(ctx, `UPDATE web_login_tickets
        SET status='confirmed',confirmed_user_id=$3,confirmed_at=$4
        WHERE id=$1 AND secret_hash=$2 AND status='pending' AND expires_at>$4`, id, secretHash, userID, now)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return ErrTicketInvalid
	}
	return nil
}

func (repo *PostgresRepository) WebTicketStatus(ctx context.Context, id uuid.UUID, secretHash string, now time.Time) (TicketStatus, error) {
	var status TicketStatus
	err := repo.pool.QueryRow(ctx, `SELECT status FROM web_login_tickets
        WHERE id=$1 AND secret_hash=$2 AND expires_at>$3`, id, secretHash, now).Scan(&status)
	if err == pgx.ErrNoRows {
		return "", ErrTicketInvalid
	}
	return status, err
}

func (repo *PostgresRepository) ConsumeWebTicket(ctx context.Context, id uuid.UUID, secretHash string, session Session, now time.Time) (User, error) {
	var user User
	err := platformpostgres.WithinTx(ctx, repo.pool, func(tx pgx.Tx) error {
		var userID uuid.UUID
		err := tx.QueryRow(ctx, `UPDATE web_login_tickets SET status='consumed',consumed_at=$3
            WHERE id=$1 AND secret_hash=$2 AND status='confirmed' AND expires_at>$3
            RETURNING confirmed_user_id`, id, secretHash, now).Scan(&userID)
		if err == pgx.ErrNoRows {
			return ErrTicketInvalid
		}
		if err != nil {
			return err
		}
		session.UserID = userID
		if _, err := tx.Exec(ctx, `INSERT INTO auth_sessions
            (id,user_id,refresh_token_hash,client_type,device_label,expires_at,created_at)
            VALUES($1,$2,$3,$4,$5,$6,$7)`, session.ID, session.UserID, session.RefreshTokenHash,
			session.ClientType, nullIfEmpty(session.DeviceLabel), session.ExpiresAt, session.CreatedAt); err != nil {
			return err
		}
		return tx.QueryRow(ctx, `SELECT id,nickname,COALESCE(avatar_url,''),status FROM users WHERE id=$1 AND deleted_at IS NULL`, userID).
			Scan(&user.ID, &user.Nickname, &user.AvatarURL, &user.Status)
	})
	return user, err
}
