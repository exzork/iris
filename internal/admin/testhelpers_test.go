package admin

import (
	"context"
)

type fakeAuditLogger struct {
	logs []struct {
		guildID   int64
		userID    int64
		eventType string
		entityType string
		entityID  string
		changes   map[string]interface{}
	}
}

func newFakeAuditLogger() *fakeAuditLogger {
	return &fakeAuditLogger{}
}

func (f *fakeAuditLogger) Log(ctx context.Context, guildID, userID int64, eventType, entityType, entityID string, changes map[string]interface{}) error {
	f.logs = append(f.logs, struct {
		guildID   int64
		userID    int64
		eventType string
		entityType string
		entityID  string
		changes   map[string]interface{}
	}{guildID, userID, eventType, entityType, entityID, changes})
	return nil
}

type fakeExceptionStore struct {
	channels map[int64]map[int64]bool
	addCalls []struct {
		guildID   int64
		channelID int64
	}
	removeCalls []struct {
		guildID   int64
		channelID int64
	}
}

func newFakeExceptionStore() *fakeExceptionStore {
	return &fakeExceptionStore{
		channels: make(map[int64]map[int64]bool),
	}
}

func (f *fakeExceptionStore) Add(ctx context.Context, guildID, channelID int64) error {
	f.addCalls = append(f.addCalls, struct {
		guildID   int64
		channelID int64
	}{guildID, channelID})

	if f.channels[guildID] == nil {
		f.channels[guildID] = make(map[int64]bool)
	}
	f.channels[guildID][channelID] = true
	return nil
}

func (f *fakeExceptionStore) Remove(ctx context.Context, guildID, channelID int64) error {
	f.removeCalls = append(f.removeCalls, struct {
		guildID   int64
		channelID int64
	}{guildID, channelID})

	if f.channels[guildID] != nil {
		delete(f.channels[guildID], channelID)
	}
	return nil
}

func (f *fakeExceptionStore) List(ctx context.Context, guildID int64) ([]int64, error) {
	var result []int64
	if f.channels[guildID] != nil {
		for ch := range f.channels[guildID] {
			result = append(result, ch)
		}
	}
	return result, nil
}

type fakeSettingsStore struct {
	settings map[int64]map[string]string
	setCalls []struct {
		guildID int64
		key     string
		value   string
	}
	getCalls []struct {
		guildID int64
		key     string
	}
}

func newFakeSettingsStore() *fakeSettingsStore {
	return &fakeSettingsStore{
		settings: make(map[int64]map[string]string),
	}
}

func (f *fakeSettingsStore) Get(ctx context.Context, guildID int64, key string) (string, bool, error) {
	f.getCalls = append(f.getCalls, struct {
		guildID int64
		key     string
	}{guildID, key})

	if f.settings[guildID] == nil {
		return "", false, nil
	}
	val, ok := f.settings[guildID][key]
	return val, ok, nil
}

func (f *fakeSettingsStore) Set(ctx context.Context, guildID int64, key, value string) error {
	f.setCalls = append(f.setCalls, struct {
		guildID int64
		key     string
		value   string
	}{guildID, key, value})

	if f.settings[guildID] == nil {
		f.settings[guildID] = make(map[string]string)
	}
	f.settings[guildID][key] = value
	return nil
}

func (f *fakeSettingsStore) List(ctx context.Context, guildID int64) (map[string]string, error) {
	if f.settings[guildID] == nil {
		return make(map[string]string), nil
	}
	result := make(map[string]string)
	for k, v := range f.settings[guildID] {
		result[k] = v
	}
	return result, nil
}

type fakeAllowedStore struct {
	channels    map[int64]map[int64]bool
	addCalls    []struct {
		guildID   int64
		channelID int64
	}
	removeCalls []struct {
		guildID   int64
		channelID int64
	}
}

func newFakeAllowedStore() *fakeAllowedStore {
	return &fakeAllowedStore{
		channels: make(map[int64]map[int64]bool),
	}
}

func (f *fakeAllowedStore) Add(ctx context.Context, guildID, channelID int64) error {
	f.addCalls = append(f.addCalls, struct {
		guildID   int64
		channelID int64
	}{guildID, channelID})

	if f.channels[guildID] == nil {
		f.channels[guildID] = make(map[int64]bool)
	}
	f.channels[guildID][channelID] = true
	return nil
}

func (f *fakeAllowedStore) Remove(ctx context.Context, guildID, channelID int64) error {
	f.removeCalls = append(f.removeCalls, struct {
		guildID   int64
		channelID int64
	}{guildID, channelID})

	if f.channels[guildID] != nil {
		delete(f.channels[guildID], channelID)
	}
	return nil
}

func (f *fakeAllowedStore) List(ctx context.Context, guildID int64) ([]int64, error) {
	var result []int64
	if f.channels[guildID] != nil {
		for ch := range f.channels[guildID] {
			result = append(result, ch)
		}
	}
	return result, nil
}
