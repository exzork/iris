package discord

import (
	"errors"
	"strconv"
	"strings"

	"github.com/bwmarrin/discordgo"
	"github.com/eko/iris-bot/internal/domain"
)

var (
	ErrBotMessage = errors.New("message from bot, ignoring")
	ErrNilMessage = errors.New("message is nil")
)

type EventNormalizer struct {
	botID              int64
	referencedMessages map[string]*discordgo.Message
}

type MessageAttachment struct {
	ID   string
	URL  string
	Size int
}

func NewEventNormalizer(botID int64) *EventNormalizer {
	return &EventNormalizer{
		botID:              botID,
		referencedMessages: make(map[string]*discordgo.Message),
	}
}

func (en *EventNormalizer) SetReferencedMessage(msgID string, msg *discordgo.Message) {
	en.referencedMessages[msgID] = msg
}

func (en *EventNormalizer) NormalizeMessageCreate(msg *discordgo.Message) (*domain.DiscordEvent, error) {
	if msg == nil {
		return nil, ErrNilMessage
	}

	authorID, err := strconv.ParseInt(msg.Author.ID, 10, 64)
	if err != nil {
		return nil, err
	}

	if authorID == en.botID {
		return nil, ErrBotMessage
	}

	guildID, err := strconv.ParseInt(msg.GuildID, 10, 64)
	if err != nil {
		return nil, err
	}

	channelID, err := strconv.ParseInt(msg.ChannelID, 10, 64)
	if err != nil {
		return nil, err
	}

	eventType := en.detectEventType(msg)

	// Extract reply metadata if present
	var replyToMessageID *int64
	var replyToChannelID *int64
	if msg.MessageReference != nil {
		if refMsgID, err := strconv.ParseInt(msg.MessageReference.MessageID, 10, 64); err == nil {
			replyToMessageID = &refMsgID
		}
		if refChanID, err := strconv.ParseInt(msg.MessageReference.ChannelID, 10, 64); err == nil {
			replyToChannelID = &refChanID
		}
	}

	authorName := msg.Author.Username
	attachmentCount := len(msg.Attachments)

	discordMsg := &domain.DiscordMessage{
		ID:               parseID(msg.ID),
		GuildID:          guildID,
		ChannelID:        channelID,
		UserID:           authorID,
		AuthorName:       &authorName,
		Content:          msg.Content,
		AttachmentCount:  attachmentCount,
		ReplyToMessageID: replyToMessageID,
		ReplyToChannelID: replyToChannelID,
		IsBot:            msg.Author.Bot,
		CreatedAt:        msg.Timestamp,
	}

	if len(msg.Attachments) > 0 {
		discordMsg.Attachments = make([]interface{}, len(msg.Attachments))
		for i, att := range msg.Attachments {
			discordMsg.Attachments[i] = MessageAttachment{
				ID:   att.ID,
				URL:  att.URL,
				Size: att.Size,
			}
		}
	}

	event := &domain.DiscordEvent{
		Type:             eventType,
		GuildID:          guildID,
		ChannelID:        channelID,
		UserID:           authorID,
		AuthorName:       &authorName,
		Message:          discordMsg,
		ReplyToMessageID: replyToMessageID,
		ReplyToChannelID: replyToChannelID,
		IsBot:            msg.Author.Bot,
		AttachmentCount:  attachmentCount,
		CreatedAt:        msg.Timestamp,
	}

	return event, nil
}

func (en *EventNormalizer) detectEventType(msg *discordgo.Message) string {
	if en.isMentioned(msg) {
		return "message_mention"
	}

	if en.isReplyToBot(msg) {
		return "message_reply"
	}

	if en.containsIrisKeyword(msg.Content) {
		return "message_content"
	}

	return "message_casual"
}

func (en *EventNormalizer) isMentioned(msg *discordgo.Message) bool {
	for _, mention := range msg.Mentions {
		mentionID, err := strconv.ParseInt(mention.ID, 10, 64)
		if err != nil {
			continue
		}
		if mentionID == en.botID {
			return true
		}
	}
	return false
}

func (en *EventNormalizer) isReplyToBot(msg *discordgo.Message) bool {
	if msg.MessageReference == nil {
		return false
	}

	refMsg, exists := en.referencedMessages[msg.MessageReference.MessageID]
	if !exists {
		return false
	}

	if refMsg.Author == nil {
		return false
	}

	refAuthorID, err := strconv.ParseInt(refMsg.Author.ID, 10, 64)
	if err != nil {
		return false
	}

	return refAuthorID == en.botID
}

func (en *EventNormalizer) containsIrisKeyword(content string) bool {
	lower := strings.ToLower(content)
	return strings.Contains(lower, "iris")
}

func parseID(id string) int64 {
	parsed, _ := strconv.ParseInt(id, 10, 64)
	return parsed
}
