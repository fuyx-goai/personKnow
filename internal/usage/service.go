package usage

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

type Service struct {
	repository Repository
	now        func() time.Time
}

func NewService(repository Repository, now func() time.Time) *Service {
	if now == nil {
		now = time.Now
	}
	return &Service{repository: repository, now: now}
}

func (service *Service) ReserveTokens(ctx context.Context, userID uuid.UUID, estimated int64) (Reservation, error) {
	if estimated <= 0 {
		return Reservation{}, ErrInvalidAmount
	}
	now := service.now()
	month := utcMonth(now)
	reservation := Reservation{ID: uuid.New(), UserID: userID, Estimated: estimated, MonthStart: month, CreatedAt: now}
	err := service.repository.UpdateMonthly(ctx, userID, month, func(current MonthlyUsage, limits Limits) (MonthlyUsage, []Record, error) {
		if current.TotalTokens+estimated > limits.MonthlyTokens {
			return current, nil, ErrTokenQuotaExceeded
		}
		current.TotalTokens += estimated
		return current, nil, nil
	})
	return reservation, err
}

func (service *Service) SettleTokens(ctx context.Context, reservation Reservation, actual Breakdown) error {
	if reservation.UserID == uuid.Nil || reservation.ID == uuid.Nil || reservation.Estimated <= 0 {
		return ErrReservationMismatch
	}
	if err := actual.Validate(); err != nil {
		return err
	}
	now := service.now()
	return service.repository.UpdateMonthly(ctx, reservation.UserID, reservation.MonthStart, func(current MonthlyUsage, _ Limits) (MonthlyUsage, []Record, error) {
		if current.TotalTokens < reservation.Estimated {
			return current, nil, ErrReservationMismatch
		}
		current.TotalTokens += actual.Total() - reservation.Estimated
		current.EmbeddingTokens += actual.Embedding
		current.InputTokens += actual.Input
		current.OutputTokens += actual.Output
		return current, tokenRecords(reservation, actual, now), nil
	})
}

func (service *Service) ApplyStorage(ctx context.Context, userID, resourceID uuid.UUID, delta int64) error {
	if delta == 0 {
		return nil
	}
	now := service.now()
	return service.repository.UpdateMonthly(ctx, userID, utcMonth(now), func(current MonthlyUsage, limits Limits) (MonthlyUsage, []Record, error) {
		next := current.StorageBytes + delta
		if next < 0 {
			return current, nil, ErrStorageUnderflow
		}
		if next > limits.StorageBytes {
			return current, nil, ErrStorageQuotaExceeded
		}
		current.StorageBytes = next
		record := newRecord(userID, UsageStorage, delta, "bytes", "document", resourceID, now, nil)
		return current, []Record{record}, nil
	})
}

func (service *Service) Summary(ctx context.Context, userID uuid.UUID, now time.Time) (Summary, error) {
	month := utcMonth(now)
	current, limits, err := service.repository.Summary(ctx, userID, month)
	if err != nil {
		return Summary{}, err
	}
	return Summary{
		MonthStart: month, StorageBytes: current.StorageBytes, StorageQuota: limits.StorageBytes,
		StorageRemaining: remaining(limits.StorageBytes, current.StorageBytes),
		EmbeddingTokens:  current.EmbeddingTokens, InputTokens: current.InputTokens,
		OutputTokens: current.OutputTokens, TotalTokens: current.TotalTokens,
		TokenQuota: limits.MonthlyTokens, TokenRemaining: remaining(limits.MonthlyTokens, current.TotalTokens),
	}, nil
}

func (service *Service) ListRecords(ctx context.Context, userID uuid.UUID, filter RecordFilter) ([]Record, error) {
	filter.Limit = normalizedLimit(filter.Limit)
	return service.repository.ListRecords(ctx, userID, filter)
}

func (service *Service) RecordAudit(ctx context.Context, command AuditCommand) error {
	if err := validateAudit(command); err != nil {
		return err
	}
	entry := AuditEntry{
		ActorUserID: command.ActorUserID, Action: command.Action, ResourceType: command.ResourceType,
		ResourceID: command.ResourceID, Result: command.Result, RequestID: command.RequestID,
		ClientType: command.ClientType, IPHash: command.IPHash, Metadata: command.Metadata, OccurredAt: service.now(),
	}
	return service.repository.AppendAudit(ctx, entry)
}

func (service *Service) ListAudit(ctx context.Context, actorID uuid.UUID, filter AuditFilter) ([]AuditEntry, error) {
	filter.Limit = normalizedLimit(filter.Limit)
	return service.repository.ListAudit(ctx, actorID, filter)
}

func utcMonth(value time.Time) time.Time {
	utc := value.UTC()
	return time.Date(utc.Year(), utc.Month(), 1, 0, 0, 0, 0, time.UTC)
}

func remaining(limit, used int64) int64 {
	if used >= limit {
		return 0
	}
	return limit - used
}

func normalizedLimit(limit int) int {
	if limit <= 0 || limit > 100 {
		return 20
	}
	return limit
}

func tokenRecords(reservation Reservation, actual Breakdown, now time.Time) []Record {
	quantities := []struct {
		kind  UsageType
		value int64
	}{{UsageEmbedding, actual.Embedding}, {UsageInput, actual.Input}, {UsageOutput, actual.Output}}
	records := make([]Record, 0, len(quantities))
	for _, quantity := range quantities {
		if quantity.value == 0 {
			continue
		}
		metadata := map[string]any{"reservation_id": reservation.ID.String()}
		records = append(records, newRecord(reservation.UserID, quantity.kind, quantity.value, "tokens", "reservation", reservation.ID, now, metadata))
	}
	return records
}

func newRecord(userID uuid.UUID, kind UsageType, quantity int64, unit, resourceType string, resourceID uuid.UUID, now time.Time, metadata map[string]any) Record {
	if metadata == nil {
		metadata = map[string]any{}
	}
	return Record{ID: uuid.New(), UserID: userID, Type: kind, Quantity: quantity, Unit: unit,
		ResourceType: resourceType, ResourceID: resourceID, OccurredAt: now, Metadata: metadata}
}

func validateAudit(command AuditCommand) error {
	if strings.TrimSpace(command.Action) == "" || len(command.Action) > 80 || strings.TrimSpace(command.ResourceType) == "" || len(command.ResourceType) > 40 {
		return ErrInvalidAuditMetadata
	}
	if command.Result == "" {
		command.Result = AuditSuccess
	}
	if command.Result != AuditSuccess && command.Result != AuditFailure {
		return ErrInvalidAuditMetadata
	}
	allowed := map[string]struct{}{"resource_name": {}, "status": {}, "error_code": {}, "changes": {}}
	for key, value := range command.Metadata {
		if _, ok := allowed[key]; !ok || !safeAuditValue(value, 0) {
			return fmt.Errorf("%w: %s", ErrInvalidAuditMetadata, key)
		}
	}
	return nil
}

func safeAuditValue(value any, depth int) bool {
	if depth > 2 {
		return false
	}
	switch typed := value.(type) {
	case nil, bool, int, int32, int64, float32, float64:
		return true
	case string:
		return len([]rune(typed)) <= 255
	case map[string]any:
		for key, child := range typed {
			lower := strings.ToLower(key)
			if strings.Contains(lower, "token") || strings.Contains(lower, "secret") || strings.Contains(lower, "content") || !safeAuditValue(child, depth+1) {
				return false
			}
		}
		return true
	default:
		return false
	}
}
