package utils

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestDoJSONAndQuery(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("expected POST, got %s", r.Method)
		}
		if r.URL.Query().Get("keyword") != "hello world" {
			t.Fatalf("unexpected query value: %s", r.URL.Query().Get("keyword"))
		}
		if got := r.Header.Get("X-Test"); got != "quickpress" {
			t.Fatalf("unexpected header: %s", got)
		}

		var body map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode body failed: %v", err)
		}
		if body["name"] != "qp" {
			t.Fatalf("unexpected body payload: %+v", body)
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":200,"data":{"token":"abc-123"}}`))
	}))
	defer server.Close()

	resp := Do(RequestOptions{
		Method: "POST",
		URL:    server.URL + "/api/{id}",
		Query: map[string]interface{}{
			"id":      99,
			"keyword": "hello world",
		},
		Body: map[string]interface{}{
			"name": "qp",
		},
		BodyType: BodyJSON,
		Headers: map[string]string{
			"X-Test": "quickpress",
		},
		Timeout: time.Second,
	})

	if resp.Error != nil {
		t.Fatalf("request failed: %v", resp.Error)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("unexpected status code: %d", resp.StatusCode)
	}
	if resp.Duration <= 0 {
		t.Fatalf("duration should be recorded")
	}
	value, err := resp.GetString("data.token")
	if err != nil {
		t.Fatalf("extract token failed: %v", err)
	}
	if value != "abc-123" {
		t.Fatalf("unexpected token: %s", value)
	}
}

func TestDoAllowsPlainTextResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte("ok"))
	}))
	defer server.Close()

	resp := Do(RequestOptions{Method: "GET", URL: server.URL, Timeout: time.Second})
	if resp.Error != nil {
		t.Fatalf("plain text response should not fail: %v", resp.Error)
	}
	if resp.StrBody != "ok" {
		t.Fatalf("unexpected response body: %s", resp.StrBody)
	}
}
