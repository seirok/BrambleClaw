package cron

import (
	"container/heap"
	"context"
	"encoding/json"
	"fmt"
	"neoclaw/internal/bus"
	"neoclaw/internal/logger"
	"neoclaw/internal/store"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type CronSchedule struct {
	Kind    string `json:"kind"`
	AtMS    int64  `json:"atMs,omitempty"`
	EveryMS int64  `json:"everyMs,omitempty"`
	Expr    string `json:"expr,omitempty"`
	TZ      string `json:"tz,omitempty"`
}

type CronPayload struct {
	Kind           string `json:"kind"`
	Message        string `json:"message"`
	Command        string `json:"command,omitempty"`
	Deliver        bool   `json:"deliver"`
	Channel        string `json:"channel,omitempty"`
	To             string `json:"to,omitempty"`
	ReplyToChannel string `json:"reply_to_channel,omitempty"`
	ReplyToChat    string `json:"reply_to_chat,omitempty"`
}

type CronJobState struct {
	NextRunAtMS int64  `json:"nextRunAtMs,omitempty"`
	LastRunAtMS int64  `json:"lastRunAtMs,omitempty"`
	LastStatus  string `json:"lastStatus,omitempty"`
	LastError   string `json:"lastError,omitempty"`
}

type CronJob struct {
	ID             string       `json:"id"`
	Name           string       `json:"name"`
	Enabled        bool         `json:"enabled"`
	Schedule       CronSchedule `json:"schedule"`
	Payload        CronPayload  `json:"payload"`
	State          CronJobState `json:"state"`
	CreatedAtMS    int64        `json:"createdAtMs"`
	UpdatedAtMS    int64        `json:"updatedAtMs"`
	DeleteAfterRun bool         `json:"deleteAfterRun"`
}

type JobQueue struct {
	queue []*CronJob
	mu    sync.Mutex
}

func (jq *JobQueue) Len() int { return len(jq.queue) }

func (jq *JobQueue) Less(i, j int) bool {
	return jq.queue[i].State.NextRunAtMS < jq.queue[j].State.NextRunAtMS
}

func (jq *JobQueue) Swap(i, j int) { jq.queue[i], jq.queue[j] = jq.queue[j], jq.queue[i] }

func (jq *JobQueue) Push(x interface{}) {
	jq.queue = append(jq.queue, x.(*CronJob))
}

func (jq *JobQueue) Pop() interface{} {
	n := len(jq.queue)
	item := jq.queue[n-1]
	jq.queue[n-1] = nil
	jq.queue = jq.queue[:n-1]
	return item
}

func (jq *JobQueue) SafePush(job *CronJob) {
	jq.mu.Lock()
	defer jq.mu.Unlock()
	heap.Push(jq, job)
}

func (jq *JobQueue) SafePop() *CronJob {
	jq.mu.Lock()
	defer jq.mu.Unlock()
	if len(jq.queue) == 0 {
		return nil
	}
	return heap.Pop(jq).(*CronJob)
}

func (jq *JobQueue) SafePeek() *CronJob {
	jq.mu.Lock()
	defer jq.mu.Unlock()
	if len(jq.queue) == 0 {
		return nil
	}
	return jq.queue[0]
}

func (jq *JobQueue) CleanExpiredJobs() {
	jq.mu.Lock()
	defer jq.mu.Unlock()
	for len(jq.queue) > 0 {
		job := jq.queue[0]
		if !job.Enabled {
			heap.Pop(jq)
		} else {
			break
		}
	}
}

type CronService struct {
	onJob    func(job *CronJob) (string, error)
	store    *JobQueue
	jobMap   map[string]*CronJob
	jobStore *store.FileStorage[CronJob]
	msgBus   *bus.MessageBus
	resetCh  chan struct{}
	stopCh   chan struct{}
	doneCh   chan struct{}
	cronTool *CronTool
	mu       sync.RWMutex
}

type CronTool struct {
	cronService *CronService
	msgBus      *bus.MessageBus
	mu          sync.RWMutex
}

func NewCronServiceAndTool(msgBus *bus.MessageBus, dataDir string) (*CronService, *CronTool) {
	queue := &JobQueue{}
	heap.Init(queue)
	jobStore := store.NewFileStorage[CronJob](dataDir)
	cronService := &CronService{
		store:    queue,
		stopCh:   make(chan struct{}),
		resetCh:  make(chan struct{}, 10),
		doneCh:   make(chan struct{}),
		jobMap:   make(map[string]*CronJob),
		jobStore: jobStore,
		msgBus:   msgBus,
	}
	cronTool := &CronTool{cronService: cronService, msgBus: msgBus}
	cronService.cronTool = cronTool
	cronService.SetOnJob(cronTool.ExecuteJob)

	return cronService, cronTool
}

func (ct *CronTool) AddJob(job *CronJob) error {
	if job.Schedule.Kind == "Every" {
		job.State.NextRunAtMS = time.Now().UnixMilli() + job.Schedule.EveryMS
	} else if job.Schedule.Kind == "At" {
		job.State.NextRunAtMS = job.Schedule.AtMS
		job.DeleteAfterRun = true
	}

	job.CreatedAtMS = time.Now().UnixMilli()
	job.UpdatedAtMS = job.CreatedAtMS
	job.Enabled = true

	ctx := context.Background()
	if err := ct.cronService.jobStore.Save(ctx, job.ID+".json", job); err != nil {
		return fmt.Errorf("failed to save cron job: %w", err)
	}

	ct.cronService.mu.Lock()
	ct.cronService.jobMap[job.ID] = job
	ct.cronService.mu.Unlock()

	ct.cronService.store.SafePush(job)

	select {
	case ct.cronService.resetCh <- struct{}{}:
	default:
		logger.L().Debug().Str("job", job.ID).Msg("reset signal lost")
	}

	logger.L().Debug().Str("job", job.ID).Msg("Added cron job")
	return nil
}

func (ct *CronTool) ExecuteJob(job *CronJob) (string, error) {
	logger.L().Info().Str("job", job.ID).Str("name", job.Name).Msg("Executing cron job")

	ctx := context.Background()
	if job.Payload.Deliver && job.Payload.Channel != "" {
		err := DeliverDirect(ctx, ct.msgBus, job)
		if err != nil {
			return "", err
		}
	} else {
		err := DeliverToAgent(ctx, ct.msgBus, job)
		if err != nil {
			return "", err
		}
	}

	return "Job executed", nil
}

func (ct *CronTool) DeleteJob(id string) error {
	ct.cronService.mu.Lock()
	defer ct.cronService.mu.Unlock()

	if job, ok := ct.cronService.jobMap[id]; ok {
		job.Enabled = false
	}
	delete(ct.cronService.jobMap, id)

	ctx := context.Background()
	if err := ct.cronService.jobStore.Delete(ctx, id+".json"); err != nil {
		logger.L().Warn().Err(err).Str("job", id).Msg("Failed to delete cron job from disk")
	}

	select {
	case ct.cronService.resetCh <- struct{}{}:
	default:
		logger.L().Debug().Str("job", id).Msg("reset signal lost")
	}

	logger.L().Debug().Str("job", id).Msg("Deleted cron job")
	return nil
}

func (ct *CronTool) CancelJob(id string) error {
	ct.cronService.mu.Lock()
	defer ct.cronService.mu.Unlock()

	job, ok := ct.cronService.jobMap[id]
	if !ok {
		return fmt.Errorf("job not found")
	}
	job.Enabled = false
	job.UpdatedAtMS = time.Now().UnixMilli()

	ctx := context.Background()
	if err := ct.cronService.jobStore.Save(ctx, id+".json", job); err != nil {
		return fmt.Errorf("failed to save cancelled job: %w", err)
	}

	select {
	case ct.cronService.resetCh <- struct{}{}:
	default:
		logger.L().Debug().Str("job", id).Msg("reset signal lost")
	}

	logger.L().Debug().Str("job", id).Msg("Cancelled cron job")
	return nil
}

func (ct *CronTool) ListJobs() []*CronJob {
	ct.cronService.mu.RLock()
	defer ct.cronService.mu.RUnlock()

	jobs := make([]*CronJob, 0, len(ct.cronService.jobMap))
	for _, job := range ct.cronService.jobMap {
		if job.Enabled {
			jobs = append(jobs, job)
		}
	}
	return jobs
}

func (cs *CronService) Name() string {
	return "CronService"
}

func (cs *CronService) SetOnJob(executor func(job *CronJob) (string, error)) {
	cs.onJob = executor
}

func (cs *CronService) handleCronJob(job *CronJob) {
	if !job.Enabled {
		logger.L().Debug().Str("job", job.ID).Msg("Skipping disabled job")
		return
	}

	_, err := cs.onJob(job)

	nowMS := time.Now().UnixMilli()
	job.State.LastRunAtMS = nowMS
	if err != nil {
		logger.L().Error().Err(err).Str("job", job.ID).Msg("Failed to execute cron job")
		job.State.LastStatus = "failed"
		job.State.LastError = err.Error()
	} else {
		job.State.LastStatus = "success"
		job.State.LastError = ""
	}
	job.UpdatedAtMS = nowMS

	if job.Schedule.Kind == "Every" {
		job.State.NextRunAtMS = CalculateNextRunAt(job.Schedule, nowMS)
		cs.cronTool.AddJob(job)
	} else if job.DeleteAfterRun {
		cs.cronTool.DeleteJob(job.ID)
	} else {
		cs.mu.Lock()
		delete(cs.jobMap, job.ID)
		cs.mu.Unlock()

		ctx := context.Background()
		cs.jobStore.Delete(ctx, job.ID+".json")
	}

	select {
	case cs.resetCh <- struct{}{}:
	default:
		logger.L().Debug().Str("job", job.ID).Msg("reset signal lost")
	}
}

func (cs *CronService) runLoop() {
	logger.L().Info().Str("service", cs.Name()).Msg("Cron Service started")
	timer := time.NewTimer(time.Hour * 24 * 24)
	defer timer.Stop()

	for {
		select {
		case <-cs.stopCh:
			logger.L().Info().Msg("Cron Service stopping")
			close(cs.doneCh)
			return
		case <-timer.C:
			job := cs.store.SafePop()
			if job != nil && job.Enabled {
				go cs.handleCronJob(job)
			}
		case <-cs.resetCh:
			cs.store.CleanExpiredJobs()
			job := cs.store.SafePeek()
			if job != nil && job.Enabled {
				if !timer.Stop() {
					select {
					case <-timer.C:
					default:
					}
				}
				waitTime := time.Duration(job.State.NextRunAtMS-time.Now().UnixMilli()) * time.Millisecond
				if waitTime <= 0 {
					waitTime = 0
				}
				timer.Reset(waitTime)
			}
		}
	}
}

func (cs *CronService) Start(ctx context.Context) error {
	if err := cs.loadPersistedJobs(ctx); err != nil {
		return fmt.Errorf("failed to load persisted cron jobs: %w", err)
	}
	go cs.runLoop()
	logger.L().Info().Msg("CronService started")
	return nil
}

func (cs *CronService) Stop(ctx context.Context) error {
	close(cs.stopCh)
	select {
	case <-cs.doneCh:
	case <-ctx.Done():
		return ctx.Err()
	}
	logger.L().Info().Msg("CronService stopped")
	return nil
}

func (cs *CronService) loadPersistedJobs(ctx context.Context) error {
	dataDir := cs.jobStore.DataDir
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return err
	}

	files, err := os.ReadDir(dataDir)
	if err != nil {
		return err
	}

	nowMS := time.Now().UnixMilli()

	for _, file := range files {
		if filepath.Ext(file.Name()) != ".json" {
			continue
		}
		id := file.Name()[:len(file.Name())-5]
		job, err := cs.jobStore.Load(ctx, file.Name())
		if err != nil {
			logger.L().Warn().Err(err).Str("file", file.Name()).Msg("Failed to load cron job from disk")
			continue
		}

		if job.DeleteAfterRun && job.State.NextRunAtMS < nowMS {
			logger.L().Debug().Str("job", id).Msg("Deleting expired one-time cron job")
			cs.jobStore.Delete(ctx, file.Name())
			continue
		}

		if job.Schedule.Kind == "Every" {
			job.State.NextRunAtMS = CalculateNextRunAt(job.Schedule, nowMS)
			job.UpdatedAtMS = nowMS
			if err := cs.jobStore.Save(ctx, file.Name(), job); err != nil {
				logger.L().Warn().Err(err).Str("job", id).Msg("Failed to save updated cron job")
			}
		}

		if job.Enabled {
			cs.jobMap[id] = job
			cs.store.SafePush(job)
		} else {
			cs.jobMap[id] = job
		}

		logger.L().Debug().Str("job", id).Msg("Loaded cron job from disk")
	}

	select {
	case cs.resetCh <- struct{}{}:
	default:
		logger.L().Debug().Msg("reset signal lost")
	}

	return nil
}

func (ct *CronTool) Name() string {
	return "cron"
}

func (ct *CronTool) Description() string {
	return "Create and manage scheduled tasks. Supports one-time (At) or recurring (Every) jobs, and can deliver messages directly to channels or send instructions to the daemon agent for complex tasks."
}

func (ct *CronTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"action": map[string]interface{}{
				"type":        "string",
				"enum":        []string{"add", "list", "delete", "cancel"},
				"description": "Operation to perform: add a new job, list active jobs, delete a job permanently, or cancel (disable) a job without deleting it.",
			},
			"name": map[string]interface{}{
				"type":        "string",
				"description": "Human-readable name for the job (required for add action).",
			},
			"schedule": map[string]interface{}{
				"type":        "object",
				"description": "Schedule definition (required for add action).",
				"properties": map[string]interface{}{
					"kind": map[string]interface{}{
						"type":        "string",
						"enum":        []string{"At", "Every"},
						"description": "At for one-time job, Every for recurring job.",
					},
					"atMs": map[string]interface{}{
						"type":        "integer",
						"description": "Unix timestamp in milliseconds when to run (required when kind is At).",
					},
					"everyMs": map[string]interface{}{
						"type":        "integer",
						"description": "Interval in milliseconds between runs (required when kind is Every).",
					},
					"tz": map[string]interface{}{
						"type":        "string",
						"description": "Timezone for the schedule (optional, defaults to local time).",
					},
				},
				"required": []string{"kind"},
			},
			"payload": map[string]interface{}{
				"type":        "object",
				"description": "Task payload (required for add action).",
				"properties": map[string]interface{}{
					"kind": map[string]interface{}{
						"type":        "string",
						"enum":        []string{"message", "command"},
						"description": "Type of task: message for delivering a message, command for executing a command (command not implemented yet).",
					},
					"message": map[string]interface{}{
						"type":        "string",
						"description": "Message content to deliver or instruction to send to daemon agent (required when kind is message).",
					},
					"command": map[string]interface{}{
						"type":        "string",
						"description": "Command to execute (required when kind is command).",
					},
					"deliver": map[string]interface{}{
						"type":        "boolean",
						"description": "If true, deliver message directly to the specified channel without going through agent; if false, send instruction to daemon agent.",
					},
					"channel": map[string]interface{}{
						"type":        "string",
						"description": "Target channel for message delivery (required when deliver is true).",
					},
					"to": map[string]interface{}{
						"type":        "string",
						"description": "Target chat ID (optional).",
					},
					"reply_to_channel": map[string]interface{}{
						"type":        "string",
						"description": "When deliver is false, the channel where the daemon agent should send its response (optional).",
					},
					"reply_to_chat": map[string]interface{}{
						"type":        "string",
						"description": "When deliver is false, the chat ID where the daemon agent should send its response (optional).",
					},
				},
				"required": []string{"kind"},
			},
			"jobId": map[string]interface{}{
				"type":        "string",
				"description": "Job ID (required for delete and cancel actions).",
			},
		},
		"required": []string{"action"},
	}
}

func (ct *CronTool) Execute(ctx context.Context, args string) (interface{}, error) {
	var req struct {
		Action   string      `json:"action"`
		Name     string      `json:"name,omitempty"`
		Schedule interface{} `json:"schedule,omitempty"`
		Payload  interface{} `json:"payload,omitempty"`
		JobID    string      `json:"jobId,omitempty"`
	}

	if err := json.Unmarshal([]byte(args), &req); err != nil {
		return nil, fmt.Errorf("failed to parse cron tool args: %w", err)
	}

	switch req.Action {
	case "add":
		if req.Name == "" {
			return nil, fmt.Errorf("name is required for add action")
		}
		if req.Schedule == nil {
			return nil, fmt.Errorf("schedule is required for add action")
		}
		if req.Payload == nil {
			return nil, fmt.Errorf("payload is required for add action")
		}

		scheduleBytes, err := json.Marshal(req.Schedule)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal schedule: %w", err)
		}
		var schedule CronSchedule
		if err := json.Unmarshal(scheduleBytes, &schedule); err != nil {
			return nil, fmt.Errorf("failed to parse schedule: %w", err)
		}

		payloadBytes, err := json.Marshal(req.Payload)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal payload: %w", err)
		}
		var payload CronPayload
		if err := json.Unmarshal(payloadBytes, &payload); err != nil {
			return nil, fmt.Errorf("failed to parse payload: %w", err)
		}

		job := &CronJob{
			ID:       GenerateJobID(),
			Name:     req.Name,
			Enabled:  true,
			Schedule: schedule,
			Payload:  payload,
		}

		if err := ct.AddJob(job); err != nil {
			return nil, err
		}

		return map[string]interface{}{"jobId": job.ID, "status": "success"}, nil

	case "list":
		jobs := ct.ListJobs()
		result := make([]map[string]interface{}, 0, len(jobs))
		for _, job := range jobs {
			result = append(result, map[string]interface{}{
				"jobId":     job.ID,
				"name":      job.Name,
				"enabled":   job.Enabled,
				"schedule":  job.Schedule,
				"state":     job.State,
				"createdAt": job.CreatedAtMS,
			})
		}
		return result, nil

	case "delete":
		if req.JobID == "" {
			return nil, fmt.Errorf("jobId is required for delete action")
		}
		if err := ct.DeleteJob(req.JobID); err != nil {
			return nil, err
		}
		return map[string]interface{}{"jobId": req.JobID, "status": "deleted"}, nil

	case "cancel":
		if req.JobID == "" {
			return nil, fmt.Errorf("jobId is required for cancel action")
		}
		if err := ct.CancelJob(req.JobID); err != nil {
			return nil, err
		}
		return map[string]interface{}{"jobId": req.JobID, "status": "cancelled"}, nil

	default:
		return nil, fmt.Errorf("unknown action: %s", req.Action)
	}
}
