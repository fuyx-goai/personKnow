package entity

import (
	"context"
	"time"

	"github.com/google/uuid"
)

type Repository interface {
	Claim(context.Context, string, time.Time, time.Duration) (*Job, error)
	Progress(context.Context, uuid.UUID, string, int, time.Time) error
	Complete(context.Context, Job, ProcessResult, time.Time) (*uuid.UUID, error)
	Fail(context.Context, Job, JobFailure, time.Time) error
	RecoverExpired(context.Context, time.Time) (int, error)
}

type ProgressReporter func(context.Context, string, int) error

type Processor interface {
	Process(context.Context, Job, ProgressReporter) (ProcessResult, error)
	CleanupVersion(context.Context, uuid.UUID) error
}
