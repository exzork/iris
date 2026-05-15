package orchestrator

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	mediaCacheDir        = "/tmp/iris-media"
	mediaDownloadMax     = 12 * 1024 * 1024
	mediaDownloadTimeout = 20 * time.Second
	mediaDownloadRetries = 1
)

// MediaFile is a downloaded media payload ready for Discord file attachment.
type MediaFile struct {
	Name        string
	ContentType string
	Bytes       []byte
}

// downloadMediaFiles fetches each URL into the local cache directory and
// returns the bytes plus a sensible Discord filename. Files larger than
// mediaDownloadMax or non-image responses are skipped silently.
func downloadMediaFiles(ctx context.Context, client *http.Client, urls []string) []MediaFile {
	if client == nil {
		client = &http.Client{Timeout: mediaDownloadTimeout}
	}
	if err := os.MkdirAll(mediaCacheDir, 0o755); err != nil {
		slog.WarnContext(ctx, "media_cache_mkdir_failed", "dir", mediaCacheDir, "err", err.Error())
		return nil
	}
	files := make([]MediaFile, 0, len(urls))
	for _, raw := range urls {
		f, err := downloadMediaFile(ctx, client, raw)
		if err != nil {
			slog.WarnContext(ctx, "media_download_skipped", "url", raw, "err", err.Error())
			continue
		}
		slog.DebugContext(ctx, "media_download_ok", "url", raw, "name", f.Name, "bytes", len(f.Bytes))
		files = append(files, f)
	}
	return files
}

func downloadMediaFile(ctx context.Context, client *http.Client, raw string) (MediaFile, error) {
	parsed, err := url.Parse(raw)
	if err != nil {
		return MediaFile{}, err
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return MediaFile{}, errors.New("unsupported scheme")
	}

	hash := sha256.Sum256([]byte(raw))
	cacheKey := hex.EncodeToString(hash[:])

	ext := strings.ToLower(filepath.Ext(parsed.Path))
	if ext == "" || len(ext) > 6 {
		ext = ".gif"
	}
	cachePath := filepath.Join(mediaCacheDir, cacheKey+ext)

	if cached, err := os.ReadFile(cachePath); err == nil && len(cached) > 0 {
		return MediaFile{
			Name:        deriveFilename(parsed, cacheKey, ext),
			ContentType: contentTypeFor(ext),
			Bytes:       cached,
		}, nil
	}

	var lastErr error
	attempts := mediaDownloadRetries + 1
	for attempt := 0; attempt < attempts; attempt++ {
		f, err := fetchMediaOnce(ctx, client, raw, parsed, cacheKey, ext, cachePath)
		if err == nil {
			return f, nil
		}
		lastErr = err
		if errors.Is(err, context.Canceled) {
			break
		}
		if attempt < attempts-1 {
			slog.WarnContext(ctx, "media_download_retry", "url", raw, "attempt", attempt+1, "err", err.Error())
		}
	}
	return MediaFile{}, lastErr
}

func fetchMediaOnce(ctx context.Context, client *http.Client, raw string, parsed *url.URL, cacheKey, ext, cachePath string) (MediaFile, error) {
	start := time.Now()
	reqCtx, cancel := context.WithTimeout(ctx, mediaDownloadTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, raw, nil)
	if err != nil {
		return MediaFile{}, err
	}
	req.Header.Set("Accept", "image/*,*/*;q=0.8")
	req.Header.Set("User-Agent", "iris-bot/1.0")

	resp, err := client.Do(req)
	if err != nil {
		return MediaFile{}, fmt.Errorf("after %s: %w", time.Since(start), err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return MediaFile{}, fmt.Errorf("status %d after %s", resp.StatusCode, time.Since(start))
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, mediaDownloadMax+1))
	if err != nil {
		return MediaFile{}, fmt.Errorf("read after %s: %w", time.Since(start), err)
	}
	if len(body) > mediaDownloadMax {
		return MediaFile{}, errors.New("media too large")
	}
	if len(body) == 0 {
		return MediaFile{}, errors.New("empty body")
	}

	contentType := resp.Header.Get("Content-Type")
	if !looksLikeImage(contentType, ext) {
		return MediaFile{}, fmt.Errorf("not an image: %s", contentType)
	}
	if extFromCT := extForContentType(contentType); extFromCT != "" {
		ext = extFromCT
		cachePath = filepath.Join(mediaCacheDir, cacheKey+ext)
	}

	if err := os.WriteFile(cachePath, body, 0o644); err != nil {
		return MediaFile{}, err
	}
	slog.DebugContext(ctx, "media_download_complete", "url", raw, "bytes", len(body), "elapsed", time.Since(start).String())
	return MediaFile{
		Name:        deriveFilename(parsed, cacheKey, ext),
		ContentType: contentType,
		Bytes:       body,
	}, nil
}

func deriveFilename(u *url.URL, cacheKey, ext string) string {
	base := strings.ToLower(filepath.Base(u.Path))
	if base != "." && base != "/" && base != "" {
		if filepath.Ext(base) == "" {
			base += ext
		}
		return base
	}
	return cacheKey[:12] + ext
}

func contentTypeFor(ext string) string {
	switch ext {
	case ".gif":
		return "image/gif"
	case ".png":
		return "image/png"
	case ".webp":
		return "image/webp"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	default:
		return "application/octet-stream"
	}
}

func looksLikeImage(contentType, ext string) bool {
	if strings.HasPrefix(strings.ToLower(contentType), "image/") {
		return true
	}
	switch strings.ToLower(ext) {
	case ".gif", ".png", ".jpg", ".jpeg", ".webp":
		return true
	}
	return false
}

func extForContentType(contentType string) string {
	ct := strings.ToLower(strings.TrimSpace(strings.SplitN(contentType, ";", 2)[0]))
	switch ct {
	case "image/gif":
		return ".gif"
	case "image/png":
		return ".png"
	case "image/webp":
		return ".webp"
	case "image/jpeg", "image/jpg":
		return ".jpg"
	}
	return ""
}
