package cron

import (
	"sync"
	"testing"
)

func TestCron(t *testing.T) {
	cronService, cronTool := NewCronServiceAndTool()
	// 启动CronService
	wg := sync.WaitGroup{}
	wg.Add(1)
	go func() {
		defer wg.Done()
		cronService.runLoop()
	}()

	// 手动添加任务
	cronTool.AddJob(&CronJob{
		ID:   "1",
		Name: "Job 1",
		Schedule: CronSchedule{
			Kind:    "Every",
			EveryMS: 10000,
		},
	})
	cronTool.AddJob(&CronJob{
		ID:   "2",
		Name: "Job 2",
		Schedule: CronSchedule{
			Kind: "At",
		},
	})

	// 观察输出 。。。

	// 中途添加任务
	cronTool.AddJob(&CronJob{
		ID:   "3",
		Name: "Job 3",
		Schedule: CronSchedule{
			Kind:    "Every",
			EveryMS: 10000,
		},
	})
	cronTool.AddJob(&CronJob{
		ID:   "4",
		Name: "Job 4",
		Schedule: CronSchedule{
			Kind: "At",
		},
	})
	wg.Wait()

	// 观察输出。。。

}
