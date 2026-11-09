package gh

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/go-github/v67/github"
)

func shortenRetryInterval(t *testing.T) {
	t.Helper()
	base, maxi := retryBaseInterval, retryMaxInterval
	retryBaseInterval, retryMaxInterval = time.Millisecond, time.Millisecond
	t.Cleanup(func() {
		retryBaseInterval, retryMaxInterval = base, maxi
	})
}

func TestClientDo(t *testing.T) {
	shortenRetryInterval(t)
	tests := []struct {
		name         string
		failStatus   int
		failCount    int
		wantStatus   int
		wantRequests int32
	}{
		{"recover from 504", http.StatusGatewayTimeout, 2, http.StatusOK, 3},
		{"recover from 502", http.StatusBadGateway, 1, http.StatusOK, 2},
		{"recover from 429", http.StatusTooManyRequests, 1, http.StatusOK, 2},
		{"give up after max retries", http.StatusGatewayTimeout, maxRetries + 10, http.StatusGatewayTimeout, maxRetries + 1},
		{"do not retry 404", http.StatusNotFound, 1, http.StatusNotFound, 1},
		{"do not retry 401", http.StatusUnauthorized, 1, http.StatusUnauthorized, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var requests atomic.Int32
			ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if int(requests.Add(1)) <= tt.failCount {
					w.WriteHeader(tt.failStatus)
					_, _ = w.Write([]byte("<html><body><h1>error</h1></body></html>"))
					return
				}
				_, _ = w.Write([]byte("ok"))
			}))
			t.Cleanup(ts.Close)
			c := &client{hc: ts.Client()}
			req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, ts.URL, nil)
			if err != nil {
				t.Fatal(err)
			}
			resp, err := c.do(req)
			if err != nil {
				t.Fatal(err)
			}
			defer resp.Body.Close()
			if resp.StatusCode != tt.wantStatus {
				t.Errorf("got status %d, want %d", resp.StatusCode, tt.wantStatus)
			}
			if tt.wantStatus == http.StatusOK {
				b, err := io.ReadAll(resp.Body)
				if err != nil {
					t.Fatal(err)
				}
				if string(b) != "ok" {
					t.Errorf("got body %q, want %q", string(b), "ok")
				}
			}
			if got := requests.Load(); got != tt.wantRequests {
				t.Errorf("got %d requests, want %d", got, tt.wantRequests)
			}
		})
	}
}

func TestClientDoStopsOnContextCancel(t *testing.T) {
	shortenRetryInterval(t)
	var requests atomic.Int32
	ctx, cancel := context.WithCancel(context.Background())
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		cancel()
		w.WriteHeader(http.StatusGatewayTimeout)
	}))
	t.Cleanup(ts.Close)
	c := &client{hc: ts.Client()}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, ts.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := c.do(req)
	if err == nil {
		resp.Body.Close()
	}
	if got := requests.Load(); got != 1 {
		t.Errorf("got %d requests, want 1", got)
	}
}

func TestRetryGitHub(t *testing.T) {
	shortenRetryInterval(t)
	errAPI := errors.New("api error")
	tests := []struct {
		name      string
		results   []int // status per call; 0 means a transport error without response, 200 means success
		wantErr   bool
		wantCalls int
	}{
		{"recover from 504", []int{504, 504, 200}, false, 3},
		{"recover from transport error", []int{0, 200}, false, 2},
		{"do not retry 404", []int{404, 200}, true, 1},
		{"do not retry 401", []int{401, 200}, true, 1},
		{"give up after max retries", []int{503, 503, 503, 503, 503, 503, 503}, true, maxRetries + 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			calls := 0
			_, _, err := retryGitHub(context.Background(), func() (string, *github.Response, error) {
				status := tt.results[calls]
				calls++
				switch status {
				case http.StatusOK:
					return "ok", &github.Response{Response: &http.Response{StatusCode: status}}, nil
				case 0:
					return "", nil, errAPI
				default:
					return "", &github.Response{Response: &http.Response{StatusCode: status}}, errAPI
				}
			})
			if (err != nil) != tt.wantErr {
				t.Errorf("got err %v, wantErr %v", err, tt.wantErr)
			}
			if calls != tt.wantCalls {
				t.Errorf("got %d calls, want %d", calls, tt.wantCalls)
			}
		})
	}
}
