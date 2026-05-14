package llm

import (
	"context"
	"fmt"
	"sync"
)

const (
	SettingKeyModelDefault = "active_model_default"
	SettingKeyModelStrong  = "active_model_strong"
	SettingKeyModelRouter  = "active_model_router"
)

type GlobalSettingsStore interface {
	Get(ctx context.Context, key string) (value string, found bool, err error)
	Set(ctx context.Context, key, value string, updatedBy int64) error
	Delete(ctx context.Context, key string) error
}

type ModelValidator func(model string) error

// ModelResolver resolves the live tier model names, preferring overrides in
// the global_settings store over the env-var fallbacks captured at boot.
// Safe for concurrent use. Overrides are cached in-memory and refreshed
// explicitly via Set/Reset; readers never hit the DB.
type ModelResolver struct {
	store        GlobalSettingsStore
	validate     ModelValidator
	fallbackDef  string
	fallbackStr  string
	fallbackRtr  string

	mu            sync.RWMutex
	overrideDef   string
	overrideStr   string
	overrideRtr   string
}

func NewModelResolver(store GlobalSettingsStore, validate ModelValidator, fallbackDefault, fallbackStrong, fallbackRouter string) *ModelResolver {
	if validate == nil {
		validate = func(string) error { return nil }
	}
	return &ModelResolver{
		store:       store,
		validate:    validate,
		fallbackDef: fallbackDefault,
		fallbackStr: fallbackStrong,
		fallbackRtr: fallbackRouter,
	}
}

// Load hydrates the in-memory overrides from the backing store. Call once
// at startup after the DB connection is ready. Missing rows leave the
// fallback in place. Errors are returned so the caller can decide whether
// to fail-fast or log-and-continue.
func (r *ModelResolver) Load(ctx context.Context) error {
	if r.store == nil {
		return nil
	}
	def, _, err := r.store.Get(ctx, SettingKeyModelDefault)
	if err != nil {
		return fmt.Errorf("load default model override: %w", err)
	}
	str, _, err := r.store.Get(ctx, SettingKeyModelStrong)
	if err != nil {
		return fmt.Errorf("load strong model override: %w", err)
	}
	rtr, _, err := r.store.Get(ctx, SettingKeyModelRouter)
	if err != nil {
		return fmt.Errorf("load router model override: %w", err)
	}

	r.mu.Lock()
	r.overrideDef = def
	r.overrideStr = str
	r.overrideRtr = rtr
	r.mu.Unlock()
	return nil
}

func (r *ModelResolver) Default() string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.overrideDef != "" {
		return r.overrideDef
	}
	return r.fallbackDef
}

func (r *ModelResolver) Strong() string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.overrideStr != "" {
		return r.overrideStr
	}
	return r.fallbackStr
}

func (r *ModelResolver) Router() string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.overrideRtr != "" {
		return r.overrideRtr
	}
	return r.fallbackRtr
}

type ModelTier string

const (
	ModelTierDefault ModelTier = "default"
	ModelTierStrong  ModelTier = "strong"
	ModelTierRouter  ModelTier = "router"
)

func (r *ModelResolver) Fallback(tier ModelTier) string {
	switch tier {
	case ModelTierStrong:
		return r.fallbackStr
	case ModelTierRouter:
		return r.fallbackRtr
	default:
		return r.fallbackDef
	}
}

// SetOverride validates the model, persists it to the store, and updates
// the in-memory cache atomically. The actor id is forwarded to the store
// for audit purposes.
func (r *ModelResolver) SetOverride(ctx context.Context, tier ModelTier, model string, actor int64) error {
	if r.store == nil {
		return fmt.Errorf("model override store not configured")
	}
	if err := r.validate(model); err != nil {
		return fmt.Errorf("invalid model %q: %w", model, err)
	}
	key, err := settingKeyFor(tier)
	if err != nil {
		return err
	}
	if err := r.store.Set(ctx, key, model, actor); err != nil {
		return err
	}
	r.writeCache(tier, model)
	return nil
}

// ResetOverride removes the persisted override for the given tier, reverting
// subsequent reads to the boot-time fallback.
func (r *ModelResolver) ResetOverride(ctx context.Context, tier ModelTier) error {
	if r.store == nil {
		return fmt.Errorf("model override store not configured")
	}
	key, err := settingKeyFor(tier)
	if err != nil {
		return err
	}
	if err := r.store.Delete(ctx, key); err != nil {
		return err
	}
	r.writeCache(tier, "")
	return nil
}

func (r *ModelResolver) writeCache(tier ModelTier, value string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	switch tier {
	case ModelTierStrong:
		r.overrideStr = value
	case ModelTierRouter:
		r.overrideRtr = value
	default:
		r.overrideDef = value
	}
}

func settingKeyFor(tier ModelTier) (string, error) {
	switch tier {
	case ModelTierDefault:
		return SettingKeyModelDefault, nil
	case ModelTierStrong:
		return SettingKeyModelStrong, nil
	case ModelTierRouter:
		return SettingKeyModelRouter, nil
	default:
		return "", fmt.Errorf("unknown tier %q", tier)
	}
}

// ParseTier maps user-facing tier names to typed values. Accepts the
// canonical forms ("default", "strong", "router") as well as common
// aliases.
func ParseTier(s string) (ModelTier, error) {
	switch s {
	case "default", "def", "standard":
		return ModelTierDefault, nil
	case "strong", "heavy", "complex":
		return ModelTierStrong, nil
	case "router", "classifier":
		return ModelTierRouter, nil
	default:
		return "", fmt.Errorf("unknown tier %q (expected default|strong|router)", s)
	}
}
