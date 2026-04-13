package scheduler

import (
	"fmt"
	"time"

	"github.com/robfig/cron/v3"
)

type OnceSchedule struct {
	at time.Time
}

func NewOnceSchedule(at time.Time) *OnceSchedule {
	return &OnceSchedule{at: at}
}

func (s *OnceSchedule) Next(now time.Time) time.Time {
	if s.at.After(now) {
		return s.at
	}
	return time.Time{}
}

func (s *OnceSchedule) String() string {
	return fmt.Sprintf("once at %s", s.at.Format("2006-01-02 15:04:05"))
}

func (s *OnceSchedule) Type() string {
	return "once"
}

type IntervalSchedule struct {
	interval time.Duration
}

func NewIntervalSchedule(interval time.Duration) *IntervalSchedule {
	return &IntervalSchedule{interval: interval}
}

func (s *IntervalSchedule) Next(now time.Time) time.Time {
	return now.Add(s.interval)
}

func (s *IntervalSchedule) String() string {
	return fmt.Sprintf("every %s", s.interval)
}

func (s *IntervalSchedule) Type() string {
	return "interval"
}

type CronSchedule struct {
	expr string
	cron *cron.Cron
}

func NewCronSchedule(expr string) (*CronSchedule, error) {
	c := cron.New(cron.WithSeconds())
	if _, err := c.AddFunc(expr, func() {}); err != nil {
		return nil, fmt.Errorf("invalid cron expression: %w", err)
	}
	return &CronSchedule{expr: expr, cron: c}, nil
}

func (s *CronSchedule) Next(now time.Time) time.Time {
	next := s.cron.Next(now)
	if next.IsZero() {
		return time.Time{}
	}
	return next
}

func (s *CronSchedule) String() string {
	return fmt.Sprintf("cron %s", s.expr)
}

func (s *CronSchedule) Type() string {
	return "cron"
}

type ScheduleFactory struct{}

func NewScheduleFactory() *ScheduleFactory {
	return &ScheduleFactory{}
}

func (f *ScheduleFactory) CreateSchedule(config map[string]any) (Schedule, error) {
	if config == nil {
		return nil, fmt.Errorf("config is required")
	}

	scheduleType, ok := config["type"].(string)
	if !ok {
		return nil, fmt.Errorf("schedule type is required")
	}

	switch scheduleType {
	case "once":
		atStr, ok := config["at"].(string)
		if !ok {
			return nil, fmt.Errorf("at time is required for once schedule")
		}
		at, err := time.Parse(time.RFC3339, atStr)
		if err != nil {
			return nil, fmt.Errorf("invalid at time format: %w", err)
		}
		return NewOnceSchedule(at), nil

	case "interval":
		seconds, ok := config["seconds"].(float64)
		if !ok || seconds <= 0 {
			return nil, fmt.Errorf("valid seconds is required for interval schedule")
		}
		return NewIntervalSchedule(time.Duration(seconds) * time.Second), nil

	case "cron":
		expr, ok := config["expr"].(string)
		if !ok {
			return nil, fmt.Errorf("cron expression is required for cron schedule")
		}
		return NewCronSchedule(expr)

	default:
		return nil, fmt.Errorf("unknown schedule type: %s", scheduleType)
	}
}

func (f *ScheduleFactory) CreateOnceSchedule(at time.Time) Schedule {
	return NewOnceSchedule(at)
}

func (f *ScheduleFactory) CreateIntervalSchedule(interval time.Duration) Schedule {
	return NewIntervalSchedule(interval)
}

func (f *ScheduleFactory) CreateCronSchedule(expr string) (Schedule, error) {
	return NewCronSchedule(expr)
}
