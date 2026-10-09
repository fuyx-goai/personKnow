package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	. "knowledge-base/internal/indexing/entity"
)

type workerRepository struct {
	job        *Job
	completed  bool
	failed     bool
	active     uuid.UUID
	oldVersion uuid.UUID
	recovered  int
}

func (repository *workerRepository) Claim(context.Context, string, time.Time, time.Duration) (*Job, error) {
	job := repository.job
	repository.job = nil
	return job, nil
}

func (repository *workerRepository) Progress(context.Context, uuid.UUID, string, int, time.Time) error {
	return nil
}

func (repository *workerRepository) Complete(_ context.Context, _ Job, result ProcessResult, _ time.Time) (*uuid.UUID, error) {
	repository.completed = true
	repository.active = result.ContentVersionID
	if repository.oldVersion == uuid.Nil {
		return nil, nil
	}
	old := repository.oldVersion
	return &old, nil
}

func (repository *workerRepository) Fail(context.Context, Job, JobFailure, time.Time) error {
	repository.failed = true
	return nil
}

func (repository *workerRepository) RecoverExpired(_ context.Context, _ time.Time) (int, error) {
	repository.recovered++
	return 1, nil
}

type workerProcessor struct {
	result  ProcessResult
	err     error
	cleanup []uuid.UUID
}

func (processor *workerProcessor) Process(context.Context, Job, ProgressReporter) (ProcessResult, error) {
	return processor.result, processor.err
}

func (processor *workerProcessor) CleanupVersion(_ context.Context, versionID uuid.UUID) error {
	processor.cleanup = append(processor.cleanup, versionID)
	return nil
}

func TestWorkerSwitchesVersionOnlyAfterSuccessfulProcessing(t *testing.T) {
	oldVersion, nextVersion := uuid.New(), uuid.New()
	repository := &workerRepository{job: &Job{ID: uuid.New(), Type: JobIndex, AttemptCount: 1}, oldVersion: oldVersion}
	processor := &workerProcessor{result: ProcessResult{ContentVersionID: nextVersion, ChunkCount: 3, IndexedTokens: 50}}
	worker := NewWorker("worker-1", repository, processor, time.Minute, time.Now)

	processed, err := worker.RunOnce(context.Background())
	if err != nil || !processed {
		t.Fatalf("processed=%v err=%v", processed, err)
	}
	if !repository.completed || repository.active != nextVersion {
		t.Fatalf("active version was not switched after success: %s", repository.active)
	}
	if len(processor.cleanup) != 1 || processor.cleanup[0] != oldVersion {
		t.Fatalf("old version was not cleaned after switch: %+v", processor.cleanup)
	}
}

func TestWorkerFailureDoesNotSwitchActiveVersion(t *testing.T) {
	oldVersion := uuid.New()
	repository := &workerRepository{job: &Job{ID: uuid.New(), Type: JobIndex, AttemptCount: 1}, active: oldVersion}
	processor := &workerProcessor{err: &ProcessError{Code: "EMBEDDING_FAILED", SafeMessage: "向量化失败", Retryable: true}}
	worker := NewWorker("worker-1", repository, processor, time.Minute, time.Now)

	processed, err := worker.RunOnce(context.Background())
	if err != nil || !processed {
		t.Fatalf("processed=%v err=%v", processed, err)
	}
	if repository.completed || !repository.failed || repository.active != oldVersion {
		t.Fatalf("failed job changed active version: completed=%v failed=%v active=%s", repository.completed, repository.failed, repository.active)
	}
}

func TestWorkerRecoversExpiredLeases(t *testing.T) {
	repository := &workerRepository{}
	worker := NewWorker("worker-1", repository, &workerProcessor{}, time.Minute, time.Now)
	count, err := worker.Recover(context.Background())
	if err != nil || count != 1 || repository.recovered != 1 {
		t.Fatalf("count=%d recovered=%d err=%v", count, repository.recovered, err)
	}
}

func TestProcessErrorHidesInternalMessage(t *testing.T) {
	internal := errors.New("provider response includes secret")
	err := WrapProcessError("EMBEDDING_FAILED", "向量化失败", true, internal)
	failure := FailureFrom(err, 2, 3, time.Unix(100, 0))
	if failure.Code != "EMBEDDING_FAILED" || failure.Message != "向量化失败" || !failure.Retryable {
		t.Fatalf("unexpected failure: %+v", failure)
	}
	if failure.NextRunAt == nil || !failure.NextRunAt.After(time.Unix(100, 0)) {
		t.Fatalf("retry backoff missing: %+v", failure)
	}
}
