package gh

import (
	"context"
	"io"
	"net/http"
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

func waitRetry(ctx context.Context, attempt int) error {
	d := min(retryBaseInterval<<attempt, retryMaxInterval)
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
		if resp != nil {
			_, _ = io.Copy(io.Discard, resp.Body)
			_ = resp.Body.Close()
		}
		slog.Info("Retry request", slog.String("url", req.URL.String()), slog.Int("status", status), slog.Int("attempt", attempt+1))
		if err := waitRetry(ctx, attempt); err != nil {
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
		if resp != nil {
			status = resp.StatusCode
		}
		if attempt >= maxRetries || !shouldRetry(ctx, status, err) {
			return v, resp, err
		}
		slog.Info("Retry GitHub API request", slog.Int("status", status), slog.Int("attempt", attempt+1), slog.String("error", err.Error()))
		if err := waitRetry(ctx, attempt); err != nil {
			return v, resp, err
		}
	}
}
