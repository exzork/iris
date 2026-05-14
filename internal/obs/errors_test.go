package obs

import (
	"context"
	"errors"
	"testing"
)

func TestClassifyTimeout(t *testing.T) {
	class := Classify(context.DeadlineExceeded)
	if class != ErrClassTimeout {
		t.Errorf("expected ErrClassTimeout, got %s", class)
	}
}

func TestClassifyRateLimited(t *testing.T) {
	err := errors.New("rate limit exceeded")
	class := Classify(err)
	if class != ErrClassRateLimited {
		t.Errorf("expected ErrClassRateLimited, got %s", class)
	}
}

func TestClassifyDefault(t *testing.T) {
	err := errors.New("some generic error")
	class := Classify(err)
	if class != ErrClassInternal {
		t.Errorf("expected ErrClassInternal, got %s", class)
	}
}

func TestClassifyBadRequest(t *testing.T) {
	err := errors.New("bad request: invalid input")
	class := Classify(err)
	if class != ErrClassBadRequest {
		t.Errorf("expected ErrClassBadRequest, got %s", class)
	}
}

func TestClassifyPermissionDenied(t *testing.T) {
	err := errors.New("permission denied")
	class := Classify(err)
	if class != ErrClassPermissionDenied {
		t.Errorf("expected ErrClassPermissionDenied, got %s", class)
	}
}

func TestClassifyProvider(t *testing.T) {
	err := errors.New("upstream provider error 503")
	class := Classify(err)
	if class != ErrClassProvider {
		t.Errorf("expected ErrClassProvider, got %s", class)
	}
}

func TestUserFacingMessageIndonesian(t *testing.T) {
	tests := []struct {
		class   ErrorClass
		want    string
	}{
		{ErrClassTransient, "Layanan sedang sibuk. Silakan coba lagi sebentar."},
		{ErrClassRateLimited, "Terlalu banyak permintaan. Mohon tunggu sejenak."},
		{ErrClassBadRequest, "Permintaan tidak valid."},
		{ErrClassPermissionDenied, "Akses ditolak."},
		{ErrClassTimeout, "Permintaan melebihi batas waktu."},
		{ErrClassProvider, "Layanan backend sedang mengalami gangguan."},
		{ErrClassInternal, "Terjadi kesalahan internal."},
	}

	for _, tt := range tests {
		t.Run(string(tt.class), func(t *testing.T) {
			msg := UserFacingMessage(tt.class)
			if msg != tt.want {
				t.Errorf("expected %q, got %q", tt.want, msg)
			}
		})
	}
}
