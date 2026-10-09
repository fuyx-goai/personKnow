package service

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"

	. "knowledge-base/internal/chat/entity"
	usage "knowledge-base/internal/usage/entity"
)

type ServiceDependencies struct {
	Repository Repository
	Engine     Engine
	Quota      TokenQuota
	Now        func() time.Time
}

type Service struct {
	repository Repository
	sessions   SessionRepository
	engine     Engine
	quota      TokenQuota
	now        func() time.Time
}

func NewService(dependencies ServiceDependencies) *Service {
	if dependencies.Now == nil {
		dependencies.Now = time.Now
	}
	service := &Service{
		repository: dependencies.Repository, engine: dependencies.Engine,
		quota: dependencies.Quota, now: dependencies.Now,
	}
	service.sessions, _ = dependencies.Repository.(SessionRepository)
	return service
}

func (service *Service) CreateSession(ctx context.Context, userID uuid.UUID, command CreateSessionCommand) (Session, error) {
	if service.sessions == nil {
		return Session{}, ErrHistoryUnavailable
	}
	if command.ScopeType == "" {
		command.ScopeType = ScopeGlobal
	}
	if (command.ScopeType == ScopeSingleLibrary) != (command.LibraryID != nil) || (command.ScopeType != ScopeSingleLibrary && command.ScopeType != ScopeGlobal) {
		return Session{}, ErrInvalidScope
	}
	now := service.now()
	title := strings.TrimSpace(command.Title)
	if title == "" {
		title = "新建问答"
	}
	session := Session{ID: uuid.New(), UserID: userID, ScopeType: command.ScopeType, LibraryID: command.LibraryID, Title: title, CreatedAt: now, UpdatedAt: now}
	if _, _, err := service.sessions.ResolveScope(ctx, userID, session); err != nil {
		return Session{}, err
	}
	if err := service.sessions.CreateSession(ctx, session); err != nil {
		return Session{}, err
	}
	return session, nil
}

func (service *Service) ListSessions(ctx context.Context, userID uuid.UUID, filter SessionFilter) ([]Session, error) {
	if service.sessions == nil {
		return nil, ErrHistoryUnavailable
	}
	filter.Limit = normalizeLimit(filter.Limit)
	return service.sessions.ListSessions(ctx, userID, filter)
}

func (service *Service) History(ctx context.Context, userID, sessionID uuid.UUID, filter SessionFilter) (Session, []Message, error) {
	if service.sessions == nil {
		return Session{}, nil, ErrHistoryUnavailable
	}
	session, err := service.sessions.GetSession(ctx, userID, sessionID)
	if err != nil {
		return Session{}, nil, err
	}
	filter.Limit = normalizeLimit(filter.Limit)
	messages, err := service.sessions.ListMessages(ctx, userID, sessionID, filter)
	return session, messages, err
}

func (service *Service) DeleteSession(ctx context.Context, userID, sessionID uuid.UUID) error {
	if service.sessions == nil {
		return ErrHistoryUnavailable
	}
	return service.sessions.DeleteSession(ctx, userID, sessionID, service.now())
}

func (service *Service) Stream(ctx context.Context, userID, sessionID uuid.UUID, question string, emit func(Event) error) error {
	session, err := service.repository.GetSession(ctx, userID, sessionID)
	if err != nil {
		return err
	}
	scope, settings, err := service.repository.ResolveScope(ctx, userID, session)
	if err != nil {
		return err
	}
	reservation, err := service.quota.ReserveTokens(ctx, userID, estimatedChatTokens(question))
	if err != nil {
		return err
	}
	_, assistant, err := service.repository.StartExchange(ctx, session, question, service.now())
	if err != nil {
		_ = service.quota.SettleTokens(ctx, reservation, usage.Breakdown{})
		return err
	}
	var answer strings.Builder
	var streamError error
	result, engineErr := service.engine.Stream(ctx, question, scope, settings, func(delta string) error {
		answer.WriteString(delta)
		streamError = emit(Event{Type: EventDelta, Data: map[string]any{"delta": delta}})
		return streamError
	})
	status := MessageComplete
	if engineErr != nil {
		status = MessageFailed
		if streamError != nil || errors.Is(engineErr, context.Canceled) {
			status = MessageInterrupted
		}
	}
	settleErr := service.quota.SettleTokens(ctx, reservation, result.Usage)
	finishErr := service.repository.FinishExchange(ctx, assistant, answer.String(), status, result.Usage, result.References, service.now())
	if engineErr != nil {
		if status == MessageFailed {
			_ = emit(Event{Type: EventError, Data: map[string]any{"code": "CHAT_FAILED", "message": "问答生成失败"}})
		}
		return engineErr
	}
	if settleErr != nil {
		return settleErr
	}
	if finishErr != nil {
		return finishErr
	}
	for _, reference := range result.References {
		if err := emit(Event{Type: EventReference, Data: reference}); err != nil {
			return err
		}
	}
	if err := emit(Event{Type: EventUsage, Data: result.Usage}); err != nil {
		return err
	}
	return emit(Event{Type: EventDone, Data: map[string]any{"message_id": assistant.ID}})
}

func estimatedChatTokens(question string) int64 {
	input := max(1, (len([]rune(question))+3)/4)
	return int64(input + 1024)
}

func normalizeLimit(limit int) int {
	if limit <= 0 || limit > 100 {
		return 20
	}
	return limit
}
