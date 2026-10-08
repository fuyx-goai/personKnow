package account

import (
	"context"
	"time"

	"github.com/google/uuid"
)

type WeChatClient interface {
	ExchangeCode(context.Context, string) (WeChatIdentity, error)
}

type Repository interface {
	FindOrCreateUser(context.Context, WeChatIdentity, Profile) (User, error)
	GetUser(context.Context, uuid.UUID) (User, error)
	CreateSession(context.Context, Session) error
	RotateSession(context.Context, string, string, time.Time) (Session, error)
	RevokeSession(context.Context, uuid.UUID, uuid.UUID) error
	ListSessions(context.Context, uuid.UUID) ([]Session, error)
	RevokeOtherSessions(context.Context, uuid.UUID, uuid.UUID) error
	CreateWebTicket(context.Context, WebLoginTicket) error
	ConfirmWebTicket(context.Context, uuid.UUID, string, uuid.UUID, time.Time) error
	ConsumeWebTicket(context.Context, uuid.UUID, string, Session, time.Time) (User, error)
	WebTicketStatus(context.Context, uuid.UUID, string, time.Time) (TicketStatus, error)
}
