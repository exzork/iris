package llm

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
)

type fakeGlobalStore struct {
	mu     sync.Mutex
	data   map[string]string
	actors map[string]int64
	getErr error
	setErr error
	delErr error
}

func newFakeGlobalStore() *fakeGlobalStore {
	return &fakeGlobalStore{
		data:   map[string]string{},
		actors: map[string]int64{},
	}
}

func (f *fakeGlobalStore) Get(ctx context.Context, key string) (string, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.getErr != nil {
		return "", false, f.getErr
	}
	v, ok := f.data[key]
	return v, ok, nil
}

func (f *fakeGlobalStore) Set(ctx context.Context, key, value string, updatedBy int64) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.setErr != nil {
		return f.setErr
	}
	f.data[key] = value
	f.actors[key] = updatedBy
	return nil
}

func (f *fakeGlobalStore) Delete(ctx context.Context, key string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.delErr != nil {
		return f.delErr
	}
	delete(f.data, key)
	delete(f.actors, key)
	return nil
}

func fakeValidator(m string) error {
	if m == "bad/model" {
		return errors.New("disallowed prefix")
	}
	return nil
}

func TestModelResolverFallsBackWhenNoOverride(t *testing.T) {
	store := newFakeGlobalStore()
	r := NewModelResolver(store, fakeValidator, "fallback-default", "fallback-strong", "fallback-router")
	if err := r.Load(context.Background()); err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if got := r.Default(); got != "fallback-default" {
		t.Errorf("Default = %q, want fallback-default", got)
	}
	if got := r.Strong(); got != "fallback-strong" {
		t.Errorf("Strong = %q, want fallback-strong", got)
	}
	if got := r.Router(); got != "fallback-router" {
		t.Errorf("Router = %q, want fallback-router", got)
	}
}

func TestModelResolverLoadsOverrideFromStore(t *testing.T) {
	store := newFakeGlobalStore()
	store.data[SettingKeyModelDefault] = "kr/override-default"
	store.data[SettingKeyModelStrong] = "kr/override-strong"

	r := NewModelResolver(store, fakeValidator, "fallback-default", "fallback-strong", "fallback-router")
	if err := r.Load(context.Background()); err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if got := r.Default(); got != "kr/override-default" {
		t.Errorf("Default = %q, want kr/override-default", got)
	}
	if got := r.Strong(); got != "kr/override-strong" {
		t.Errorf("Strong = %q, want kr/override-strong", got)
	}
	if got := r.Router(); got != "fallback-router" {
		t.Errorf("Router = %q, want fallback-router", got)
	}
}

func TestModelResolverSetOverridePersistsAndCaches(t *testing.T) {
	store := newFakeGlobalStore()
	r := NewModelResolver(store, fakeValidator, "fallback-default", "fallback-strong", "fallback-router")

	if err := r.SetOverride(context.Background(), ModelTierDefault, "kr/new-default", 42); err != nil {
		t.Fatalf("SetOverride failed: %v", err)
	}
	if got := r.Default(); got != "kr/new-default" {
		t.Errorf("cache miss: Default = %q", got)
	}
	if got := store.data[SettingKeyModelDefault]; got != "kr/new-default" {
		t.Errorf("store miss: got %q", got)
	}
	if got := store.actors[SettingKeyModelDefault]; got != 42 {
		t.Errorf("actor not recorded: got %d", got)
	}
}

func TestModelResolverSetOverrideRejectsInvalid(t *testing.T) {
	store := newFakeGlobalStore()
	r := NewModelResolver(store, fakeValidator, "fallback-default", "fallback-strong", "fallback-router")

	err := r.SetOverride(context.Background(), ModelTierDefault, "bad/model", 7)
	if err == nil {
		t.Fatal("expected validation error, got nil")
	}
	if len(store.data) != 0 {
		t.Errorf("store should not be mutated on validation failure: %v", store.data)
	}
	if got := r.Default(); got != "fallback-default" {
		t.Errorf("cache should be untouched: got %q", got)
	}
}

func TestModelResolverResetOverrideRevertsCache(t *testing.T) {
	store := newFakeGlobalStore()
	r := NewModelResolver(store, fakeValidator, "fallback-default", "fallback-strong", "fallback-router")
	if err := r.SetOverride(context.Background(), ModelTierStrong, "kr/new-strong", 9); err != nil {
		t.Fatalf("SetOverride: %v", err)
	}
	if err := r.ResetOverride(context.Background(), ModelTierStrong); err != nil {
		t.Fatalf("ResetOverride: %v", err)
	}
	if got := r.Strong(); got != "fallback-strong" {
		t.Errorf("after reset Strong = %q, want fallback-strong", got)
	}
	if _, ok := store.data[SettingKeyModelStrong]; ok {
		t.Errorf("store key not deleted: %v", store.data)
	}
}

func TestModelResolverSetOverridePropagatesStoreError(t *testing.T) {
	store := newFakeGlobalStore()
	store.setErr = fmt.Errorf("boom")
	r := NewModelResolver(store, fakeValidator, "fallback-default", "fallback-strong", "fallback-router")

	if err := r.SetOverride(context.Background(), ModelTierDefault, "kr/ok", 1); err == nil {
		t.Fatal("expected error")
	}
	if got := r.Default(); got != "fallback-default" {
		t.Errorf("cache leaked through failed set: %q", got)
	}
}

func TestParseTier(t *testing.T) {
	cases := map[string]ModelTier{
		"default":  ModelTierDefault,
		"standard": ModelTierDefault,
		"strong":   ModelTierStrong,
		"heavy":    ModelTierStrong,
		"router":   ModelTierRouter,
	}
	for in, want := range cases {
		got, err := ParseTier(in)
		if err != nil {
			t.Errorf("ParseTier(%q) error: %v", in, err)
			continue
		}
		if got != want {
			t.Errorf("ParseTier(%q) = %q, want %q", in, got, want)
		}
	}
	if _, err := ParseTier("garbage"); err == nil {
		t.Error("expected ParseTier(\"garbage\") error")
	}
}

func TestModelResolverConcurrentReadsAndWrites(t *testing.T) {
	store := newFakeGlobalStore()
	r := NewModelResolver(store, fakeValidator, "fallback-default", "fallback-strong", "fallback-router")
	var wg sync.WaitGroup
	stop := make(chan struct{})

	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
					_ = r.Default()
					_ = r.Strong()
				}
			}
		}()
	}

	for i := 0; i < 50; i++ {
		if err := r.SetOverride(context.Background(), ModelTierDefault, fmt.Sprintf("kr/m-%d", i), int64(i)); err != nil {
			t.Fatalf("SetOverride: %v", err)
		}
	}
	close(stop)
	wg.Wait()
}
