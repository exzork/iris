package obs

import (
	"context"
	"errors"
	"strings"
)

type ErrorClass string

const (
	ErrClassTransient       ErrorClass = "transient"
	ErrClassRateLimited     ErrorClass = "rate_limited"
	ErrClassBadRequest      ErrorClass = "bad_request"
	ErrClassPermissionDenied ErrorClass = "permission_denied"
	ErrClassTimeout         ErrorClass = "timeout"
	ErrClassProvider        ErrorClass = "provider"
	ErrClassInternal        ErrorClass = "internal"
)

// Classify inspects the error chain and returns an ErrorClass.
func Classify(err error) ErrorClass {
	if err == nil {
		return ErrClassInternal
	}

	if errors.Is(err, context.DeadlineExceeded) {
		return ErrClassTimeout
	}

	errStr := err.Error()
	errStrLower := strings.ToLower(errStr)

	if strings.Contains(errStrLower, "rate limit") || strings.Contains(errStrLower, "429") {
		return ErrClassRateLimited
	}

	if strings.Contains(errStrLower, "permission") || strings.Contains(errStrLower, "forbidden") || strings.Contains(errStrLower, "403") {
		return ErrClassPermissionDenied
	}

	if strings.Contains(errStrLower, "bad request") || strings.Contains(errStrLower, "400") || strings.Contains(errStrLower, "invalid") {
		return ErrClassBadRequest
	}

	if strings.Contains(errStrLower, "timeout") {
		return ErrClassTimeout
	}

	if strings.Contains(errStrLower, "provider") || strings.Contains(errStrLower, "upstream") || strings.Contains(errStrLower, "500") || strings.Contains(errStrLower, "502") || strings.Contains(errStrLower, "503") {
		return ErrClassProvider
	}

	return ErrClassInternal
}

// UserFacingMessage returns the Indonesian user-facing message for an ErrorClass.
func UserFacingMessage(class ErrorClass) string {
	switch class {
	case ErrClassTransient:
		return "Layanan sedang sibuk. Silakan coba lagi sebentar."
	case ErrClassRateLimited:
		return "Terlalu banyak permintaan. Mohon tunggu sejenak."
	case ErrClassBadRequest:
		return "Permintaan tidak valid."
	case ErrClassPermissionDenied:
		return "Akses ditolak."
	case ErrClassTimeout:
		return "Permintaan melebihi batas waktu."
	case ErrClassProvider:
		return "Layanan backend sedang mengalami gangguan."
	case ErrClassInternal:
		return "Terjadi kesalahan internal."
	default:
		return "Terjadi kesalahan internal."
	}
}
