package scheduler

import (
	"time"
)

type DefaultClock struct{}

func NewDefaultClock() *DefaultClock {
	return &DefaultClock{}
}

func (c *DefaultClock) Now() time.Time {
	return time.Now()
}

func (c *DefaultClock) After(d time.Duration) <-chan time.Time {
	return time.After(d)
}

func (c *DefaultClock) Tick(d time.Duration) <-chan time.Time {
	return time.Tick(d)
}

func (c *DefaultClock) NewTimer(d time.Duration) *time.Timer {
	return time.NewTimer(d)
}

func (c *DefaultClock) NewTicker(d time.Duration) *time.Ticker {
	return time.NewTicker(d)
}

func (c *DefaultClock) Sleep(d time.Duration) {
	time.Sleep(d)
}

type MockClock struct {
	now time.Time
}

func NewMockClock(now time.Time) *MockClock {
	return &MockClock{
		now: now,
	}
}

func (c *MockClock) Now() time.Time {
	return c.now
}

func (c *MockClock) SetNow(now time.Time) {
	c.now = now
}

func (c *MockClock) Add(d time.Duration) {
	c.now = c.now.Add(d)
}

func (c *MockClock) After(d time.Duration) <-chan time.Time {
	ch := make(chan time.Time, 1)
	go func() {
		time.Sleep(d)
		ch <- c.now.Add(d)
	}()
	return ch
}

func (c *MockClock) Tick(d time.Duration) <-chan time.Time {
	ch := make(chan time.Time)
	go func() {
		for {
			time.Sleep(d)
			c.now = c.now.Add(d)
			ch <- c.now
		}
	}()
	return ch
}

func (c *MockClock) NewTimer(d time.Duration) *time.Timer {
	return time.NewTimer(d)
}

func (c *MockClock) NewTicker(d time.Duration) *time.Ticker {
	return time.NewTicker(d)
}

func (c *MockClock) Sleep(d time.Duration) {
	time.Sleep(d)
	c.now = c.now.Add(d)
}
