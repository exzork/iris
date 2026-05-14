package browser

import (
	"testing"
	"time"
)

func TestLimiterBurstThenDeny(t *testing.T) {
	limiter := NewLimiter(1*time.Hour, 2)

	if !limiter.Allow("host1") {
		t.Error("first token should be allowed")
	}
	if !limiter.Allow("host1") {
		t.Error("second token should be allowed")
	}
	if limiter.Allow("host1") {
		t.Error("third token should be denied")
	}
}

func TestLimiterResetAfterInterval(t *testing.T) {
	limiter := NewLimiter(10*time.Millisecond, 1)

	if !limiter.Allow("host1") {
		t.Error("first token should be allowed")
	}
	if limiter.Allow("host1") {
		t.Error("second token should be denied")
	}

	time.Sleep(15 * time.Millisecond)

	if !limiter.Allow("host1") {
		t.Error("token should be allowed after interval reset")
	}
}

func TestLimiterIsolatesHosts(t *testing.T) {
	limiter := NewLimiter(1*time.Hour, 1)

	if !limiter.Allow("host1") {
		t.Error("host1 first token should be allowed")
	}
	if limiter.Allow("host1") {
		t.Error("host1 second token should be denied")
	}

	if !limiter.Allow("host2") {
		t.Error("host2 first token should be allowed")
	}
	if limiter.Allow("host2") {
		t.Error("host2 second token should be denied")
	}
}
