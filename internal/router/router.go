package router

import (
	"context"
	"time"

	"github.com/eko/iris-bot/internal/domain"
	"github.com/eko/iris-bot/internal/repository"
)

type TriggerRouter struct {
	exceptionChannelRepo repository.ExceptionChannelQuerier
	allowedChannelRepo   repository.AllowedChannelQuerier
	convRepo             repository.ChannelConversationQuerier
	botID                int64
}

func NewTriggerRouter(exceptionChannelRepo repository.ExceptionChannelQuerier) *TriggerRouter {
	return &TriggerRouter{
		exceptionChannelRepo: exceptionChannelRepo,
		allowedChannelRepo:   &NoopAllowedRepo{},
		convRepo:             nil,
		botID:                999,
	}
}

func NewTriggerRouterWithBotID(exceptionChannelRepo repository.ExceptionChannelQuerier, botID int64) *TriggerRouter {
	return &TriggerRouter{
		exceptionChannelRepo: exceptionChannelRepo,
		allowedChannelRepo:   &NoopAllowedRepo{},
		convRepo:             nil,
		botID:                botID,
	}
}

func NewTriggerRouterWithAllowList(exceptionChannelRepo repository.ExceptionChannelQuerier, allowedChannelRepo repository.AllowedChannelQuerier) *TriggerRouter {
	return &TriggerRouter{
		exceptionChannelRepo: exceptionChannelRepo,
		allowedChannelRepo:   allowedChannelRepo,
		convRepo:             nil,
		botID:                999,
	}
}

func NewTriggerRouterWithConversation(exceptionChannelRepo repository.ExceptionChannelQuerier, allowedChannelRepo repository.AllowedChannelQuerier, convRepo repository.ChannelConversationQuerier, botID int64) *TriggerRouter {
	return &TriggerRouter{
		exceptionChannelRepo: exceptionChannelRepo,
		allowedChannelRepo:   allowedChannelRepo,
		convRepo:             convRepo,
		botID:                botID,
	}
}

func (tr *TriggerRouter) SetBotID(botID int64) {
	tr.botID = botID
}

// NoopAllowedRepo is a stub that always reports no allowed channels (fallback mode).
type NoopAllowedRepo struct{}

func (n *NoopAllowedRepo) Add(ctx context.Context, guildID int64, channelID int64) error {
	return nil
}

func (n *NoopAllowedRepo) Remove(ctx context.Context, guildID int64, channelID int64) error {
	return nil
}

func (n *NoopAllowedRepo) IsAllowed(ctx context.Context, guildID int64, channelID int64) (bool, error) {
	return false, nil
}

func (n *NoopAllowedRepo) HasAny(ctx context.Context, guildID int64) (bool, error) {
	return false, nil
}

func (n *NoopAllowedRepo) ListByGuild(ctx context.Context, guildID int64) ([]int64, error) {
	return nil, nil
}

func (tr *TriggerRouter) Decide(ctx context.Context, event *domain.DiscordEvent) (*Decision, error) {
	if event.UserID == tr.botID {
		return Ignore(ReasonBotMessage), nil
	}

	hasAllowed, err := tr.allowedChannelRepo.HasAny(ctx, event.GuildID)
	if err != nil {
		return nil, err
	}

	if hasAllowed {
		// Include-list mode: only respond in allowed channels
		isAllowed, err := tr.allowedChannelRepo.IsAllowed(ctx, event.GuildID, event.ChannelID)
		if err != nil {
			return nil, err
		}

		if !isAllowed {
			return Ignore(ReasonChannelNotAllowed), nil
		}
	} else {
		// Fallback mode: use exception-channel behavior
		isException, err := tr.exceptionChannelRepo.IsException(ctx, event.GuildID, event.ChannelID)
		if err != nil {
			return nil, err
		}

		if isException {
			return Ignore(ReasonExceptionChannel), nil
		}
	}

	switch event.Type {
	case "message_mention":
		return Respond(ReasonMention), nil
	case "message_reply":
		return Respond(ReasonReply), nil
	case "message_content":
		return Respond(ReasonNameMention), nil
	case "message_casual":
		// Check if conversation is active for casual messages
		if tr.convRepo != nil {
			isActive, err := tr.convRepo.Active(ctx, event.GuildID, event.ChannelID, time.Now())
			if err != nil {
				return nil, err
			}

			if isActive {
				return Respond(ReasonActiveConversation), nil
			}
		}

		return Ignore(ReasonNoTrigger), nil
	default:
		return Ignore(ReasonNoTrigger), nil
	}
}
