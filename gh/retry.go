package gh

import (
	"context"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/google/go-github/v67/github"
	"golang.org/x/exp/slog"
)

const maxRetries = 3

var (
	retryBaseInterval = 1 * time.Second
	retryMaxInterval  = 16 * time.Second
)

// shouldRetry reports whether a request is worth retrying. status is 0 when no response was received.
// 4xx other than 408/429 are not retried so that callers can fall back (e.g. to the no-auth client) quickly.
func shouldRetry(ctx context.Context, status int, err error) bool {
	if ctx.Err() != nil {
		return false
	}
	if status == 0 {
		return err != nil
	}
	return status == http.StatusRequestTimeout || status == http.StatusTooManyRequests || status >= http.StatusInternalServerError
}

// retryDelay returns how long to wait before the next attempt, honoring Retry-After (RFC 9110 Section 10.2.3).
// When the server asks to wait longer than retryMaxInterval, it returns false instead of waiting less than asked,
// because retrying earlier than requested only extends rate limiting.
func retryDelay(attempt int, h http.Header) (time.Duration, bool) {
	d := min(retryBaseInterval<<attempt, retryMaxInterval)
	ra, ok := parseRetryAfter(h.Get("Retry-After"))
	if !ok {
		return d, true
	}
	if ra > retryMaxInterval {
		return 0, false
	}
	return max(d, ra), true
}

func parseRetryAfter(v string) (time.Duration, bool) {
	if v == "" {
		return 0, false
	}
	if sec, err := strconv.Atoi(v); err == nil {
		if sec < 0 {
			return 0, false
		}
		return time.Duration(sec) * time.Second, true
	}
	if t, err := http.ParseTime(v); err == nil {
		return max(time.Until(t), 0), true
	}
	return 0, false
}

func waitRetry(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

func (c *client) do(req *http.Request) (*http.Response, error) {
	ctx := req.Context()
	for attempt := 0; ; attempt++ {
		resp, err := c.hc.Do(req.Clone(ctx))
		status := 0
		if resp != nil {
			status = resp.StatusCode
		}
		if attempt >= maxRetries || !shouldRetry(ctx, status, err) {
			return resp, err
		}
		var h http.Header
		if resp != nil {
			h = resp.Header
		}
		d, ok := retryDelay(attempt, h)
		if !ok {
			return resp, err
		}
		if resp != nil {
			_, _ = io.Copy(io.Discard, resp.Body)
			_ = resp.Body.Close()
		}
		slog.Info("Retry request", slog.String("url", req.URL.String()), slog.Int("status", status), slog.Int("attempt", attempt+1), slog.Duration("delay", d))
		if err := waitRetry(ctx, d); err != nil {
			return nil, err
		}
	}
}

func retryGitHub[T any](ctx context.Context, f func() (T, *github.Response, error)) (T, *github.Response, error) {
	for attempt := 0; ; attempt++ {
		v, resp, err := f()
		if err == nil {
			return v, resp, nil
		}
		status := 0
		var h http.Header
		if resp != nil && resp.Response != nil {
			status = resp.StatusCode
			h = resp.Header
		}
		if attempt >= maxRetries || !shouldRetry(ctx, status, err) {
			return v, resp, err
		}
		d, ok := retryDelay(attempt, h)
		if !ok {
			return v, resp, err
		}
		slog.Info("Retry GitHub API request", slog.Int("status", status), slog.Int("attempt", attempt+1), slog.Duration("delay", d), slog.String("error", err.Error()))
		if err := waitRetry(ctx, d); err != nil {
			return v, resp, err
		}
	}
}
