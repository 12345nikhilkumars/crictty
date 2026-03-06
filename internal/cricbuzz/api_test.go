package cricbuzz

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type hostRewriteTransport struct {
	target *url.URL
	base   http.RoundTripper
}

func (t hostRewriteTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	clone := req.Clone(req.Context())
	clone.URL.Scheme = t.target.Scheme
	clone.URL.Host = t.target.Host
	return t.base.RoundTrip(clone)
}

func newRewrittenClient(t *testing.T, serverURL string) *Client {
	t.Helper()
	target, err := url.Parse(serverURL)
	if err != nil {
		t.Fatalf("parse server url: %v", err)
	}

	c := NewClient()
	c.httpClient = &http.Client{
		Timeout: 2 * time.Second,
		Transport: hostRewriteTransport{
			target: target,
			base:   http.DefaultTransport,
		},
	}
	c.requestInterval = 0
	c.maxRetries = 0
	c.retryBaseBackoff = 1 * time.Millisecond
	return c
}

func TestMakeRequestRateLimiterConcurrentSafety(t *testing.T) {
	var (
		mu    sync.Mutex
		times []time.Time
	)

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		times = append(times, time.Now())
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer ts.Close()

	c := NewClient()
	c.httpClient = ts.Client()
	c.requestInterval = 20 * time.Millisecond
	c.maxRetries = 0

	const n = 4
	start := time.Now()
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			resp, err := c.makeRequestWithContext(context.Background(), ts.URL)
			if err != nil {
				t.Errorf("makeRequestWithContext failed: %v", err)
				return
			}
			resp.Body.Close()
		}()
	}
	wg.Wait()
	elapsed := time.Since(start)

	mu.Lock()
	defer mu.Unlock()
	if len(times) != n {
		t.Fatalf("expected %d requests, got %d", n, len(times))
	}
	for i := 1; i < len(times); i++ {
		delta := times[i].Sub(times[i-1])
		if delta < 12*time.Millisecond {
			t.Fatalf("requests not rate-limited enough: delta[%d]=%v", i, delta)
		}
	}
	if elapsed < 45*time.Millisecond {
		t.Fatalf("expected batched requests to take longer, got %v", elapsed)
	}
}

func TestLimiterIsClientScoped(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer ts.Close()

	c1 := NewClient()
	c1.httpClient = ts.Client()
	c1.requestInterval = 60 * time.Millisecond
	c1.maxRetries = 0

	c2 := NewClient()
	c2.httpClient = ts.Client()
	c2.requestInterval = 60 * time.Millisecond
	c2.maxRetries = 0

	resp, err := c1.makeRequestWithContext(context.Background(), ts.URL)
	if err != nil {
		t.Fatalf("c1 request failed: %v", err)
	}
	resp.Body.Close()

	start := time.Now()
	resp, err = c2.makeRequestWithContext(context.Background(), ts.URL)
	if err != nil {
		t.Fatalf("c2 request failed: %v", err)
	}
	resp.Body.Close()

	if took := time.Since(start); took > 25*time.Millisecond {
		t.Fatalf("second client was unexpectedly throttled: %v", took)
	}
}

func TestStatusErrorIncludesURLStatusAndSnippet(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte("upstream overloaded"))
	}))
	defer ts.Close()

	c := newRewrittenClient(t, ts.URL)
	_, err := c.GetFullCommentaryWithContext(context.Background(), 123, 1)
	if err == nil {
		t.Fatalf("expected error")
	}

	var statusErr *HTTPStatusError
	if !errors.As(err, &statusErr) {
		t.Fatalf("expected HTTPStatusError, got %T (%v)", err, err)
	}
	if statusErr.StatusCode != http.StatusBadGateway {
		t.Fatalf("expected status code %d, got %d", http.StatusBadGateway, statusErr.StatusCode)
	}
	if statusErr.URL == "" {
		t.Fatalf("expected request url in error")
	}
	if statusErr.BodySnippet != "upstream overloaded" {
		t.Fatalf("unexpected body snippet: %q", statusErr.BodySnippet)
	}
}

func TestMakeRequestRetriesTransientStatus(t *testing.T) {
	var attempts int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&attempts, 1)
		if n <= 2 {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte("try again"))
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer ts.Close()

	c := NewClient()
	c.httpClient = ts.Client()
	c.requestInterval = 0
	c.maxRetries = 2
	c.retryBaseBackoff = 5 * time.Millisecond

	resp, err := c.makeRequestWithContext(context.Background(), ts.URL)
	if err != nil {
		t.Fatalf("makeRequestWithContext failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected eventual 200, got %d", resp.StatusCode)
	}
	if got := atomic.LoadInt32(&attempts); got != 3 {
		t.Fatalf("expected 3 attempts, got %d", got)
	}
}

func TestGetFullCommentaryReversesToOldestFirst(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{
			"commentary": [{
				"inningsId": 1,
				"commentaryList": [
					{"commText":"newest","overNumber":19.6,"inningsId":1,"event":"run","timestamp":300},
					{"commText":"middle","overNumber":"19.5","inningsId":1,"event":"four","timestamp":200},
					{"commText":"oldest","overNumber":19.4,"inningsId":1,"event":"dot","timestamp":100}
				]
			}]
		}`))
	}))
	defer ts.Close()

	c := newRewrittenClient(t, ts.URL)

	entries, err := c.GetFullCommentaryWithContext(context.Background(), 999, 1)
	if err != nil {
		t.Fatalf("GetFullCommentaryWithContext failed: %v", err)
	}
	if len(entries) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(entries))
	}

	if entries[0].CommText != "oldest" || entries[2].CommText != "newest" {
		t.Fatalf("unexpected order after reverse: first=%q last=%q", entries[0].CommText, entries[2].CommText)
	}
	if entries[0].OverNumber != "19.4" || entries[1].OverNumber != "19.5" || entries[2].OverNumber != "19.6" {
		t.Fatalf("unexpected over number parsing: %+v", entries)
	}
}
