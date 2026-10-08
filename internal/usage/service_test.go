package usage

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
)

type memoryRepository struct {
	mu        sync.Mutex
	limits    Limits
	months    map[monthKey]MonthlyUsage
	records   []Record
	audits    []AuditEntry
	lastMonth time.Time
}

type monthKey struct {
	userID uuid.UUID
	month  string
}

func newMemoryRepository(limits Limits) *memoryRepository {
	return &memoryRepository{limits: limits, months: make(map[monthKey]MonthlyUsage)}
}

func (repository *memoryRepository) UpdateMonthly(_ context.Context, userID uuid.UUID, month time.Time, mutate MonthlyMutation) error {
	repository.mu.Lock()
	defer repository.mu.Unlock()
	repository.lastMonth = month
	key := monthKey{userID: userID, month: month.Format(time.DateOnly)}
	current := repository.months[key]
	next, records, err := mutate(current, repository.limits)
	if err != nil {
		return err
	}
	repository.months[key] = next
	repository.records = append(repository.records, records...)
	return nil
}

func (repository *memoryRepository) Summary(_ context.Context, userID uuid.UUID, month time.Time) (MonthlyUsage, Limits, error) {
	repository.mu.Lock()
	defer repository.mu.Unlock()
	return repository.months[monthKey{userID: userID, month: month.Format(time.DateOnly)}], repository.limits, nil
}

func (repository *memoryRepository) ListRecords(_ context.Context, userID uuid.UUID, _ RecordFilter) ([]Record, error) {
	repository.mu.Lock()
	defer repository.mu.Unlock()
	result := make([]Record, 0, len(repository.records))
	for _, record := range repository.records {
		if record.UserID == userID {
			result = append(result, record)
		}
	}
	return result, nil
}

func (repository *memoryRepository) AppendAudit(_ context.Context, entry AuditEntry) error {
	repository.mu.Lock()
	defer repository.mu.Unlock()
	repository.audits = append(repository.audits, entry)
	return nil
}

func (repository *memoryRepository) ListAudit(_ context.Context, actorID uuid.UUID, _ AuditFilter) ([]AuditEntry, error) {
	repository.mu.Lock()
	defer repository.mu.Unlock()
	result := make([]AuditEntry, 0, len(repository.audits))
	for _, entry := range repository.audits {
		if entry.ActorUserID != nil && *entry.ActorUserID == actorID {
			result = append(result, entry)
		}
	}
	return result, nil
}

func TestServiceUsesUTCMonthBoundary(t *testing.T) {
	now := time.Date(2026, time.February, 28, 17, 30, 0, 0, time.FixedZone("PST", -8*60*60))
	repository := newMemoryRepository(Limits{StorageBytes: 1000, MonthlyTokens: 1000})
	service := NewService(repository, func() time.Time { return now })

	if _, err := service.ReserveTokens(context.Background(), uuid.New(), 10); err != nil {
		t.Fatal(err)
	}
	if got := repository.lastMonth.Format(time.DateOnly); got != "2026-03-01" {
		t.Fatalf("month_start = %s, want 2026-03-01", got)
	}
}

func TestConcurrentReservationsCannotExceedQuota(t *testing.T) {
	userID := uuid.New()
	repository := newMemoryRepository(Limits{MonthlyTokens: 100})
	service := NewService(repository, time.Now)
	start := make(chan struct{})
	errorsChannel := make(chan error, 2)
	for range 2 {
		go func() {
			<-start
			_, err := service.ReserveTokens(context.Background(), userID, 60)
			errorsChannel <- err
		}()
	}
	close(start)
	var successes, rejected int
	for range 2 {
		err := <-errorsChannel
		if err == nil {
			successes++
		} else if errors.Is(err, ErrTokenQuotaExceeded) {
			rejected++
		} else {
			t.Fatalf("unexpected reservation error: %v", err)
		}
	}
	if successes != 1 || rejected != 1 {
		t.Fatalf("successes=%d rejected=%d, want 1 and 1", successes, rejected)
	}
}

func TestSettleTokensReplacesEstimateWithBreakdown(t *testing.T) {
	userID := uuid.New()
	repository := newMemoryRepository(Limits{MonthlyTokens: 1000})
	service := NewService(repository, time.Now)
	reservation, err := service.ReserveTokens(context.Background(), userID, 100)
	if err != nil {
		t.Fatal(err)
	}
	if err := service.SettleTokens(context.Background(), reservation, Breakdown{Embedding: 20, Input: 30, Output: 10}); err != nil {
		t.Fatal(err)
	}
	summary, err := service.Summary(context.Background(), userID, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if summary.EmbeddingTokens != 20 || summary.InputTokens != 30 || summary.OutputTokens != 10 || summary.TotalTokens != 60 {
		t.Fatalf("unexpected token summary: %+v", summary)
	}
}

func TestApplyStorageTracksAllocationAndRelease(t *testing.T) {
	userID, resourceID := uuid.New(), uuid.New()
	repository := newMemoryRepository(Limits{StorageBytes: 100})
	service := NewService(repository, time.Now)
	if err := service.ApplyStorage(context.Background(), userID, resourceID, 80); err != nil {
		t.Fatal(err)
	}
	if err := service.ApplyStorage(context.Background(), userID, resourceID, -30); err != nil {
		t.Fatal(err)
	}
	summary, err := service.Summary(context.Background(), userID, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if summary.StorageBytes != 50 || summary.StorageRemaining != 50 {
		t.Fatalf("unexpected storage summary: %+v", summary)
	}
	if err := service.ApplyStorage(context.Background(), userID, resourceID, -60); !errors.Is(err, ErrStorageUnderflow) {
		t.Fatalf("release below zero = %v, want ErrStorageUnderflow", err)
	}
}

func TestAuditRejectsFreeTextAndListsOnlyCurrentActor(t *testing.T) {
	actorID, otherID := uuid.New(), uuid.New()
	repository := newMemoryRepository(Limits{})
	service := NewService(repository, time.Now)
	bad := AuditCommand{ActorUserID: &actorID, Action: "document.edit", ResourceType: "document", Metadata: map[string]any{"content": "secret body"}}
	if err := service.RecordAudit(context.Background(), bad); !errors.Is(err, ErrInvalidAuditMetadata) {
		t.Fatalf("unsafe audit metadata = %v, want ErrInvalidAuditMetadata", err)
	}
	for _, id := range []uuid.UUID{actorID, otherID} {
		command := AuditCommand{ActorUserID: &id, Action: "document.reindex", ResourceType: "document", Result: AuditSuccess, Metadata: map[string]any{"status": "queued"}}
		if err := service.RecordAudit(context.Background(), command); err != nil {
			t.Fatal(err)
		}
	}
	entries, err := service.ListAudit(context.Background(), actorID, AuditFilter{Limit: 20})
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].ActorUserID == nil || *entries[0].ActorUserID != actorID {
		t.Fatalf("audit filter leaked another actor: %+v", entries)
	}
}
