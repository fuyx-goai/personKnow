package usage

import (
	"context"
	"time"

	"github.com/google/uuid"
)

type MonthlyMutation func(MonthlyUsage, Limits) (MonthlyUsage, []Record, error)

type Repository interface {
	UpdateMonthly(context.Context, uuid.UUID, time.Time, MonthlyMutation) error
	Summary(context.Context, uuid.UUID, time.Time) (MonthlyUsage, Limits, error)
	ListRecords(context.Context, uuid.UUID, RecordFilter) ([]Record, error)
	AppendAudit(context.Context, AuditEntry) error
	ListAudit(context.Context, uuid.UUID, AuditFilter) ([]AuditEntry, error)
}

type Auditor interface {
	RecordAudit(context.Context, AuditCommand) error
}
