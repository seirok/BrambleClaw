package scheduler

import (
	"context"
	"fmt"
	"testing"
	"time"
)

func TestBasicSche(t *testing.T) {
	ctx := context.Background()

	sched := NewScheduler()
	sched.Start(ctx)
	defer sched.Stop()

	handler := NewSimpleHandler("my_task", func(ctx context.Context, params map[string]any) (any, error) {
		fmt.Println("Task executed!")
		return "success", nil
	})

	schedule := NewIntervalSchedule(1 * time.Minute)
	job := NewJob("my_job", "My first job", handler, schedule, nil)

	sched.AddJob(job)
}
