package requests

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"quickpress/config"
)

func TestRunUsesDynamicTimestampPlaceholders(t *testing.T) {
	var gotSecond string
	var gotMilli string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotSecond = r.URL.Query().Get("ts")
		gotMilli = r.URL.Query().Get("ms")
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	before := time.Now()
	executor := NewExecutor(config.Config{
		Global: map[string]string{
			"ts": "{timestamp}",
			"ms": "{timestamp_ms}",
		},
		Requests: []config.Request{{
			Name:           "timestamp",
			Method:         "GET",
			URL:            server.URL,
			ExpectedStatus: http.StatusOK,
			Query: map[string]any{
				"ts": "${ts}",
				"ms": "${ms}",
			},
		}},
	})

	result := executor.Run(nil)
	after := time.Now()
	if !result.Success {
		t.Fatalf("run should succeed: %s", result.Error)
	}

	second, err := strconv.ParseInt(gotSecond, 10, 64)
	if err != nil {
		t.Fatalf("timestamp should be unix seconds, got %q: %v", gotSecond, err)
	}
	milli, err := strconv.ParseInt(gotMilli, 10, 64)
	if err != nil {
		t.Fatalf("timestamp_ms should be unix milliseconds, got %q: %v", gotMilli, err)
	}
	if second < before.Unix() || second > after.Unix() {
		t.Fatalf("timestamp out of range: %d", second)
	}
	if milli < before.UnixMilli() || milli > after.UnixMilli() {
		t.Fatalf("timestamp_ms out of range: %d", milli)
	}
}

func TestRunTreatsPostQueryAsFormWhenBodyEmpty(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("expected POST, got %s", r.Method)
		}
		if r.URL.RawQuery != "" {
			t.Fatalf("query should be sent as form body, got raw query %q", r.URL.RawQuery)
		}
		if contentType := r.Header.Get("Content-Type"); !strings.HasPrefix(contentType, "application/x-www-form-urlencoded") {
			t.Fatalf("expected form content type, got %q", contentType)
		}
		if err := r.ParseForm(); err != nil {
			t.Fatalf("parse form failed: %v", err)
		}
		if got := r.PostForm.Get("name"); got != "quickpress" {
			t.Fatalf("unexpected form name: %q", got)
		}
		if got := r.PostForm.Get("id"); got != "42" {
			t.Fatalf("unexpected form id: %q", got)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	executor := NewExecutor(config.Config{
		Requests: []config.Request{{
			Name:           "form",
			Method:         "POST",
			URL:            server.URL,
			ExpectedStatus: http.StatusOK,
			Query: map[string]any{
				"name": "quickpress",
				"id":   "${id}",
			},
		}},
	})

	result := executor.Run(map[string]string{"id": "42"})
	if !result.Success {
		t.Fatalf("run should succeed: %s", result.Error)
	}
	if len(result.Steps) != 1 {
		t.Fatalf("expected one step, got %d", len(result.Steps))
	}
	if result.Steps[0].RequestBodyType != "application/x-www-form-urlencoded" {
		t.Fatalf("unexpected body type: %s", result.Steps[0].RequestBodyType)
	}
}
