package wire

import (
	"context"
	"fmt"

	"github.com/eko/iris-bot/internal/llm"
	"github.com/eko/iris-bot/internal/persona"
)

type SynthesizerAdapter struct {
	Client *llm.Client
	Model  string
}

func (a *SynthesizerAdapter) Synthesize(ctx context.Context, guildID int64, toolName, userQuery, toolOutput string) (string, error) {
	if a.Client == nil {
		return "", nil
	}

	system := persona.BuildSystemPrompt(persona.PromptInput{})

	userPrompt := fmt.Sprintf(
		"User memanggil slash command yang di belakangnya jalan tool %q.\n"+
			"Query / argumen user: %s\n\n"+
			"Ini hasil mentah dari tool (JSON atau teks). Ringkas ke Bahasa Indonesia santai sesuai persona di atas. "+
			"JANGAN tempel JSON atau struktur mentah. Kalau hasil kosong, bilang gak ada hasilnya.\n\n"+
			"---BEGIN TOOL OUTPUT---\n%s\n---END TOOL OUTPUT---",
		toolName, userQuery, toolOutput,
	)

	messages := []map[string]string{
		{"role": "system", "content": system},
		{"role": "user", "content": userPrompt},
	}

	model := a.Model
	if model == "" {
		return a.Client.Chat(ctx, guildID, messages)
	}
	return a.Client.ChatWithModel(ctx, model, guildID, messages)
}
