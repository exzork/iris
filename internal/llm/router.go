package llm

import (
	"context"
	"fmt"
	"strings"
)

type Tier string

const (
	TierDefault Tier = "default"
	TierStrong  Tier = "strong"
)

type TierRouter struct {
	Classifier ChatClient
	Router     string
	Default    string
	Strong     string
	Resolver   *ModelResolver
}

type ChatClient interface {
	ChatWithModel(ctx context.Context, model string, guildID int64, messages []map[string]string) (string, error)
}

// Classify returns the tier decision for the given user query.
// Returns TierDefault for empty queries without calling the classifier.
// If classification fails, returns TierDefault with a logged error.
func (r *TierRouter) Classify(ctx context.Context, guildID int64, userQuery string) (Tier, error) {
	if strings.TrimSpace(userQuery) == "" {
		return TierDefault, nil
	}

	sys := "Anda adalah router. Balas HANYA dengan satu kata: DEFAULT atau STRONG.\n" +
		"STRONG jika pertanyaan membutuhkan: lore Wuthering Waves kompleks/mendalam, perbandingan multi-karakter/build, analisis teori kanon panjang, debugging kode/matematika panjang, rangkuman/proses banyak langkah. \n" +
		"DEFAULT untuk semua pertanyaan ringan, sapaan, perintah utilitas, pertanyaan singkat, meme, atau obrolan pendek.\n" +
		"Jawab SATU kata saja."

	messages := []map[string]string{
		{"role": "system", "content": sys},
		{"role": "user", "content": userQuery},
	}

	out, err := r.Classifier.ChatWithModel(ctx, r.routerModel(), guildID, messages)
	if err != nil {
		return TierDefault, fmt.Errorf("classifier failed: %w", err)
	}

	upper := strings.ToUpper(strings.TrimSpace(out))
	if strings.HasPrefix(upper, "STRONG") {
		return TierStrong, nil
	}

	return TierDefault, nil
}

// ModelFor returns the model ID for the given tier, consulting the resolver
// override first when configured.
func (r *TierRouter) ModelFor(tier Tier) string {
	if tier == TierStrong {
		return r.strongModel()
	}
	return r.defaultModel()
}

func (r *TierRouter) routerModel() string {
	if r.Resolver != nil {
		return r.Resolver.Router()
	}
	return r.Router
}

func (r *TierRouter) defaultModel() string {
	if r.Resolver != nil {
		return r.Resolver.Default()
	}
	return r.Default
}

func (r *TierRouter) strongModel() string {
	if r.Resolver != nil {
		return r.Resolver.Strong()
	}
	return r.Strong
}
