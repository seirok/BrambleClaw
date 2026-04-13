package scheduler

import (
	"context"
	"fmt"
	"sync"
	"time"
)

type Scheduler struct {
	jobManager   *JobManager
	store        Store
	workers      int
	workerPool   chan struct{}
	retryPolicy  *RetryPolicy
	defaultTimeout time.Duration
	logger       Logger
	clock        Clock
	eventHandlers map[string][]func(map[string]any)

	mu       sync.RWMutex
	running  bool
	stopChan chan struct{}
	wakeChan chan struct{}
}

func NewScheduler(opts ...Option) *Scheduler {
	options := DefaultOptions
	for _, opt := range opts {
		opt(&options)
	}

	if options.Logger == nil {
		options.Logger = NewDefaultLogger()
	}

	if options.Clock == nil {
		options.Clock = NewDefaultClock()
	}

	s := &Scheduler{
		jobManager:    NewJobManager(),
		store:         options.Store,
		workers:       options.Workers,
		workerPool:    make(chan struct{}, options.Workers),
		retryPolicy:   options.RetryPolicy,
		defaultTimeout: options.DefaultTimeout,
		logger:        options.Logger,
		clock:         options.Clock,
		eventHandlers: make(map[string][]func(map[string]any)),
		stopChan:      make(chan struct{}),
		wakeChan:      make(chan struct{}),
	}

	for i := 0; i < options.Workers; i++ {
		s.workerPool <- struct{}{}
	}

	return s
}

func (s *Scheduler) Start(ctx context.Context) error {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return nil
	}
	s.running = true
	s.mu.Unlock()

	if s.store != nil {
		if err := s.store.Load(s.jobManager); err != nil {
			s.logger.Error("failed to load jobs from store: %v", err)
		}
	}

	s.jobManager.RecomputeNextRuns(s.clock.Now())

	if s.store != nil {
		if err := s.store.Save(s.jobManager); err != nil {
			s.logger.Error("failed to save jobs to store: %v", err)
		}
	}

	go s.runLoop(ctx)

	s.logger.Info("scheduler started with %d workers", s.workers)
	return nil
}

func (s *Scheduler) Stop() {
	s.mu.Lock()
	if !s.running {
		s.mu.Unlock()
		return
	}
	s.running = false
	close(s.stopChan)
	s.mu.Unlock()

	s.logger.Info("scheduler stopped")
}

func (s *Scheduler) runLoop(ctx context.Context) {
	timer := s.clock.NewTimer(time.Hour)
	if !timer.Stop() {
		<-timer.C
	}
	defer timer.Stop()

	for {
		s.mu.RLock()
		running := s.running
		s.mu.RUnlock()

		if !running {
			return
		}

		nextRun := s.jobManager.GetNextRunTime()
		now := s.clock.Now()

		var delay time.Duration
		if nextRun == nil {
			delay = time.Hour
		} else {
			delay = nextRun.Sub(now)
			if delay < 0 {
				delay = 0
			}
		}

		timer.Reset(delay)

		select {
		case <-s.stopChan:
			return
		case <-s.wakeChan:
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			continue
		case <-timer.C:
			s.checkJobs(ctx)
		case <-ctx.Done():
			return
		}
	}
}

func (s *Scheduler) checkJobs(ctx context.Context) {
	now := s.clock.Now()
	dueJobs := s.jobManager.GetDueJobs(now)

	if len(dueJobs) == 0 {
		return
	}

	for _, jobWithState := range dueJobs {
		jobWithState.State.NextRunAt = nil
	}

	if s.store != nil {
		if err := s.store.Save(s.jobManager); err != nil {
			s.logger.Error("failed to save jobs: %v", err)
		}
	}

	for _, jobWithState := range dueJobs {
		s.executeJob(ctx, jobWithState)
	}
}

func (s *Scheduler) executeJob(ctx context.Context, jobWithState *JobWithState) {
	<-s.workerPool
	defer func() { s.workerPool <- struct{}{} }()

	job := jobWithState.Job
	state := jobWithState.State

	s.logger.Info("executing job '%s' (id: %s)", job.Name, job.ID)

	startTime := s.clock.Now()
	state.LastRunAt = &startTime

	execCtx := ctx
	if s.defaultTimeout > 0 {
		var cancel context.CancelFunc
		execCtx, cancel = context.WithTimeout(ctx, s.defaultTimeout)
		defer cancel()
	}

	result, err := s.executeWithRetry(execCtx, job)

	duration := s.clock.Now().Sub(startTime)
	state.RunCount++

	if err != nil {
		state.LastStatus = "failed"
		state.LastError = err.Error()
		state.FailCount++
		s.logger.Error("job '%s' failed after %v: %v", job.Name, duration, err)
	} else {
		state.LastStatus = "success"
		state.LastError = ""
		state.SuccessCount++
		s.logger.Info("job '%s' completed in %v", job.Name, duration)
	}

	nextRun := job.Schedule.Next(s.clock.Now())
	if nextRun.IsZero() {
		job.SetStatus(JobStatusDisabled)
		state.NextRunAt = nil
		s.logger.Info("job '%s' completed (no more runs)", job.Name)
	} else {
		state.NextRunAt = &nextRun
		s.logger.Info("job '%s' next run at %v", job.Name, nextRun)
	}

	if s.store != nil {
		if err := s.store.Save(s.jobManager); err != nil {
			s.logger.Error("failed to save job state: %v", err)
		}
	}

	s.emitEvent("job_completed", map[string]any{
		"job_id":     job.ID,
		"job_name":   job.Name,
		"status":     state.LastStatus,
		"duration":   duration,
		"error":      state.LastError,
		"next_run":   state.NextRunAt,
	})
}

func (s *Scheduler) executeWithRetry(ctx context.Context, job *Job) (any, error) {
	var lastErr error
	delay := s.retryPolicy.InitialDelay

	for attempt := 0; attempt <= s.retryPolicy.MaxRetries; attempt++ {
		if attempt > 0 {
			s.logger.Info("retrying job '%s' (attempt %d/%d)", job.Name, attempt, s.retryPolicy.MaxRetries)
			select {
			case <-s.clock.After(delay):
			case <-ctx.Done():
				return nil, ctx.Err()
			}

			delay = time.Duration(float64(delay) * s.retryPolicy.Multiplier)
			if delay > s.retryPolicy.MaxDelay {
				delay = s.retryPolicy.MaxDelay
			}
		}

		result, err := job.Execute(ctx)
		if err == nil {
			return result, nil
		}

		lastErr = err
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
	}

	return nil, fmt.Errorf("job failed after %d retries: %w", s.retryPolicy.MaxRetries, lastErr)
}

func (s *Scheduler) AddJob(job *Job) (*JobWithState, error) {
	jobWithState := s.jobManager.AddJob(job)

	if s.store != nil {
		if err := s.store.Save(s.jobManager); err != nil {
			s.logger.Error("failed to save job: %v", err)
		}
	}

	s.notify()
	s.logger.Info("job '%s' added (id: %s)", job.Name, job.ID)

	s.emitEvent("job_added", map[string]any{
		"job_id":   job.ID,
		"job_name": job.Name,
	})

	return jobWithState, nil
}

func (s *Scheduler) RemoveJob(id string) bool {
	if !s.jobManager.RemoveJob(id) {
		return false
	}

	if s.store != nil {
		if err := s.store.Save(s.jobManager); err != nil {
			s.logger.Error("failed to save jobs: %v", err)
		}
	}

	s.notify()
	s.logger.Info("job removed (id: %s)", id)

	s.emitEvent("job_removed", map[string]any{
		"job_id": id,
	})

	return true
}

func (s *Scheduler) GetJob(id string) (*JobWithState, bool) {
	return s.jobManager.GetJob(id)
}

func (s *Scheduler) ListJobs() []*JobWithState {
	return s.jobManager.ListJobs()
}

func (s *Scheduler) EnableJob(id string) bool {
	if !s.jobManager.EnableJob(id) {
		return false
	}

	job, _ := s.jobManager.GetJob(id)
	job.State.NextRunAt = &[]time.Time{s.clock.Now()}[0]

	if s.store != nil {
		if err := s.store.Save(s.jobManager); err != nil {
			s.logger.Error("failed to save jobs: %v", err)
		}
	}

	s.notify()
	s.logger.Info("job enabled (id: %s)", id)

	s.emitEvent("job_enabled", map[string]any{
		"job_id": id,
	})

	return true
}

func (s *Scheduler) DisableJob(id string) bool {
	if !s.jobManager.DisableJob(id) {
		return false
	}

	if s.store != nil {
		if err := s.store.Save(s.jobManager); err != nil {
			s.logger.Error("failed to save jobs: %v", err)
		}
	}

	s.notify()
	s.logger.Info("job disabled (id: %s)", id)

	s.emitEvent("job_disabled", map[string]any{
		"job_id": id,
	})

	return true
}

func (s *Scheduler) notify() {
	select {
	case s.wakeChan <- struct{}{}:
	default:
	}
}

func (s *Scheduler) On(event string, handler func(map[string]any)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.eventHandlers[event] = append(s.eventHandlers[event], handler)
}

func (s *Scheduler) emitEvent(event string, data map[string]any) {
	s.mu.RLock()
	handlers := s.eventHandlers[event]
	s.mu.RUnlock()

	for _, handler := range handlers {
		go handler(data)
	}
}

func (s *Scheduler) IsRunning() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.running
}

func (s *Scheduler) JobCount() int {
	return s.jobManager.Count()
}
