package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	. "knowledge-base/internal/chat/entity"
	platformvector "knowledge-base/internal/platform/vector"
	usage "knowledge-base/internal/usage/entity"
)

type chatRepository struct {
	session     Session
	scope       platformvector.SearchScope
	messages    []Message
	finalStatus MessageStatus
	finalAnswer string
	references  []Reference
}

func (repository *chatRepository) GetSession(_ context.Context, userID, sessionID uuid.UUID) (Session, error) {
	if repository.session.UserID != userID || repository.session.ID != sessionID {
		return Session{}, ErrSessionNotFound
	}
	return repository.session, nil
}

func (repository *chatRepository) ResolveScope(context.Context, uuid.UUID, Session) (platformvector.SearchScope, RetrievalSettings, error) {
	return repository.scope, RetrievalSettings{TopK: 5}, nil
}

func (repository *chatRepository) StartExchange(_ context.Context, session Session, question string, now time.Time) (Message, Message, error) {
	userMessage := Message{ID: uuid.New(), SessionID: session.ID, Role: RoleUser, Content: question, Status: MessageComplete, CreatedAt: now}
	assistant := Message{ID: uuid.New(), SessionID: session.ID, Role: RoleAssistant, Status: MessageStreaming, CreatedAt: now}
	repository.messages = append(repository.messages, userMessage, assistant)
	return userMessage, assistant, nil
}

func (repository *chatRepository) FinishExchange(_ context.Context, assistant Message, answer string, status MessageStatus, breakdown usage.Breakdown, references []Reference, _ time.Time) error {
	repository.finalStatus, repository.finalAnswer = status, answer
	repository.references = append(repository.references, references...)
	return nil
}

type chatQuota struct {
	err     error
	settled usage.Breakdown
}

func (quota *chatQuota) ReserveTokens(_ context.Context, userID uuid.UUID, estimated int64) (usage.Reservation, error) {
	if quota.err != nil {
		return usage.Reservation{}, quota.err
	}
	return usage.Reservation{ID: uuid.New(), UserID: userID, Estimated: estimated, MonthStart: time.Now()}, nil
}

func (quota *chatQuota) SettleTokens(_ context.Context, _ usage.Reservation, breakdown usage.Breakdown) error {
	quota.settled = breakdown
	return nil
}

type chatEngine struct {
	called bool
	scope  platformvector.SearchScope
	err    error
}

func (engine *chatEngine) Stream(_ context.Context, question string, scope platformvector.SearchScope, settings RetrievalSettings, onDelta func(string) error) (EngineResult, error) {
	engine.called, engine.scope = true, scope
	if engine.err != nil {
		return EngineResult{}, engine.err
	}
	if err := onDelta("答案"); err != nil {
		return EngineResult{Usage: usage.Breakdown{Input: 3, Output: 1}}, err
	}
	return EngineResult{
		Usage:      usage.Breakdown{Input: 3, Output: 2},
		References: []Reference{{DocumentID: uuid.New(), ContentVersionID: uuid.New(), ChunkID: "chunk-1", SourceName: "notes.md", Rank: 1}},
	}, nil
}

func TestStreamUsesFreshServerResolvedScopeAndPersistsUsage(t *testing.T) {
	userID, sessionID, libraryID := uuid.New(), uuid.New(), uuid.New()
	repository := &chatRepository{
		session: Session{ID: sessionID, UserID: userID, ScopeType: ScopeGlobal},
		scope:   platformvector.SearchScope{LibraryIDs: []uuid.UUID{libraryID}},
	}
	quota, engine := &chatQuota{}, &chatEngine{}
	service := NewService(ServiceDependencies{Repository: repository, Engine: engine, Quota: quota, Now: time.Now})
	var eventTypes []EventType
	err := service.Stream(context.Background(), userID, sessionID, "问题", func(event Event) error {
		eventTypes = append(eventTypes, event.Type)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if !engine.called || len(engine.scope.LibraryIDs) != 1 || engine.scope.LibraryIDs[0] != libraryID {
		t.Fatalf("engine did not receive server scope: %+v", engine.scope)
	}
	if repository.finalStatus != MessageComplete || repository.finalAnswer != "答案" || len(repository.references) != 1 {
		t.Fatalf("exchange not persisted: status=%s answer=%q refs=%d", repository.finalStatus, repository.finalAnswer, len(repository.references))
	}
	if quota.settled.Input != 3 || quota.settled.Output != 2 || len(eventTypes) < 4 {
		t.Fatalf("usage or SSE events missing: usage=%+v events=%+v", quota.settled, eventTypes)
	}
}

func TestQuotaExceededRejectsBeforeCallingEngine(t *testing.T) {
	userID, sessionID := uuid.New(), uuid.New()
	repository := &chatRepository{session: Session{ID: sessionID, UserID: userID}}
	engine := &chatEngine{}
	service := NewService(ServiceDependencies{
		Repository: repository, Engine: engine, Quota: &chatQuota{err: usage.ErrTokenQuotaExceeded}, Now: time.Now,
	})
	err := service.Stream(context.Background(), userID, sessionID, "问题", func(Event) error { return nil })
	if !errors.Is(err, usage.ErrTokenQuotaExceeded) || engine.called || len(repository.messages) != 0 {
		t.Fatalf("quota guard failed: err=%v engine=%v messages=%d", err, engine.called, len(repository.messages))
	}
}

func TestClientInterruptionPersistsInterruptedMessage(t *testing.T) {
	userID, sessionID := uuid.New(), uuid.New()
	repository := &chatRepository{session: Session{ID: sessionID, UserID: userID}}
	service := NewService(ServiceDependencies{Repository: repository, Engine: &chatEngine{}, Quota: &chatQuota{}, Now: time.Now})
	clientStopped := errors.New("client stopped")
	err := service.Stream(context.Background(), userID, sessionID, "问题", func(event Event) error {
		if event.Type == EventDelta {
			return clientStopped
		}
		return nil
	})
	if !errors.Is(err, clientStopped) || repository.finalStatus != MessageInterrupted {
		t.Fatalf("interruption not persisted: err=%v status=%s", err, repository.finalStatus)
	}
}
