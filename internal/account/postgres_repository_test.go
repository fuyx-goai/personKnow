package account

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"

	platformpostgres "knowledge-base/internal/platform/postgres"
	"knowledge-base/pkg/config"
)

func TestPostgresRepositoryConsumesTicketOnce(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}
	ctx := context.Background()
	pool, err := platformpostgres.Open(ctx, config.DatabaseConfig{URL: databaseURL, MaxConns: 4})
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	if err := platformpostgres.Migrate(ctx, pool); err != nil {
		t.Fatal(err)
	}
	codec, err := NewIdentityCodec([]byte("01234567890123456789012345678901"))
	if err != nil {
		t.Fatal(err)
	}
	repository := NewPostgresRepository(pool, codec)
	identity := WeChatIdentity{AppID: "wx-test", OpenID: "openid-" + uuid.NewString()}
	user, err := repository.FindOrCreateUser(ctx, identity, Profile{Nickname: "仓储测试"})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	ticket := WebLoginTicket{ID: uuid.New(), SecretHash: HashSecret("ticket"), Status: TicketPending, ExpiresAt: now.Add(time.Minute)}
	if err := repository.CreateWebTicket(ctx, ticket); err != nil {
		t.Fatal(err)
	}
	if err := repository.ConfirmWebTicket(ctx, ticket.ID, ticket.SecretHash, user.ID, now); err != nil {
		t.Fatal(err)
	}
	session := Session{ID: uuid.New(), RefreshTokenHash: HashSecret("refresh"), ClientType: ClientWeb, ExpiresAt: now.Add(time.Hour), CreatedAt: now}
	if _, err := repository.ConsumeWebTicket(ctx, ticket.ID, ticket.SecretHash, session, now); err != nil {
		t.Fatal(err)
	}
	if _, err := repository.ConsumeWebTicket(ctx, ticket.ID, ticket.SecretHash, session, now); !errors.Is(err, ErrTicketInvalid) {
		t.Fatalf("expected duplicate consume to fail, got %v", err)
	}
}
