package lorethread

import "time"

// RealClock provides actual system time.
type RealClock struct{}

func (c *RealClock) Now() time.Time {
	return time.Now()
}

func (c *RealClock) After(d time.Duration) <-chan time.Time {
	return time.After(d)
}

// FakeClock provides deterministic time for testing.
type FakeClock struct {
	current time.Time
	timers  []*fakeTimer
}

type fakeTimer struct {
	deadline time.Time
	ch       chan time.Time
}

// NewFakeClock creates a new fake clock starting at the given time.
func NewFakeClock(t time.Time) *FakeClock {
	return &FakeClock{current: t}
}

func (c *FakeClock) Now() time.Time {
	return c.current
}

func (c *FakeClock) After(d time.Duration) <-chan time.Time {
	ch := make(chan time.Time, 1)
	c.timers = append(c.timers, &fakeTimer{
		deadline: c.current.Add(d),
		ch:       ch,
	})
	return ch
}

// Advance moves the fake clock forward and fires any timers that have expired.
func (c *FakeClock) Advance(d time.Duration) {
	c.current = c.current.Add(d)
	for _, timer := range c.timers {
		if !timer.deadline.After(c.current) {
			select {
			case timer.ch <- c.current:
			default:
			}
		}
	}
}
