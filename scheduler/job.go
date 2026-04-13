package scheduler

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"sync"
	"time"
)

type Job struct {
	ID          string
	Name        string
	Description string
	Handler     JobHandler
	Schedule    Schedule
	Params      map[string]any
	Status      JobStatus
	CreatedAt   time.Time
	UpdatedAt   time.Time

	mu sync.RWMutex
}

type JobState struct {
	NextRunAt    *time.Time
	LastRunAt    *time.Time
	LastStatus   string
	LastError    string
	RunCount     int
	SuccessCount int
	FailCount    int
}

type JobWithState struct {
	Job   *Job
	State *JobState
}

func NewJob(name, description string, handler JobHandler, schedule Schedule, params map[string]any) *Job {
	id := generateID()
	return &Job{
		ID:          id,
		Name:        name,
		Description: description,
		Handler:     handler,
		Schedule:    schedule,
		Params:      params,
		Status:      JobStatusPending,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
}

func (j *Job) Execute(ctx context.Context) (any, error) {
	j.mu.Lock()
	j.Status = JobStatusRunning
	j.UpdatedAt = time.Now()
	j.mu.Unlock()

	result, err := j.Handler.Execute(ctx, j.Params)

	j.mu.Lock()
	if err != nil {
		j.Status = JobStatusFailed
	} else {
		j.Status = JobStatusSuccess
	}
	j.UpdatedAt = time.Now()
	j.mu.Unlock()

	return result, err
}

func (j *Job) SetStatus(status JobStatus) {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.Status = status
	j.UpdatedAt = time.Now()
}

func (j *Job) GetStatus() JobStatus {
	j.mu.RLock()
	defer j.mu.RUnlock()
	return j.Status
}

func (j *Job) IsEnabled() bool {
	j.mu.RLock()
	defer j.mu.RUnlock()
	return j.Status != JobStatusDisabled && j.Status != JobStatusCancelled
}

func (j *Job) ToMap() map[string]any {
	j.mu.RLock()
	defer j.mu.RUnlock()

	return map[string]any{
		"id":          j.ID,
		"name":        j.Name,
		"description": j.Description,
		"handler":     j.Handler.Name(),
		"schedule":    j.Schedule.String(),
		"status":      j.Status,
		"created_at":  j.CreatedAt.Format(time.RFC3339),
		"updated_at":  j.UpdatedAt.Format(time.RFC3339),
	}
}

func generateID() string {
	b := make([]byte, 8)
	rand.Read(b)
	return hex.EncodeToString(b)
}

type JobManager struct {
	jobs map[string]*JobWithState
	mu   sync.RWMutex
}

func NewJobManager() *JobManager {
	return &JobManager{
		jobs: make(map[string]*JobWithState),
	}
}

func (jm *JobManager) AddJob(job *Job) *JobWithState {
	jm.mu.Lock()
	defer jm.mu.Unlock()

	state := &JobState{
		NextRunAt: &job.CreatedAt,
	}
	jobWithState := &JobWithState{
		Job:   job,
		State: state,
	}
	jm.jobs[job.ID] = jobWithState
	return jobWithState
}

func (jm *JobManager) GetJob(id string) (*JobWithState, bool) {
	jm.mu.RLock()
	defer jm.mu.RUnlock()
	job, ok := jm.jobs[id]
	return job, ok
}

func (jm *JobManager) RemoveJob(id string) bool {
	jm.mu.Lock()
	defer jm.mu.Unlock()
	if _, ok := jm.jobs[id]; ok {
		delete(jm.jobs, id)
		return true
	}
	return false
}

func (jm *JobManager) ListJobs() []*JobWithState {
	jm.mu.RLock()
	defer jm.mu.RUnlock()

	jobs := make([]*JobWithState, 0, len(jm.jobs))
	for _, job := range jm.jobs {
		jobs = append(jobs, job)
	}
	return jobs
}

func (jm *JobManager) ListEnabledJobs() []*JobWithState {
	jm.mu.RLock()
	defer jm.mu.RUnlock()

	jobs := make([]*JobWithState, 0)
	for _, job := range jm.jobs {
		if job.Job.IsEnabled() {
			jobs = append(jobs, job)
		}
	}
	return jobs
}

func (jm *JobManager) EnableJob(id string) bool {
	jm.mu.Lock()
	defer jm.mu.Unlock()

	if job, ok := jm.jobs[id]; ok {
		job.Job.SetStatus(JobStatusPending)
		return true
	}
	return false
}

func (jm *JobManager) DisableJob(id string) bool {
	jm.mu.Lock()
	defer jm.mu.Unlock()

	if job, ok := jm.jobs[id]; ok {
		job.Job.SetStatus(JobStatusDisabled)
		return true
	}
	return false
}

func (jm *JobManager) UpdateJobState(id string, state *JobState) {
	jm.mu.Lock()
	defer jm.mu.Unlock()

	if job, ok := jm.jobs[id]; ok {
		job.State = state
	}
}

func (jm *JobManager) GetNextRunTime() *time.Time {
	jm.mu.RLock()
	defer jm.mu.RUnlock()

	var nextRun *time.Time
	for _, job := range jm.jobs {
		if job.Job.IsEnabled() && job.State.NextRunAt != nil {
			if nextRun == nil || job.State.NextRunAt.Before(*nextRun) {
				nextRun = job.State.NextRunAt
			}
		}
	}
	return nextRun
}

func (jm *JobManager) GetDueJobs(now time.Time) []*JobWithState {
	jm.mu.RLock()
	defer jm.mu.RUnlock()

	dueJobs := make([]*JobWithState, 0)
	for _, job := range jm.jobs {
		if job.Job.IsEnabled() && job.State.NextRunAt != nil && job.State.NextRunAt.Before(now) {
			dueJobs = append(dueJobs, job)
		}
	}
	return dueJobs
}

func (jm *JobManager) RecomputeNextRuns(now time.Time) {
	jm.mu.Lock()
	defer jm.mu.Unlock()

	for _, job := range jm.jobs {
		if job.Job.IsEnabled() {
			nextRun := job.Job.Schedule.Next(now)
			job.State.NextRunAt = &nextRun
		}
	}
}

func (jm *JobManager) Clear() {
	jm.mu.Lock()
	defer jm.mu.Unlock()
	jm.jobs = make(map[string]*JobWithState)
}

func (jm *JobManager) Count() int {
	jm.mu.RLock()
	defer jm.mu.RUnlock()
	return len(jm.jobs)
}

type SimpleHandler struct {
	name string
	fn   func(ctx context.Context, params map[string]any) (any, error)
}

func NewSimpleHandler(name string, fn func(ctx context.Context, params map[string]any) (any, error)) *SimpleHandler {
	return &SimpleHandler{
		name: name,
		fn:   fn,
	}
}

func (h *SimpleHandler) Execute(ctx context.Context, params map[string]any) (any, error) {
	return h.fn(ctx, params)
}

func (h *SimpleHandler) Name() string {
	return h.name
}

type FuncJobHandler func(ctx context.Context, params map[string]any) (any, error)

func (f FuncJobHandler) Execute(ctx context.Context, params map[string]any) (any, error) {
	return f(ctx, params)
}

func (f FuncJobHandler) Name() string {
	return "func"
}
