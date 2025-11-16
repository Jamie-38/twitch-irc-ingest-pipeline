package channelrecord

import (
	"sync"
	"time"
)

type fakeClock struct {
	mu  sync.Mutex
	now time.Time

	afters []chan time.Time
}

func newFakeClock(start time.Time) *fakeClock {
	return &fakeClock{now: start}
}

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *fakeClock) After(d time.Duration) <-chan time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	ch := make(chan time.Time, 1)

	c.afters = append(c.afters, ch)
	return ch
}

func (c *fakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	c.now = c.now.Add(d)

	fired := append([]chan time.Time(nil), c.afters...)
	c.afters = nil
	now := c.now
	c.mu.Unlock()

	for _, ch := range fired {
		ch <- now
		close(ch)
	}
}
