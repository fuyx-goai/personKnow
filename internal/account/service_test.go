package account

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
)

type fakeWeChatClient struct{}

func (fakeWeChatClient) ExchangeCode(_ context.Context, code string) (WeChatIdentity, error) {
	if code == "bad" {
		return WeChatIdentity{}, ErrWeChatLogin
	}
	return WeChatIdentity{AppID: "wx-test", OpenID: "openid-" + code}, nil
}

type fakeAccountRepository struct {
	mu       sync.Mutex
	user     User
	sessions map[string]Session
	tickets  map[uuid.UUID]WebLoginTicket
}

func newFakeAccountRepository() *fakeAccountRepository {
	return &fakeAccountRepository{
		user:     User{ID: uuid.New(), Nickname: "测试用户", Status: UserActive},
		sessions: make(map[string]Session),
		tickets:  make(map[uuid.UUID]WebLoginTicket),
	}
}

func (repo *fakeAccountRepository) FindOrCreateUser(context.Context, WeChatIdentity, Profile) (User, error) {
	return repo.user, nil
}

func (repo *fakeAccountRepository) GetUser(context.Context, uuid.UUID) (User, error) {
	return repo.user, nil
}

func (repo *fakeAccountRepository) CreateSession(_ context.Context, session Session) error {
	repo.mu.Lock()
	defer repo.mu.Unlock()
	repo.sessions[session.RefreshTokenHash] = session
	return nil
}

func (repo *fakeAccountRepository) RotateSession(_ context.Context, oldHash, newHash string, expiresAt time.Time) (Session, error) {
	repo.mu.Lock()
	defer repo.mu.Unlock()
	session, ok := repo.sessions[oldHash]
	if !ok || session.RevokedAt != nil || !session.ExpiresAt.After(time.Now()) {
		return Session{}, ErrSessionInvalid
	}
	delete(repo.sessions, oldHash)
	session.RefreshTokenHash = newHash
	session.ExpiresAt = expiresAt
	repo.sessions[newHash] = session
	return session, nil
}

func (repo *fakeAccountRepository) RevokeSession(_ context.Context, userID, sessionID uuid.UUID) error {
	repo.mu.Lock()
	defer repo.mu.Unlock()
	for hash, session := range repo.sessions {
		if session.UserID == userID && session.ID == sessionID {
			now := time.Now()
			session.RevokedAt = &now
			repo.sessions[hash] = session
			return nil
		}
	}
	return ErrSessionInvalid
}

func (repo *fakeAccountRepository) ListSessions(_ context.Context, userID uuid.UUID) ([]Session, error) {
	repo.mu.Lock()
	defer repo.mu.Unlock()
	var sessions []Session
	for _, session := range repo.sessions {
		if session.UserID == userID && session.RevokedAt == nil {
			sessions = append(sessions, session)
		}
	}
	return sessions, nil
}

func (repo *fakeAccountRepository) RevokeOtherSessions(_ context.Context, userID, currentSessionID uuid.UUID) error {
	repo.mu.Lock()
	defer repo.mu.Unlock()
	for hash, session := range repo.sessions {
		if session.UserID == userID && session.ID != currentSessionID {
			now := time.Now()
			session.RevokedAt = &now
			repo.sessions[hash] = session
		}
	}
	return nil
}

func (repo *fakeAccountRepository) CreateWebTicket(_ context.Context, ticket WebLoginTicket) error {
	repo.mu.Lock()
	defer repo.mu.Unlock()
	repo.tickets[ticket.ID] = ticket
	return nil
}

func (repo *fakeAccountRepository) ConfirmWebTicket(_ context.Context, id uuid.UUID, secretHash string, userID uuid.UUID, now time.Time) error {
	repo.mu.Lock()
	defer repo.mu.Unlock()
	ticket, ok := repo.tickets[id]
	if !ok || ticket.SecretHash != secretHash || ticket.Status != TicketPending || !ticket.ExpiresAt.After(now) {
		return ErrTicketInvalid
	}
	ticket.Status = TicketConfirmed
	ticket.ConfirmedUserID = &userID
	repo.tickets[id] = ticket
	return nil
}

func (repo *fakeAccountRepository) ConsumeWebTicket(_ context.Context, id uuid.UUID, secretHash string, session Session, now time.Time) (User, error) {
	repo.mu.Lock()
	defer repo.mu.Unlock()
	ticket, ok := repo.tickets[id]
	if !ok || ticket.SecretHash != secretHash || ticket.Status != TicketConfirmed || !ticket.ExpiresAt.After(now) {
		return User{}, ErrTicketInvalid
	}
	ticket.Status = TicketConsumed
	repo.tickets[id] = ticket
	session.UserID = *ticket.ConfirmedUserID
	repo.sessions[session.RefreshTokenHash] = session
	return repo.user, nil
}

func (repo *fakeAccountRepository) WebTicketStatus(_ context.Context, id uuid.UUID, secretHash string, now time.Time) (TicketStatus, error) {
	repo.mu.Lock()
	defer repo.mu.Unlock()
	ticket, ok := repo.tickets[id]
	if !ok || ticket.SecretHash != secretHash || !ticket.ExpiresAt.After(now) {
		return "", ErrTicketInvalid
	}
	return ticket.Status, nil
}

func TestConfirmAndConsumeWebTicketOnlyOnce(t *testing.T) {
	service := newAccountServiceForTest(t)
	ticket, err := service.CreateWebTicket(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	userID := service.repository.(*fakeAccountRepository).user.ID
	if err := service.ConfirmWebTicket(context.Background(), userID, ticket.ID, ticket.Secret); err != nil {
		t.Fatal(err)
	}
	if err := service.ConfirmWebTicket(context.Background(), userID, ticket.ID, ticket.Secret); !errors.Is(err, ErrTicketInvalid) {
		t.Fatalf("expected duplicate confirmation to fail, got %v", err)
	}
	result, err := service.PollWebTicket(context.Background(), ticket.ID, ticket.Secret, "浏览器")
	if err != nil {
		t.Fatal(err)
	}
	if result.AccessToken == "" || result.RefreshToken == "" {
		t.Fatal("confirmed ticket did not create tokens")
	}
	if _, err := service.PollWebTicket(context.Background(), ticket.ID, ticket.Secret, "浏览器"); !errors.Is(err, ErrTicketInvalid) {
		t.Fatalf("expected consumed ticket to fail, got %v", err)
	}
}

func TestRefreshRotatesRefreshToken(t *testing.T) {
	service := newAccountServiceForTest(t)
	login, err := service.WeChatLogin(context.Background(), LoginCommand{Code: "one", ClientType: ClientMiniProgram})
	if err != nil {
		t.Fatal(err)
	}
	refreshed, err := service.Refresh(context.Background(), login.RefreshToken)
	if err != nil {
		t.Fatal(err)
	}
	if refreshed.RefreshToken == login.RefreshToken {
		t.Fatal("refresh token was not rotated")
	}
	if _, err := service.Refresh(context.Background(), login.RefreshToken); !errors.Is(err, ErrSessionInvalid) {
		t.Fatalf("old refresh token should be invalid, got %v", err)
	}
}

func TestAccessTokenCarriesUserAndSession(t *testing.T) {
	manager := NewTokenManager([]byte("01234567890123456789012345678901"), 15*time.Minute, 30*24*time.Hour)
	userID, sessionID := uuid.New(), uuid.New()
	token, err := manager.IssueAccess(userID, sessionID, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	identity, err := manager.ParseAccess(token)
	if err != nil {
		t.Fatal(err)
	}
	if identity.UserID != userID || identity.SessionID != sessionID {
		t.Fatalf("unexpected identity: %#v", identity)
	}
}

func newAccountServiceForTest(t *testing.T) *Service {
	t.Helper()
	repository := newFakeAccountRepository()
	manager := NewTokenManager([]byte("01234567890123456789012345678901"), 15*time.Minute, 30*24*time.Hour)
	return NewService(repository, fakeWeChatClient{}, manager, time.Now)
}
