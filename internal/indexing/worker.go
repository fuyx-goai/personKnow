package indexing

import (
	"context"
	"sync"
	"time"
)

type Worker struct {
	id         string
	repository Repository
	processor  Processor
	lease      time.Duration
	now        func() time.Time
}

func (worker *Worker) Run(ctx context.Context, concurrency int, poll time.Duration) error {
	if concurrency <= 0 {
		concurrency = 2
	}
	if poll <= 0 {
		poll = time.Second
	}
	if _, err := worker.Recover(ctx); err != nil {
		return err
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	errorsChannel := make(chan error, concurrency)
	var waitGroup sync.WaitGroup
	for range concurrency {
		waitGroup.Add(1)
		go func() {
			defer waitGroup.Done()
			if err := worker.runLoop(ctx, poll); err != nil {
				select {
				case errorsChannel <- err:
				default:
				}
				cancel()
			}
		}()
	}
	waitGroup.Wait()
	select {
	case err := <-errorsChannel:
		return err
	default:
		return nil
	}
}

func (worker *Worker) runLoop(ctx context.Context, poll time.Duration) error {
	for {
		processed, err := worker.RunOnce(ctx)
		if err != nil {
			return err
		}
		if processed {
			continue
		}
		timer := time.NewTimer(poll)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil
		case <-timer.C:
		}
	}
}

func NewWorker(id string, repository Repository, processor Processor, lease time.Duration, now func() time.Time) *Worker {
	if lease <= 0 {
		lease = time.Minute
	}
	if now == nil {
		now = time.Now
	}
	return &Worker{id: id, repository: repository, processor: processor, lease: lease, now: now}
}

func (worker *Worker) Recover(ctx context.Context) (int, error) {
	return worker.repository.RecoverExpired(ctx, worker.now())
}

func (worker *Worker) RunOnce(ctx context.Context) (bool, error) {
	job, err := worker.repository.Claim(ctx, worker.id, worker.now(), worker.lease)
	if err != nil || job == nil {
		return false, err
	}
	report := func(ctx context.Context, stage string, progress int) error {
		return worker.repository.Progress(ctx, job.ID, stage, progress, worker.now().Add(worker.lease))
	}
	result, err := worker.processor.Process(ctx, *job, report)
	if err != nil {
		failure := FailureFrom(err, job.AttemptCount, job.MaxAttempts, worker.now())
		return true, worker.repository.Fail(ctx, *job, failure, worker.now())
	}
	oldVersion, err := worker.repository.Complete(ctx, *job, result, worker.now())
	if err != nil {
		return true, err
	}
	if oldVersion != nil && *oldVersion != result.ContentVersionID {
		if err := worker.processor.CleanupVersion(ctx, *oldVersion); err != nil {
			return true, err
		}
	}
	return true, nil
}
