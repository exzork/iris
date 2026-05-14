package reminder

import (
	"context"
	"sync"
	"time"
)

type Sender interface {
	Send(ctx context.Context, guildID, channelID int64, content string) error
}

type Scheduler struct {
	Store  Store
	Clock  Clock
	Sender Sender
	Tick   time.Duration
	stopCh chan struct{}
	runMu  sync.Mutex
	running bool
}

func NewScheduler(store Store, clock Clock, sender Sender) *Scheduler {
	return &Scheduler{
		Store:  store,
		Clock:  clock,
		Sender: sender,
		Tick:   30 * time.Second,
		stopCh: make(chan struct{}),
	}
}

func (s *Scheduler) TickOnce(ctx context.Context) error {
	now := s.Clock.Now()
	due, err := s.Store.Due(ctx, now)
	if err != nil {
		return err
	}

	for _, reminder := range due {
		err := s.Sender.Send(ctx, reminder.GuildID, reminder.ChannelID, reminder.Message)
		if err != nil {
			continue
		}

		if reminder.Kind == KindOnce {
			s.Store.Delete(ctx, reminder.GuildID, reminder.ID)
		} else {
			next, err := ComputeNextRun(reminder, now)
			if err != nil {
				continue
			}
			s.Store.UpdateNextRun(ctx, reminder.ID, next)
		}
	}

	return nil
}

func (s *Scheduler) Start(ctx context.Context) {
	s.runMu.Lock()
	if s.running {
		s.runMu.Unlock()
		return
	}
	s.running = true
	s.runMu.Unlock()

	go func() {
		ticker := time.NewTicker(s.Tick)
		defer ticker.Stop()

		for {
			select {
			case <-s.stopCh:
				return
			case <-ticker.C:
				s.TickOnce(ctx)
			}
		}
	}()
}

func (s *Scheduler) Stop() {
	s.runMu.Lock()
	defer s.runMu.Unlock()

	if s.running {
		s.running = false
		close(s.stopCh)
	}
}
