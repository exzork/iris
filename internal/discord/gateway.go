package discord

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"sync"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/eko/iris-bot/internal/domain"
)

type EventCallback func(ctx context.Context, event *domain.DiscordEvent) error

type InteractionHandler interface {
	HandleInteraction(ctx context.Context, s *discordgo.Session, i *discordgo.InteractionCreate)
}

type GuildAvailableCallback func(ctx context.Context, guildID int64)

type GatewayAdapter struct {
	session             *discordgo.Session
	normalizer          *EventNormalizer
	callback            EventCallback
	interactionHandler  InteractionHandler
	onGuildAvailable    GuildAvailableCallback
	workQueue           chan *domain.DiscordEvent
	stopChan            chan struct{}
	wg                  sync.WaitGroup
	maxQueueSize        int
	logger              *slog.Logger
}

type SessionManager struct {
	adapters map[int64]*GatewayAdapter
	mu       sync.RWMutex
}

func NewSessionManager() *SessionManager {
	return &SessionManager{
		adapters: make(map[int64]*GatewayAdapter),
	}
}

func NewGatewayAdapter(token string, botID int64, callback EventCallback) (*GatewayAdapter, error) {
	session, err := discordgo.New("Bot " + token)
	if err != nil {
		return nil, fmt.Errorf("failed to create Discord session: %w", err)
	}

	session.Identify.Intents = discordgo.IntentsGuilds | discordgo.IntentsGuildMessages | discordgo.IntentsGuildMessageTyping | discordgo.IntentMessageContent

	adapter := &GatewayAdapter{
		session:      session,
		normalizer:   NewEventNormalizer(botID),
		callback:     callback,
		workQueue:    make(chan *domain.DiscordEvent, 100),
		stopChan:     make(chan struct{}),
		maxQueueSize: 100,
		logger:       slog.Default(),
	}

	session.AddHandler(adapter.onMessageCreate)
	session.AddHandler(adapter.onMessageUpdate)
	session.AddHandler(adapter.onInteractionCreate)
	session.AddHandler(adapter.onReady)
	session.AddHandler(adapter.onGuildCreate)

	return adapter, nil
}

// Session exposes the underlying discordgo session so callers (e.g. the slash
// command registrar) can issue REST calls outside the event-loop hot path.
// Returns nil before Connect has been called.
func (ga *GatewayAdapter) Session() *discordgo.Session { return ga.session }

// SetInteractionHandler attaches the slash/interaction dispatcher. Safe to
// call before Connect; interactions arriving before a handler is set are
// dropped with a debug log.
func (ga *GatewayAdapter) SetInteractionHandler(h InteractionHandler) {
	ga.interactionHandler = h
}

// SetGuildAvailableCallback attaches a callback fired for every guild the
// bot sees at startup (via Ready) and for every guild join (via GuildCreate).
// Used to register per-guild slash commands.
func (ga *GatewayAdapter) SetGuildAvailableCallback(cb GuildAvailableCallback) {
	ga.onGuildAvailable = cb
}

func (ga *GatewayAdapter) Connect(ctx context.Context) error {
	if err := ga.session.Open(); err != nil {
		return fmt.Errorf("failed to open Discord session: %w", err)
	}

	ga.wg.Add(1)
	go ga.processWorkQueue()

	return nil
}

func (ga *GatewayAdapter) Close() error {
	close(ga.stopChan)
	ga.wg.Wait()
	return ga.session.Close()
}

func (ga *GatewayAdapter) BotID() int64 {
	if ga.session.State == nil || ga.session.State.User == nil {
		return 0
	}
	botID, err := strconv.ParseInt(ga.session.State.User.ID, 10, 64)
	if err != nil {
		return 0
	}
	return botID
}

func (ga *GatewayAdapter) SetNormalizerBotID(botID int64) {
	ga.normalizer.botID = botID
}

func (ga *GatewayAdapter) onMessageCreate(s *discordgo.Session, m *discordgo.MessageCreate) {
	ga.handleMessage(m.Message)
}

func (ga *GatewayAdapter) onMessageUpdate(s *discordgo.Session, m *discordgo.MessageUpdate) {
	ga.handleMessage(m.Message)
}

func (ga *GatewayAdapter) onInteractionCreate(s *discordgo.Session, i *discordgo.InteractionCreate) {
	if ga.interactionHandler == nil {
		ga.logger.Debug("interaction received with no handler", "type", i.Type.String())
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()
	ga.interactionHandler.HandleInteraction(ctx, s, i)
}

func (ga *GatewayAdapter) onReady(s *discordgo.Session, r *discordgo.Ready) {
	if ga.onGuildAvailable == nil {
		return
	}
	ctx := context.Background()
	for _, g := range r.Guilds {
		if g == nil || g.ID == "" {
			continue
		}
		id, err := strconv.ParseInt(g.ID, 10, 64)
		if err != nil {
			continue
		}
		ga.onGuildAvailable(ctx, id)
	}
}

func (ga *GatewayAdapter) onGuildCreate(s *discordgo.Session, g *discordgo.GuildCreate) {
	if ga.onGuildAvailable == nil || g == nil || g.Guild == nil || g.Guild.ID == "" {
		return
	}
	id, err := strconv.ParseInt(g.Guild.ID, 10, 64)
	if err != nil {
		return
	}
	ga.onGuildAvailable(context.Background(), id)
}

func (ga *GatewayAdapter) handleMessage(msg *discordgo.Message) {
	event, err := ga.normalizer.NormalizeMessageCreate(msg)
	if err != nil {
		if err == ErrBotMessage {
			ga.logger.Debug("ignoring bot message", "channel", msg.ChannelID, "user", msg.Author.ID, "reason", "bot_message")
			return
		}
		if err == ErrNilMessage {
			ga.logger.Debug("ignoring nil message", "reason", "nil_message")
			return
		}
		ga.logger.Debug("failed to normalize message", "error", err.Error())
		return
	}

	select {
	case ga.workQueue <- event:
	case <-ga.stopChan:
		return
	default:
		ga.logger.Debug("work queue full, dropping event", "channel", event.ChannelID, "user", event.UserID, "type", event.Type)
		return
	}
}

func (ga *GatewayAdapter) processWorkQueue() {
	defer ga.wg.Done()

	for {
		select {
		case event := <-ga.workQueue:
			ga.logger.Info("dispatching event", "type", event.Type, "guild", event.GuildID, "channel", event.ChannelID, "user", event.UserID)
			ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
			if err := ga.callback(ctx, event); err != nil {
				ga.logger.Error("callback failed", "error", err)
			}
			cancel()

		case <-ga.stopChan:
			return
		}
	}
}

func (ga *GatewayAdapter) SendTyping(ctx context.Context, guildID, channelID int64) error {
	channelIDStr := fmt.Sprintf("%d", channelID)
	if err := ga.session.ChannelTyping(channelIDStr); err != nil {
		var restErr *discordgo.RESTError
		if errors.As(err, &restErr) && restErr.Response != nil {
			ga.logger.Warn("send_typing_failed",
				"guild", guildID,
				"channel", channelID,
				"http_status", restErr.Response.StatusCode,
				"err", err)
		} else {
			ga.logger.Warn("send_typing_failed",
				"guild", guildID,
				"channel", channelID,
				"err", err)
		}
		return err
	}
	return nil
}

func (ga *GatewayAdapter) SendMessage(ctx context.Context, guildID, channelID int64, content string) error {
	channelIDStr := fmt.Sprintf("%d", channelID)
	_, err := ga.session.ChannelMessageSend(channelIDStr, content)
	return err
}

func (ga *GatewayAdapter) SendMessageReturningID(ctx context.Context, guildID, channelID int64, content string) (int64, error) {
	channelIDStr := fmt.Sprintf("%d", channelID)
	msg, err := ga.session.ChannelMessageSend(channelIDStr, content)
	if err != nil {
		return 0, err
	}
	id, parseErr := strconv.ParseInt(msg.ID, 10, 64)
	if parseErr != nil {
		return 0, parseErr
	}
	return id, nil
}

func buildReplyMessage(guildID, channelID, replyToMessageID int64, content string, mentionRepliedUser bool) *discordgo.MessageSend {
	failIfNotExists := false
	return &discordgo.MessageSend{
		Content: content,
		Reference: &discordgo.MessageReference{
			Type:            discordgo.MessageReferenceTypeDefault,
			MessageID:       fmt.Sprintf("%d", replyToMessageID),
			ChannelID:       fmt.Sprintf("%d", channelID),
			GuildID:         fmt.Sprintf("%d", guildID),
			FailIfNotExists: &failIfNotExists,
		},
		AllowedMentions: &discordgo.MessageAllowedMentions{
			Parse:       []discordgo.AllowedMentionType{discordgo.AllowedMentionTypeUsers},
			RepliedUser: mentionRepliedUser,
		},
	}
}

func (ga *GatewayAdapter) ReplyMessage(ctx context.Context, guildID, channelID, replyToMessageID int64, content string, mentionRepliedUser bool) error {
	channelIDStr := fmt.Sprintf("%d", channelID)
	send := buildReplyMessage(guildID, channelID, replyToMessageID, content, mentionRepliedUser)
	_, err := ga.session.ChannelMessageSendComplex(channelIDStr, send)
	return err
}

func (ga *GatewayAdapter) ReplyMessageReturningID(ctx context.Context, guildID, channelID, replyToMessageID int64, content string, mentionRepliedUser bool) (int64, error) {
	channelIDStr := fmt.Sprintf("%d", channelID)
	send := buildReplyMessage(guildID, channelID, replyToMessageID, content, mentionRepliedUser)
	msg, err := ga.session.ChannelMessageSendComplex(channelIDStr, send)
	if err != nil {
		return 0, err
	}
	id, parseErr := strconv.ParseInt(msg.ID, 10, 64)
	if parseErr != nil {
		return 0, parseErr
	}
	return id, nil
}

func (ga *GatewayAdapter) EditMessage(ctx context.Context, guildID, channelID, messageID int64, content string) error {
	channelIDStr := fmt.Sprintf("%d", channelID)
	messageIDStr := fmt.Sprintf("%d", messageID)
	_, err := ga.session.ChannelMessageEdit(channelIDStr, messageIDStr, content)
	return err
}

func (ga *GatewayAdapter) GetMessage(ctx context.Context, guildID, channelID, messageID int64) (*domain.DiscordMessage, error) {
	channelIDStr := fmt.Sprintf("%d", channelID)
	messageIDStr := fmt.Sprintf("%d", messageID)

	msg, err := ga.session.ChannelMessage(channelIDStr, messageIDStr)
	if err != nil {
		return nil, err
	}

	return &domain.DiscordMessage{
		ID:        messageID,
		GuildID:   guildID,
		ChannelID: channelID,
		UserID:    parseID(msg.Author.ID),
		Content:   msg.Content,
		CreatedAt: msg.Timestamp,
	}, nil
}

func (ga *GatewayAdapter) GetGuild(ctx context.Context, guildID int64) (*domain.Guild, error) {
	guildIDStr := fmt.Sprintf("%d", guildID)
	_, err := ga.session.Guild(guildIDStr)
	if err != nil {
		return nil, err
	}

	return &domain.Guild{
		ID:        guildID,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}, nil
}

func (ga *GatewayAdapter) CreateThreadFromMessage(ctx context.Context, guildID, channelID, parentMessageID int64, name string, archiveAfter time.Duration) (int64, error) {
	channelIDStr := fmt.Sprintf("%d", channelID)
	messageIDStr := fmt.Sprintf("%d", parentMessageID)

	thread, err := ga.session.MessageThreadStartComplex(channelIDStr, messageIDStr, &discordgo.ThreadStart{
		Name:                name,
		AutoArchiveDuration: int(archiveAfter.Minutes()),
	})
	if err != nil {
		var restErr *discordgo.RESTError
		if errors.As(err, &restErr) && restErr.Response != nil {
			ga.logger.Warn("create_thread_failed",
				"guild", guildID,
				"channel", channelID,
				"parent_message", parentMessageID,
				"http_status", restErr.Response.StatusCode)
		} else {
			ga.logger.Warn("create_thread_failed",
				"guild", guildID,
				"channel", channelID,
				"parent_message", parentMessageID,
				"err", err)
		}
		return 0, err
	}

	threadID, parseErr := strconv.ParseInt(thread.ID, 10, 64)
	if parseErr != nil {
		ga.logger.Error("failed_to_parse_thread_id",
			"guild", guildID,
			"channel", channelID,
			"thread_id_str", thread.ID)
		return 0, parseErr
	}

	return threadID, nil
}

func (ga *GatewayAdapter) SendMessageToThread(ctx context.Context, threadID int64, content string) (int64, error) {
	threadIDStr := fmt.Sprintf("%d", threadID)

	msg, err := ga.session.ChannelMessageSend(threadIDStr, content)
	if err != nil {
		var restErr *discordgo.RESTError
		if errors.As(err, &restErr) && restErr.Response != nil {
			ga.logger.Warn("send_message_to_thread_failed",
				"thread", threadID,
				"http_status", restErr.Response.StatusCode)
		} else {
			ga.logger.Warn("send_message_to_thread_failed",
				"thread", threadID,
				"err", err)
		}
		return 0, err
	}

	messageID, parseErr := strconv.ParseInt(msg.ID, 10, 64)
	if parseErr != nil {
		ga.logger.Error("failed_to_parse_message_id",
			"thread", threadID,
			"message_id_str", msg.ID)
		return 0, parseErr
	}

	return messageID, nil
}

func (sm *SessionManager) AddAdapter(guildID int64, adapter *GatewayAdapter) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	sm.adapters[guildID] = adapter
}

func (sm *SessionManager) GetAdapter(guildID int64) (*GatewayAdapter, bool) {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	adapter, exists := sm.adapters[guildID]
	return adapter, exists
}

func (sm *SessionManager) RemoveAdapter(guildID int64) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	delete(sm.adapters, guildID)
}

func (sm *SessionManager) CloseAll() error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	for _, adapter := range sm.adapters {
		if err := adapter.Close(); err != nil {
			return err
		}
	}

	return nil
}
