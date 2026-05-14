package reminder

import (
	"context"
	"time"
)

type Service struct {
	Store     Store
	Clock     Clock
	Scheduler *Scheduler
}

type CreateInput struct {
	GuildID   int64
	ChannelID int64
	CreatedBy int64
	Kind      ReminderKind
	Message   string
	Timezone  string
	HourMin   string
	Weekday   time.Weekday
	RunAt     time.Time
}

func NewService(store Store, clock Clock, sender Sender) *Service {
	scheduler := NewScheduler(store, clock, sender)
	return &Service{
		Store:     store,
		Clock:     clock,
		Scheduler: scheduler,
	}
}

func (s *Service) Create(ctx context.Context, input CreateInput) (*Reminder, error) {
	if input.ChannelID <= 0 {
		return nil, ErrInvalidChannel
	}

	if input.Message == "" {
		return nil, ErrInvalidMessage
	}

	_, err := time.LoadLocation(input.Timezone)
	if err != nil {
		return nil, ErrInvalidTimezone
	}

	parts := input.HourMin
	if input.Kind != KindOnce {
		_, err := ComputeNextRun(&Reminder{
			Kind:     input.Kind,
			Timezone: input.Timezone,
			HourMin:  parts,
			Weekday:  input.Weekday,
		}, s.Clock.Now())
		if err != nil {
			return nil, err
		}
	}

	reminder := &Reminder{
		GuildID:   input.GuildID,
		ChannelID: input.ChannelID,
		CreatedBy: input.CreatedBy,
		Kind:      input.Kind,
		Message:   input.Message,
		Timezone:  input.Timezone,
		HourMin:   parts,
		Weekday:   input.Weekday,
	}

	if input.Kind == KindOnce {
		reminder.NextRun = input.RunAt
	} else {
		next, err := ComputeNextRun(reminder, s.Clock.Now())
		if err != nil {
			return nil, err
		}
		reminder.NextRun = next
	}

	id, err := s.Store.Create(ctx, reminder)
	if err != nil {
		return nil, err
	}

	reminder.ID = id
	return reminder, nil
}

func (s *Service) List(ctx context.Context, guildID int64) ([]*Reminder, error) {
	return s.Store.List(ctx, guildID)
}

func (s *Service) Delete(ctx context.Context, guildID, id int64) error {
	return s.Store.Delete(ctx, guildID, id)
}

func (s *Service) Start(ctx context.Context) {
	s.Scheduler.Start(ctx)
}

func (s *Service) Stop() {
	s.Scheduler.Stop()
}
