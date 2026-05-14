package app

import (
	"context"
	"errors"
	"log/slog"

	"github.com/eko/iris-bot/internal/domain"
	"github.com/eko/iris-bot/internal/llm"
	ragpkg "github.com/eko/iris-bot/internal/lore/rag"
	"github.com/eko/iris-bot/internal/safety"
)

var ErrImageUnavailable = errors.New("image generation unavailable")

type noopMemory struct{}

func (noopMemory) AssemblePromptContext(ctx context.Context, guildID int64, query string) ([]string, error) {
	return nil, nil
}

func (noopMemory) Consider(ctx context.Context, guildID, userID int64, text string) (bool, error) {
	return false, nil
}

type noopLore struct{}

func (noopLore) Compose(ctx context.Context, query string) (*ragpkg.PromptContext, *ragpkg.UnsupportedResponse, error) {
	return nil, nil, nil
}

type App struct {
	Router      TriggerPort
	Memory      MemoryPort
	Lore        LorePort
	LLM         LLMPort
	Image       *ImagePipeline
	Sender      SenderPort
	Safety      *safety.SafetyPipeline
	Responder   *Responder
	PersonaText string
	Logger      *slog.Logger
	TierRouter  *llm.TierRouter
}

type Response struct {
	Sent      bool
	Content   string
	Citations int
	ImageURL  string
	Decision  string
	Reason    string
}

const (
	fallbackIndonesianError = "Terjadi kendala saat memproses permintaan. Silakan coba lagi sebentar."
	fallbackImageError      = "Maaf, gambar tidak dapat dibuat saat ini."
)

func New(
	routerPort TriggerPort,
	memory MemoryPort,
	lore LorePort,
	llm LLMPort,
	image *ImagePipeline,
	sender SenderPort,
	pipeline *safety.SafetyPipeline,
	persona string,
	logger *slog.Logger,
) *App {
	if logger == nil {
		logger = slog.Default()
	}
	if memory == nil {
		memory = noopMemory{}
	}
	if lore == nil {
		lore = noopLore{}
	}
	return &App{
		Router:      routerPort,
		Memory:      memory,
		Lore:        lore,
		LLM:         llm,
		Image:       image,
		Sender:      sender,
		Safety:      pipeline,
		Responder:   NewResponder(pipeline, persona),
		PersonaText: persona,
		Logger:      logger,
	}
}

func (a *App) Handle(ctx context.Context, event *domain.DiscordEvent) (*Response, error) {
	if event == nil || event.Message == nil {
		return &Response{}, nil
	}
	decision, err := a.Router.Decide(ctx, event)
	if err != nil {
		return &Response{Decision: "ignore", Reason: "router_error"}, err
	}
	if !decision.Should {
		return &Response{Decision: "ignore", Reason: string(decision.Reason)}, nil
	}

	query := event.Message.Content
	guildID := event.GuildID
	channelID := event.ChannelID
	userID := event.UserID

	memoryFacts, _ := a.Memory.AssemblePromptContext(ctx, guildID, query)

	var loreCtx *ragpkg.PromptContext
	var unsupported *ragpkg.UnsupportedResponse
	if a.Lore != nil {
		loreCtx, unsupported, _ = a.Lore.Compose(ctx, query)
	}

	imageURL := ""
	imageFailed := false
	if a.Image != nil && DetectIntent(query) {
		res := a.Image.Generate(ctx, query)
		if res.Err == nil {
			imageURL = res.URL
		} else {
			imageFailed = true
			if a.Logger != nil {
				a.Logger.Warn("image generation failed", "reason", res.Err.Error())
			}
		}
	}

	messages := a.Responder.BuildMessages(query, memoryFacts, loreCtx)

	tier := llm.TierDefault
	modelOverride := ""
	if a.TierRouter != nil {
		t, classifyErr := a.TierRouter.Classify(ctx, guildID, query)
		if classifyErr != nil && a.Logger != nil {
			a.Logger.Warn("tier classifier failed", "err", classifyErr.Error())
		}
		tier = t
		modelOverride = a.TierRouter.ModelFor(tier)
	}

	var llmOut string
	var llmErr error
	if modelOverride != "" {
		llmOut, llmErr = a.LLM.ChatWithModel(ctx, modelOverride, guildID, messages)
	} else {
		llmOut, llmErr = a.LLM.Chat(ctx, guildID, messages)
	}
	if llmErr != nil {
		if a.Logger != nil {
			a.Logger.Warn("llm chat failed", "reason", llmErr.Error())
		}
		llmOut = fallbackIndonesianError
	}

	content := llmOut
	if loreCtx != nil && loreCtx.HasSupport {
		content = a.Responder.WithCitations(content, loreCtx.Citations)
	}
	if unsupported != nil && (loreCtx == nil || !loreCtx.HasSupport) {
		content += "\n\n" + unsupported.Message
	}
	if imageURL != "" {
		content += "\n\n[Gambar] " + imageURL
	} else if imageFailed {
		content += "\n\n" + fallbackImageError
	}

	filtered := a.Safety.SanitizeFinalResponse(content)
	if filtered.Blocked {
		return &Response{Sent: false, Decision: "respond", Reason: "blocked_by_safety"}, nil
	}
	if err := a.Sender.Send(ctx, guildID, channelID, filtered.Content); err != nil {
		return &Response{Sent: false, Decision: "respond", Reason: "send_error"}, err
	}

	_, _ = a.Memory.Consider(ctx, guildID, userID, query)

	citations := 0
	if loreCtx != nil {
		citations = len(loreCtx.Citations)
	}
	return &Response{
		Sent:      true,
		Content:   filtered.Content,
		Citations: citations,
		ImageURL:  imageURL,
		Decision:  "respond",
		Reason:    string(decision.Reason),
	}, nil
}
