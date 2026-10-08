package chat

import (
	"context"
	"time"

	"github.com/google/uuid"

	platformvector "knowledge-base/internal/platform/vector"
	usage "knowledge-base/internal/usage"
)

type Repository interface {
	GetSession(context.Context, uuid.UUID, uuid.UUID) (Session, error)
	ResolveScope(context.Context, uuid.UUID, Session) (platformvector.SearchScope, RetrievalSettings, error)
	StartExchange(context.Context, Session, string, time.Time) (Message, Message, error)
	FinishExchange(context.Context, Message, string, MessageStatus, usage.Breakdown, []Reference, time.Time) error
}

type SessionRepository interface {
	Repository
	CreateSession(context.Context, Session) error
	ListSessions(context.Context, uuid.UUID, SessionFilter) ([]Session, error)
	ListMessages(context.Context, uuid.UUID, uuid.UUID, SessionFilter) ([]Message, error)
	DeleteSession(context.Context, uuid.UUID, uuid.UUID, time.Time) error
}

type Engine interface {
	Stream(context.Context, string, platformvector.SearchScope, RetrievalSettings, func(string) error) (EngineResult, error)
}

type TokenQuota interface {
	ReserveTokens(context.Context, uuid.UUID, int64) (usage.Reservation, error)
	SettleTokens(context.Context, usage.Reservation, usage.Breakdown) error
}
