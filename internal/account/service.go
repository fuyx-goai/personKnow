package account

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

const webTicketTTL = 5 * time.Minute

type Service struct {
	repository Repository
	wechat     WeChatClient
	tokens     *TokenManager
	now        func() time.Time
}

func NewService(repository Repository, wechat WeChatClient, tokens *TokenManager, now func() time.Time) *Service {
	return &Service{repository: repository, wechat: wechat, tokens: tokens, now: now}
}

func (service *Service) WeChatLogin(ctx context.Context, command LoginCommand) (AuthResult, error) {
	if strings.TrimSpace(command.Code) == "" {
		return AuthResult{}, ErrWeChatLogin
	}
	identity, err := service.wechat.ExchangeCode(ctx, command.Code)
	if err != nil {
		return AuthResult{}, ErrWeChatLogin
	}
	user, err := service.repository.FindOrCreateUser(ctx, identity, command.Profile)
	if err != nil {
		return AuthResult{}, err
	}
	if user.Status != UserActive {
		return AuthResult{}, ErrUserDisabled
	}
	return service.createSession(ctx, user, command.ClientType, command.DeviceLabel)
}

func (service *Service) Refresh(ctx context.Context, refreshToken string) (AuthResult, error) {
	now := service.now()
	newToken, newHash, expiresAt, err := service.tokens.NewRefresh(now)
	if err != nil {
		return AuthResult{}, err
	}
	session, err := service.repository.RotateSession(ctx, HashSecret(refreshToken), newHash, expiresAt)
	if err != nil {
		return AuthResult{}, ErrSessionInvalid
	}
	accessToken, err := service.tokens.IssueAccess(session.UserID, session.ID, now)
	if err != nil {
		return AuthResult{}, err
	}
	return AuthResult{AccessToken: accessToken, RefreshToken: newToken, ExpiresIn: int64(service.tokens.AccessTTL().Seconds())}, nil
}

func (service *Service) CreateWebTicket(ctx context.Context) (TicketResult, error) {
	rawSecret, err := randomSecret()
	if err != nil {
		return TicketResult{}, err
	}
	ticket := WebLoginTicket{
		ID:         uuid.New(),
		SecretHash: HashSecret(rawSecret),
		Status:     TicketPending,
		ExpiresAt:  service.now().Add(webTicketTTL),
	}
	if err := service.repository.CreateWebTicket(ctx, ticket); err != nil {
		return TicketResult{}, err
	}
	payload := fmt.Sprintf("personknow://web-login?id=%s&secret=%s", ticket.ID, rawSecret)
	return TicketResult{ID: ticket.ID, Secret: rawSecret, QRPayload: payload, ExpiresAt: ticket.ExpiresAt}, nil
}

func (service *Service) ConfirmWebTicket(ctx context.Context, userID, ticketID uuid.UUID, secret string) error {
	return service.repository.ConfirmWebTicket(ctx, ticketID, HashSecret(secret), userID, service.now())
}

func (service *Service) PollWebTicket(ctx context.Context, ticketID uuid.UUID, secret, deviceLabel string) (AuthResult, error) {
	now := service.now()
	status, err := service.repository.WebTicketStatus(ctx, ticketID, HashSecret(secret), now)
	if err != nil {
		return AuthResult{}, err
	}
	if status == TicketPending {
		return AuthResult{TicketStatus: TicketPending}, nil
	}
	if status != TicketConfirmed {
		return AuthResult{}, ErrTicketInvalid
	}
	rawRefresh, refreshHash, expiresAt, err := service.tokens.NewRefresh(now)
	if err != nil {
		return AuthResult{}, err
	}
	session := Session{ID: uuid.New(), RefreshTokenHash: refreshHash, ClientType: ClientWeb, DeviceLabel: deviceLabel, ExpiresAt: expiresAt, CreatedAt: now}
	user, err := service.repository.ConsumeWebTicket(ctx, ticketID, HashSecret(secret), session, now)
	if err != nil {
		return AuthResult{}, err
	}
	access, err := service.tokens.IssueAccess(user.ID, session.ID, now)
	if err != nil {
		return AuthResult{}, err
	}
	return AuthResult{TicketStatus: TicketConsumed, User: user, AccessToken: access, RefreshToken: rawRefresh, ExpiresIn: int64(service.tokens.AccessTTL().Seconds())}, nil
}

func (service *Service) ListSessions(ctx context.Context, userID uuid.UUID) ([]Session, error) {
	return service.repository.ListSessions(ctx, userID)
}

func (service *Service) CurrentUser(ctx context.Context, userID uuid.UUID) (User, error) {
	return service.repository.GetUser(ctx, userID)
}

func (service *Service) RevokeSession(ctx context.Context, userID, sessionID uuid.UUID) error {
	return service.repository.RevokeSession(ctx, userID, sessionID)
}

func (service *Service) RevokeOtherSessions(ctx context.Context, userID, currentSessionID uuid.UUID) error {
	return service.repository.RevokeOtherSessions(ctx, userID, currentSessionID)
}

func (service *Service) createSession(ctx context.Context, user User, client ClientType, device string) (AuthResult, error) {
	now := service.now()
	rawRefresh, refreshHash, expiresAt, err := service.tokens.NewRefresh(now)
	if err != nil {
		return AuthResult{}, err
	}
	session := Session{ID: uuid.New(), UserID: user.ID, RefreshTokenHash: refreshHash, ClientType: client, DeviceLabel: device, ExpiresAt: expiresAt, CreatedAt: now}
	if err := service.repository.CreateSession(ctx, session); err != nil {
		return AuthResult{}, err
	}
	access, err := service.tokens.IssueAccess(user.ID, session.ID, now)
	if err != nil {
		return AuthResult{}, err
	}
	return AuthResult{User: user, AccessToken: access, RefreshToken: rawRefresh, ExpiresIn: int64(service.tokens.AccessTTL().Seconds())}, nil
}

func randomSecret() (string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(bytes), nil
}
