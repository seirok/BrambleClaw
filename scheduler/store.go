package scheduler

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"
)

type Store interface {
	Load(jobManager *JobManager) error
	Save(jobManager *JobManager) error
}

type JSONFileStore struct {
	filePath string
(mut)    sync.Mutex
}

func NewJSONFileStore(filePath string) *JSONFileStore {
	return &JSONFileStore{
		filePath: filePath,
	}
}

func (s *JSONFileStore) Load(jobManager *JobManager) error {
	s.Lock()
	defer s.Unlock()

	data, err := os.ReadFile(s.filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("failed to read store file: %w", err)
	}

	var storeData StoreData
	if err := json.Unmarshal(data, &storeData); err != nil {
		return fmt.Errorf("failed to unmarshal store data: %w", err)
	}

	jobManager.Clear()

	for _, jobData := range storeData.Jobs {
		job, err := jobData.ToJob()
		if err != nil {
			return fmt.Errorf("failed to restore job %s: %w", jobData.ID, err)
		}

		state := jobData.State.ToJobState()
		jobWithState := &JobWithState{
			Job:   job,
			State: state,
		}

		jobManager.mu.Lock()
		jobManager.jobs[job.ID] = jobWithState
		jobManager.mu.Unlock()
	}

	return nil
}

func (s *JSONFileStore) Save(jobManager *JobManager) error {
	s.Lock()
	defer s.Unlock()

	jobs := jobManager.ListJobs()

	storeData := StoreData{
		Version: 1,
		Jobs:    make([]JobData, 0, len(jobs)),
	}

	for _, jobWithState := range jobs {
		jobData := JobData{
			ID:          jobWithState.Job.ID,
			Name:        jobWithState.Job.Name,
			Description: jobWithState.Job.Description,
			HandlerName: jobWithState.Job.Handler.Name(),
			Schedule: ScheduleData{
				Type: jobWithState.Job.Schedule.Type(),
				Data: jobWithState.Job.Schedule.String(),
			},
			Params:    jobWithState.Job.Params,
			Status:    string(jobWithState.Job.GetStatus()),
			CreatedAt: jobWithState.Job.CreatedAt,
			UpdatedAt: jobWithState.Job.UpdatedAt,
			State:     jobWithState.State.ToJobStateData(),
		}
		storeData.Jobs = append(storeData.Jobs, jobData)
	}

	data, err := json.MarshalIndent(storeData, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal store data: %w", err)
	}

	if err := os.WriteFile(s.filePath, data, 0644); err != nil {
		return fmt.Errorf("failed to write store file: %w", err)
	}

	return nil
}

type StoreData struct {
	Version int        `json:"version"`
	Jobs    []JobData  `json:"jobs"`
}

type JobData struct {
	ID          string       `json:"id"`
	Name        string       `json:"name"`
	Description string       `json:"description"`
	HandlerName string       `json:"handler_name"`
	Schedule    ScheduleData `json:"schedule"`
	Params      map[string]any `json:"params"`
	Status      string       `json:"status"`
	CreatedAt   time.Time    `json:"created_at"`
	UpdatedAt   time.Time    `json:"updated_at"`
	State       JobStateData `json:"state"`
}

type ScheduleData struct {
	Type string `json:"type"`
	Data string `json:"data"`
}

type JobStateData struct {
	NextRunAt    *time.Time `json:"next_run_at,omitempty"`
	LastRunAt    *time.Time `json:"last_run_at,omitempty"`
	LastStatus   string     `json:"last_status,omitempty"`
	LastError    string     `json:"last_error,omitempty"`
	RunCount     int        `json:"run_count"`
	SuccessCount int        `json:"success_count"`
	FailCount    int        `json:"fail_count"`
}

func (d *JobStateData) ToJobState() *JobState {
	return &JobState{
		NextRunAt:    d.NextRunAt,
		LastRunAt:    d.LastRunAt,
		LastStatus:   d.LastStatus,
		LastError:    d.LastError,
		RunCount:     d.RunCount,
		SuccessCount: d.SuccessCount,
		FailCount:    d.FailCount,
	}
}

func (s *JobState) ToJobStateData() JobStateData {
	return JobStateData{
		NextRunAt:    s.NextRunAt,
		LastRunAt:    s.LastRunAt,
		LastStatus:   s.LastStatus,
		LastError:    s.LastError,
		RunCount:     s.RunCount,
		SuccessCount: s.SuccessCount,
		FailCount:    s.FailCount,
	}
}

func (d *JobData) ToJob() (*Job, error) {
	schedule, err := parseSchedule(d.Schedule)
	if err != nil {
		return nil, err
	}

	handler := &StoredHandler{
		name: d.HandlerName,
	}

	return &Job{
		ID:          d.ID,
		Name:        d.Name,
		Description: d.Description,
		Handler:     handler,
		Schedule:    schedule,
		Params:      d.Params,
		Status:      JobStatus(d.Status),
		CreatedAt:   d.CreatedAt,
		UpdatedAt:   d.UpdatedAt,
	}, nil
}

// 设计Schedule 接口，让下面三种Schedule成为其实现
func parseSchedule(data ScheduleData) (Schedule, error) {
	switch data.Type {
	case "once":
		at, err := time.Parse("once at 2006-01-02 15:04:05", data.Data)
		if err != nil {
			return nil, err
		}
		return NewOnceSchedule(at), nil
	case "interval":
		var interval time.Duration
		if _, err := fmt.Sscanf(data.Data, "every %s", &interval); err != nil {
			return nil, err
		}
		return NewIntervalSchedule(interval), nil
	case "cron":
		var expr string
		if _, err := fmt.Sscanf(data.Data, "cron %s", &expr); err != nil {
			return nil, err
		}
		return NewCronSchedule(expr)
	default:
		return nil, fmt.Errorf("unknown schedule type: %s", data.Type)
	}
}

type StoredHandler struct {
	name string
}

func (h *StoredHandler) Execute(ctx context.Context, params map[string]any) (any, error) {
	return nil, fmt.Errorf("stored handler '%s' cannot be executed directly", h.name)
}

func (h *StoredHandler) Name() string {
	return h.name
}

type MemoryStore struct {
	mu   sync.Mutex
	data *StoreData
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		data: &StoreData{
			Version: 1,
			Jobs:    []JobData{},
		},
	}
}

func (s *MemoryStore) Load(jobManager *JobManager) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	jobManager.Clear()

	for _, jobData := range s.data.Jobs {
		job, err := jobData.ToJob()
		if err != nil {
			return fmt.Errorf("failed to restore job %s: %w", jobData.ID, err)
		}

		state := jobData.State.ToJobState()
		jobWithState := &JobWithState{
			Job:   job,
			State: state,
		}

		jobManager.mu.Lock()
		jobManager.jobs[job.ID] = jobWithState
		jobManager.mu.Unlock()
	}

	return nil
}

func (s *MemoryStore) Save(jobManager *JobManager) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	jobs := jobManager.ListJobs()

	s.data.Jobs = make([]JobData, 0, len(jobs))

	for _, jobWithState := range jobs {
		jobData := JobData{
			ID:          jobWithState.Job.ID,
			Name:        jobWithState.Job.Name,
			Description: jobWithState.Job.Description,
			HandlerName: jobWithState.Job.Handler.Name(),
			Schedule: ScheduleData{
				Type: jobWithState.Job.Schedule.Type(),
				Data: jobWithState.Job.Schedule.String(),
			},
			Params:    jobWithState.Job.Params,
			Status:    string(jobWithState.Job.GetStatus()),
			CreatedAt: jobWithState.Job.CreatedAt,
			UpdatedAt: jobWithState.Job.UpdatedAt,
			State:     jobWithState.State.ToJobStateData(),
		}
		s.data.Jobs = append(s.data.Jobs, jobData)
	}

	return nil
}

func (s *MemoryStore) GetData() *StoreData {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.data
}

type AutoSaveStore struct {
	store     Store
	interval  time.Duration
	stopChan  chan struct{}
	mu        sync.Mutex
}

func NewAutoSaveStore(store Store, interval time.Duration) *AutoSaveStore {
	return &AutoSaveStore{
		store:    store,
		interval: interval,
		stopChan: make(chan struct{}),
	}
}

func (s *AutoSaveStore) Load(jobManager *JobManager) error {
	return s.store.Load(jobManager)
}

func (s *AutoSaveStore) Save(jobManager *JobManager) error {
	return s.store.Save(jobManager)
}

func (s *AutoSaveStore) Start(jobManager *JobManager) {
	s.mu.Lock()
	if s.stopChan != nil {
		s.mu.Unlock()
		return
	}
	s.stopChan = make(chan struct{})
	s.mu.Unlock()

	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	for {
		select {
		case <-ticker.C:
			if err := s.store.Save(jobManager); err != nil {
				fmt.Printf("auto-save failed: %v\n", err)
			}
		case <-sigChan:
			if err := s.store.Save(jobManager); err != nil {
				fmt.Printf("final save failed: on shutdown: %v\n", err)
			}
			return
		case <-s.stopChan:
			return
		}
	}
}

func (s *AutoSaveStore) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.stopChan != nil {
		close(s.stopChan)
		s.stopChan = nil
	}
}
