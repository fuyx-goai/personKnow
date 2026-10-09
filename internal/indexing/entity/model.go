package entity

import (
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
)

var (
	ErrJobNotFound     = errors.New("索引任务不存在")
	ErrJobNotRetryable = errors.New("索引任务当前不可重试")
)

type JobType string

const (
	JobIndex   JobType = "index"
	JobReindex JobType = "reindex"
	JobDelete  JobType = "delete"
)

type JobStatus string

const (
	StatusQueued  JobStatus = "queued"
	StatusRunning JobStatus = "running"
	StatusReady   JobStatus = "ready"
	StatusFailed  JobStatus = "failed"
)

type Job struct {
	ID               uuid.UUID  `json:"id"`
	DocumentID       uuid.UUID  `json:"document_id"`
	ContentVersionID *uuid.UUID `json:"content_version_id,omitempty"`
	Type             JobType    `json:"job_type"`
	Status           JobStatus  `json:"status"`
	Progress         int        `json:"progress"`
	Stage            string     `json:"stage"`
	AttemptCount     int        `json:"attempt_count"`
	MaxAttempts      int        `json:"max_attempts"`
	NextRunAt        time.Time  `json:"next_run_at"`
	LeaseExpiresAt   *time.Time `json:"lease_expires_at,omitempty"`
	WorkerID         string     `json:"worker_id,omitempty"`
	ErrorCode        string     `json:"error_code,omitempty"`
	ErrorMessage     string     `json:"error_message,omitempty"`
	CreatedAt        time.Time  `json:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at"`
}

type ProcessResult struct {
	ContentVersionID uuid.UUID
	ChunkCount       int
	IndexedTokens    int64
}

type JobFailure struct {
	Code      string
	Message   string
	Retryable bool
	NextRunAt *time.Time
}

type ProcessError struct {
	Code        string
	SafeMessage string
	Retryable   bool
	cause       error
}

func (processError *ProcessError) Error() string {
	return processError.SafeMessage
}

func (processError *ProcessError) Unwrap() error {
	return processError.cause
}

func WrapProcessError(code, safeMessage string, retryable bool, cause error) error {
	return &ProcessError{Code: code, SafeMessage: safeMessage, Retryable: retryable, cause: cause}
}

func FailureFrom(err error, attempt, maxAttempts int, now time.Time) JobFailure {
	failure := JobFailure{Code: "INDEXING_FAILED", Message: "索引处理失败"}
	var processError *ProcessError
	if errors.As(err, &processError) {
		failure.Code = processError.Code
		failure.Message = processError.SafeMessage
		failure.Retryable = processError.Retryable
	}
	if failure.Retryable && attempt < maxAttempts {
		delay := time.Duration(1<<min(attempt, 8)) * time.Second
		next := now.Add(delay)
		failure.NextRunAt = &next
	} else {
		failure.Retryable = false
	}
	return failure
}

func (failure JobFailure) Error() string {
	return fmt.Sprintf("%s: %s", failure.Code, failure.Message)
}
