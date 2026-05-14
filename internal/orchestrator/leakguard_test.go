package orchestrator

import (
	"strings"
	"testing"
)

func TestScrubRawUserIDs_WrapsKnownIDInMention(t *testing.T) {
	known := map[int64]bool{123456789012345678: true}

	cases := []struct {
		name string
		in   string
		want string
	}{
		{"raw id at start", "123456789012345678 said hi", "<@123456789012345678> said hi"},
		{"raw id mid sentence", "user 123456789012345678 said hi", "user <@123456789012345678> said hi"},
		{"raw id at end", "ping 123456789012345678", "ping <@123456789012345678>"},
		{"already mentioned stays unchanged", "ping <@123456789012345678>", "ping <@123456789012345678>"},
		{"legacy nick mention stays unchanged", "ping <@!123456789012345678>", "ping <@!123456789012345678>"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := scrubRawUserIDs(tc.in, known)
			if got != tc.want {
				t.Errorf("scrubRawUserIDs(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestScrubRawUserIDs_UnknownIDPassesThrough(t *testing.T) {
	known := map[int64]bool{111111111111111111: true}
	in := "version 999999999999999999 released"
	got := scrubRawUserIDs(in, known)
	if got != in {
		t.Errorf("unknown long-digit run should pass through, got %q", got)
	}
}

func TestScrubRawUserIDs_MultipleKnownUsers(t *testing.T) {
	known := map[int64]bool{
		111111111111111111: true,
		222222222222222222: true,
	}
	in := "111111111111111111 and 222222222222222222 talked"
	want := "<@111111111111111111> and <@222222222222222222> talked"
	got := scrubRawUserIDs(in, known)
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestScrubRawUserIDs_PreservesExplicitMention(t *testing.T) {
	known := map[int64]bool{123456789012345678: true}
	in := "please tag <@123456789012345678> and 123456789012345678"
	want := "please tag <@123456789012345678> and <@123456789012345678>"
	got := scrubRawUserIDs(in, known)
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestScrubRawUserIDs_EmptyOrNoKnown(t *testing.T) {
	if got := scrubRawUserIDs("", map[int64]bool{1: true}); got != "" {
		t.Errorf("empty input should stay empty, got %q", got)
	}
	in := "111111111111111111 says hi"
	if got := scrubRawUserIDs(in, nil); got != in {
		t.Errorf("nil known set should pass through, got %q", got)
	}
	if got := scrubRawUserIDs(in, map[int64]bool{}); got != in {
		t.Errorf("empty known set should pass through, got %q", got)
	}
}

func TestCollectKnownUserIDs_FromTriggerAndContextLabels(t *testing.T) {
	history := []map[string]string{
		{"role": "user", "content": "alice (user id: 111111111111111111): hello"},
		{"role": "user", "content": "bob (user id: 222222222222222222): hi"},
		{"role": "assistant", "content": "iris reply"},
	}
	got := collectKnownUserIDs(333333333333333333, history)

	for _, want := range []int64{111111111111111111, 222222222222222222, 333333333333333333} {
		if !got[want] {
			t.Errorf("expected id %d in known set, got %v", want, got)
		}
	}
}

func TestCollectKnownUserIDs_TaggedFormatLabels(t *testing.T) {
	history := []map[string]string{
		{"role": "user", "content": "[ALLOWED-CHANNELS]\n<general|-|alice|111111111111111111|2026-01-01T00:00:00Z|hi>\n<general|-|bob|222222222222222222|2026-01-01T00:00:01Z|yo>"},
	}
	got := collectKnownUserIDs(0, history)
	if !got[111111111111111111] {
		t.Errorf("expected id 111111111111111111 from tagged format, got %v", got)
	}
	if !got[222222222222222222] {
		t.Errorf("expected id 222222222222222222 from tagged format, got %v", got)
	}
}

func TestScrubRawUserIDs_DoesNotCorruptShorterNumbers(t *testing.T) {
	known := map[int64]bool{}
	in := "the year is 2026 and 1234 fish swam"
	got := scrubRawUserIDs(in, known)
	if got != in {
		t.Errorf("short numbers must not be touched, got %q", got)
	}
	if strings.Contains(got, "<@") {
		t.Errorf("no mention syntax should appear, got %q", got)
	}
}

func TestReplacePlainUsernamesWithMentions_BasicPromotion(t *testing.T) {
	usernameToID := map[string]int64{"eko": 111111111111111111}
	in := "hi eko, check this"
	want := "hi <@111111111111111111>, check this"
	got := replacePlainUsernamesWithMentions(in, usernameToID)
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestReplacePlainUsernamesWithMentions_WordBoundary(t *testing.T) {
	usernameToID := map[string]int64{"eko": 111111111111111111}
	in := "ekosystem is great"
	got := replacePlainUsernamesWithMentions(in, usernameToID)
	if got != in {
		t.Errorf("should not match inside larger word, got %q", got)
	}
}

func TestReplacePlainUsernamesWithMentions_AlreadyMention(t *testing.T) {
	usernameToID := map[string]int64{"eko": 111111111111111111}
	in := "hi <@111111111111111111>"
	got := replacePlainUsernamesWithMentions(in, usernameToID)
	if got != in {
		t.Errorf("should not modify existing mention, got %q", got)
	}
}

func TestReplacePlainUsernamesWithMentions_InsideLabel(t *testing.T) {
	usernameToID := map[string]int64{"alice": 222222222222222222}
	in := "alice (user id: 222222222222222222) says hi"
	got := replacePlainUsernamesWithMentions(in, usernameToID)
	if got != in {
		t.Errorf("should not promote username inside label format, got %q", got)
	}
}

func TestReplacePlainUsernamesWithMentions_InsideInlineCode(t *testing.T) {
	usernameToID := map[string]int64{"eko": 111111111111111111}
	in := "use `eko` as the variable"
	got := replacePlainUsernamesWithMentions(in, usernameToID)
	if got != in {
		t.Errorf("should not promote username inside inline code, got %q", got)
	}
}

func TestReplacePlainUsernamesWithMentions_InsideFencedCode(t *testing.T) {
	usernameToID := map[string]int64{"eko": 111111111111111111}
	in := "```\necho eko\n```"
	got := replacePlainUsernamesWithMentions(in, usernameToID)
	if got != in {
		t.Errorf("should not promote username inside fenced code block, got %q", got)
	}
}

func TestReplacePlainUsernamesWithMentions_MultipleUsers(t *testing.T) {
	usernameToID := map[string]int64{
		"alice": 111111111111111111,
		"bob":   222222222222222222,
	}
	in := "alice and bob talked"
	want := "<@111111111111111111> and <@222222222222222222> talked"
	got := replacePlainUsernamesWithMentions(in, usernameToID)
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestReplacePlainUsernamesWithMentions_StopWordDenylist(t *testing.T) {
	usernameToID := map[string]int64{"me": 111111111111111111}
	in := "tell me about this"
	got := replacePlainUsernamesWithMentions(in, usernameToID)
	if got != in {
		t.Errorf("should not promote stop word 'me', got %q", got)
	}
}

func TestReplacePlainUsernamesWithMentions_TooShort(t *testing.T) {
	usernameToID := map[string]int64{"ab": 111111111111111111}
	in := "ab is too short"
	got := replacePlainUsernamesWithMentions(in, usernameToID)
	if got != in {
		t.Errorf("should not promote username shorter than 3 chars, got %q", got)
	}
}

func TestReplacePlainUsernamesWithMentions_CaseInsensitive(t *testing.T) {
	usernameToID := map[string]int64{"eko": 111111111111111111}
	in := "hi Eko and EKO"
	want := "hi <@111111111111111111> and <@111111111111111111>"
	got := replacePlainUsernamesWithMentions(in, usernameToID)
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestReplacePlainUsernamesWithMentions_Idempotent(t *testing.T) {
	usernameToID := map[string]int64{"eko": 111111111111111111}
	in := "hi eko"
	once := replacePlainUsernamesWithMentions(in, usernameToID)
	twice := replacePlainUsernamesWithMentions(once, usernameToID)
	if once != twice {
		t.Errorf("should be idempotent: once=%q, twice=%q", once, twice)
	}
}

func TestCollectKnownUserMap_ExtractsUsernames(t *testing.T) {
	history := []map[string]string{
		{"role": "user", "content": "alice (user id: 111111111111111111): hello"},
		{"role": "user", "content": "bob (user id: 222222222222222222): hi"},
	}
	knownIDs, usernameToID := collectKnownUserMap(333333333333333333, "charlie", history)

	if !knownIDs[111111111111111111] {
		t.Errorf("expected id 111111111111111111 in known set")
	}
	if !knownIDs[222222222222222222] {
		t.Errorf("expected id 222222222222222222 in known set")
	}
	if !knownIDs[333333333333333333] {
		t.Errorf("expected trigger id 333333333333333333 in known set")
	}

	if usernameToID["alice"] != 111111111111111111 {
		t.Errorf("expected alice->111111111111111111 mapping")
	}
	if usernameToID["bob"] != 222222222222222222 {
		t.Errorf("expected bob->222222222222222222 mapping")
	}
	if usernameToID["charlie"] != 333333333333333333 {
		t.Errorf("expected charlie->333333333333333333 mapping")
	}
}

func TestCollectKnownUserMap_TaggedFormat(t *testing.T) {
	history := []map[string]string{
		{"role": "user", "content": "[ALLOWED-CHANNELS]\n<general|-|alice|111111111111111111|2026-01-01T00:00:00Z|hi>\n<general|-|bob|222222222222222222|2026-01-01T00:00:01Z|yo>"},
	}
	_, usernameToID := collectKnownUserMap(0, "", history)

	if usernameToID["alice"] != 111111111111111111 {
		t.Errorf("expected alice from tagged format")
	}
	if usernameToID["bob"] != 222222222222222222 {
		t.Errorf("expected bob from tagged format")
	}
}

func TestScrubOutbound_CombinesBothPasses(t *testing.T) {
	knownIDs := map[int64]bool{111111111111111111: true}
	usernameToID := map[string]int64{"eko": 111111111111111111}

	in := "raw id 111111111111111111 and username eko"
	got := scrubOutbound(in, knownIDs, usernameToID)

	if !strings.Contains(got, "<@111111111111111111>") {
		t.Errorf("should convert raw id to mention, got %q", got)
	}
	if strings.Contains(got, " eko") {
		t.Errorf("should convert username to mention, got %q", got)
	}
}

func TestScrubOutbound_Idempotent(t *testing.T) {
	knownIDs := map[int64]bool{111111111111111111: true}
	usernameToID := map[string]int64{"eko": 111111111111111111}

	in := "raw id 111111111111111111 and username eko"
	once := scrubOutbound(in, knownIDs, usernameToID)
	twice := scrubOutbound(once, knownIDs, usernameToID)

	if once != twice {
		t.Errorf("should be idempotent: once=%q, twice=%q", once, twice)
	}
}
