package account

import "github.com/jackc/pgx/v5/pgxpool"

type PostgresRepository struct {
	pool  *pgxpool.Pool
	codec *IdentityCodec
}

func NewPostgresRepository(pool *pgxpool.Pool, codec *IdentityCodec) *PostgresRepository {
	return &PostgresRepository{pool: pool, codec: codec}
}
