package repo

import (
	"context"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	. "knowledge-base/internal/account/entity"
	platformpostgres "knowledge-base/internal/platform/postgres"
)

const personalProPlanID = "00000000-0000-7000-8000-000000000001"

func (repo *PostgresRepository) FindOrCreateUser(ctx context.Context, identity WeChatIdentity, profile Profile) (User, error) {
	openidCipher, openidHash, err := repo.codec.Encode(identity.OpenID)
	if err != nil {
		return User{}, err
	}
	var unionCipher []byte
	var unionHash *string
	if identity.UnionID != "" {
		encoded, hash, encodeErr := repo.codec.Encode(identity.UnionID)
		if encodeErr != nil {
			return User{}, encodeErr
		}
		unionCipher, unionHash = encoded, &hash
	}
	var user User
	err = platformpostgres.WithinTx(ctx, repo.pool, func(tx pgx.Tx) error {
		lockKey := identity.AppID + ":" + openidHash
		if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock(hashtextextended($1,0))", lockKey); err != nil {
			return err
		}
		found, err := findUserByIdentity(ctx, tx, identity.AppID, openidHash)
		if err == nil {
			user = found
			return updateUserProfile(ctx, tx, user.ID, profile)
		}
		if err != pgx.ErrNoRows {
			return err
		}
		user = User{ID: uuid.New(), Nickname: normalizedNickname(profile.Nickname), AvatarURL: profile.AvatarURL, Status: UserActive}
		if _, err := tx.Exec(ctx, `INSERT INTO users(id,nickname,avatar_url,last_login_at) VALUES($1,$2,$3,now())`, user.ID, user.Nickname, nullIfEmpty(user.AvatarURL)); err != nil {
			return err
		}
		identityID := uuid.New()
		if _, err := tx.Exec(ctx, `INSERT INTO wechat_identities
            (id,user_id,app_id,openid_ciphertext,openid_hash,unionid_ciphertext,unionid_hash)
            VALUES($1,$2,$3,$4,$5,$6,$7)`, identityID, user.ID, identity.AppID, openidCipher, openidHash, unionCipher, unionHash); err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `INSERT INTO user_plans(id,user_id,plan_id) VALUES($1,$2,$3)`, uuid.New(), user.ID, personalProPlanID)
		return err
	})
	return user, err
}

func (repo *PostgresRepository) GetUser(ctx context.Context, userID uuid.UUID) (User, error) {
	var user User
	err := repo.pool.QueryRow(ctx, `SELECT id,nickname,COALESCE(avatar_url,''),status
        FROM users WHERE id=$1 AND deleted_at IS NULL`, userID).
		Scan(&user.ID, &user.Nickname, &user.AvatarURL, &user.Status)
	if err == pgx.ErrNoRows {
		return User{}, ErrSessionInvalid
	}
	return user, err
}

func findUserByIdentity(ctx context.Context, tx pgx.Tx, appID, openidHash string) (User, error) {
	var user User
	err := tx.QueryRow(ctx, `SELECT u.id,u.nickname,COALESCE(u.avatar_url,''),u.status
        FROM users u JOIN wechat_identities w ON w.user_id=u.id
        WHERE w.app_id=$1 AND w.openid_hash=$2 AND u.deleted_at IS NULL`, appID, openidHash).
		Scan(&user.ID, &user.Nickname, &user.AvatarURL, &user.Status)
	return user, err
}

func updateUserProfile(ctx context.Context, tx pgx.Tx, userID uuid.UUID, profile Profile) error {
	_, err := tx.Exec(ctx, `UPDATE users SET nickname=COALESCE(NULLIF($2,''),nickname),
        avatar_url=COALESCE(NULLIF($3,''),avatar_url),last_login_at=now(),updated_at=now() WHERE id=$1`,
		userID, strings.TrimSpace(profile.Nickname), strings.TrimSpace(profile.AvatarURL))
	return err
}

func normalizedNickname(value string) string {
	if nickname := strings.TrimSpace(value); nickname != "" {
		return nickname
	}
	return "微信用户"
}

func nullIfEmpty(value string) any {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return value
}
