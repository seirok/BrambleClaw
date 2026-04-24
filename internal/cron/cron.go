package cron

import (
	"brambleclaw/internal/logger"
	"container/heap"
	"fmt"
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
	Kind    string `json:"kind"`
	Message string `json:"message"`
	Command string `json:"command,omitempty"`
	Deliver bool   `json:"deliver"`
	Channel string `json:"channel,omitempty"`
	To      string `json:"to,omitempty"`
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
	mu    sync.Mutex // 内部互斥锁
}

// --- 以下为 heap.Interface 的非导出实现，仅供内部使用 ---

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
	jq.queue[n-1] = nil // 避免内存泄漏
	jq.queue = jq.queue[0 : n-1]
	return item
}

// --- 以下为对外提供的安全接口 ---
func (jq *JobQueue) SafePush(job *CronJob) {
	jq.mu.Lock()
	defer jq.mu.Unlock()
	heap.Push(jq, job) // heap 内部会调用非导出的 Push/Less/Swap
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
	// 堆顶永远是 queue[0]
	return jq.queue[0]
}

func (jq *JobQueue) CleanExpiredJobs() {
	jq.mu.Lock()
	defer jq.mu.Unlock()
	for len(jq.queue) > 0 {
		job := jq.queue[0]
		waitTime := time.Duration(job.State.NextRunAtMS-time.Now().UnixMilli()) * time.Millisecond
		if waitTime < 0 {
			logger.L().Debug().Str("Job", job.ID).Msg("clean expired job")
			heap.Pop(jq)
		} else {
			break
		}
	}
	return
}

type CronService struct {
	onJob    func(job *CronJob) (string, error) // 任务队列的任务处理函数
	store    *JobQueue
	resetCh  chan struct{} // 有新任务加入队列的信号
	stopCh   chan struct{} // 中止信号
	cronTool *CronTool
	mu       sync.RWMutex
}

type CronTool struct {
	cronService *CronService
	mu          sync.RWMutex
}

type NewCronTool *CronTool

func (ct *CronTool) AddJob(job *CronJob) error {
	if job.Schedule.Kind == "Every" {
		job.State.NextRunAtMS = time.Now().UnixMilli() + job.Schedule.EveryMS
	} else if job.Schedule.Kind == "At" {
		job.State.NextRunAtMS = job.Schedule.AtMS
	}

	ct.cronService.store.SafePush(job)
	ct.cronService.resetCh <- struct{}{}
	logger.L().Debug().Str("Job", job.ID).Msg("Added job")
	return nil
}

func (ct *CronTool) ExecuteJob(job *CronJob) (string, error) {
	fmt.Println(job.Name + "Execute job!")
	return "", nil
}

func NewCronServiceAndTool() (*CronService, *CronTool) {
	store := &JobQueue{}
	heap.Init(store)
	cronService := &CronService{
		store:   store,
		stopCh:  make(chan struct{}),
		resetCh: make(chan struct{}),
	}
	cronTool := &CronTool{cronService: cronService}
	cronService.cronTool = cronTool
	cronService.SetOnJob(cronTool.ExecuteJob)

	return cronService, cronTool
}

func (ct *CronTool) calculateNextRun(sche CronSchedule) int64 {
	return time.Now().UnixMilli() + sche.EveryMS
}

func (cs *CronService) Name() string {
	return "CronService"
}

func (cs *CronService) SetOnJob(executor func(job *CronJob) (string, error)) {
	cs.onJob = executor
}

func (cs *CronService) handleCronJob(job *CronJob) {
	// 调用注册的任务处理函数处理任务
	_, err := cs.onJob(job)
	if err != nil {
		logger.L().Error().Err(err).Str("job", job.ID).Msg("Fail to execute cron job")
		return
	}

	// 更新任务状态
	job.State.LastRunAtMS = time.Now().UnixMilli()

	// 如果任务是多轮任务，计算任务下一次执行时间并重新加入队列
	if job.Schedule.Kind == "Every" {
		cs.cronTool.AddJob(job)
	}

	// 发送reset信号给loop，以重新设置定时器
	select {
	case cs.resetCh <- struct{}{}:
	default:
		logger.L().Debug().Str("job", job.ID).Msg("reset signal lost")
	}
	return
}

func (cs *CronService) runLoop() {
	// 初始状态，没有任务，整个Loop应该阻塞
	logger.L().Info().Str("job", cs.Name()).Msg("Cron Service start...")
	timer := time.NewTimer(time.Hour * 24 * 24)
	defer timer.Stop()
	for {
		select {
		case <-cs.stopCh:
			logger.L().Info().Msg("CronService stopped")
			return
		case <-timer.C:
			job := cs.store.SafePop()
			if job != nil {
				go func() {
					cs.handleCronJob(job)
				}()
			}
			continue

		case <-cs.resetCh:
			cs.store.CleanExpiredJobs()
			job := cs.store.SafePeek()
			if job != nil {
				if !timer.Stop() {
					select {
					case <-timer.C:
					default:
					}
				}
				waitTime := time.Duration(job.State.NextRunAtMS-time.Now().UnixMilli()) * time.Millisecond
				if waitTime > 0 {
					timer.Reset(waitTime)
				}
			}
		}
	}
}
