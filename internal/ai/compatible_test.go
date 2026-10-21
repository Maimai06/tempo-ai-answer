package ai_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Maimai06/tempo-ai-answer/internal/ai"
)

func TestCompatibleChatCompletions(t *testing.T) {
	tests := []struct {
		name     string
		baseURL  func(string) string
		wantPath string
	}{
		{name: "root URL", baseURL: func(server string) string { return server }, wantPath: "/chat/completions"},
		{name: "versioned URL", baseURL: func(server string) string { return server + "/v1/" }, wantPath: "/v1/chat/completions"},
		{name: "full endpoint", baseURL: func(server string) string { return server + "/v1/chat/completions" }, wantPath: "/v1/chat/completions"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != tc.wantPath || r.Method != http.MethodPost {
					t.Errorf("request = %s %s", r.Method, r.URL.Path)
				}
				if got := r.Header.Get("Authorization"); got != "Bearer secret-fixture" {
					t.Errorf("authorization header = %q", got)
				}
				var request struct {
					Model    string `json:"model"`
					Messages []struct {
						Role    string `json:"role"`
						Content string `json:"content"`
					} `json:"messages"`
					Stream bool `json:"stream"`
				}
				if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
					t.Error(err)
					return
				}
				if request.Model != "deepseek-chat" || len(request.Messages) != 1 || request.Messages[0].Role != "user" || request.Messages[0].Content != "fixture prompt" || request.Stream {
					t.Errorf("request body = %+v", request)
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"  fixture answer  "}}]}`))
			}))
			defer server.Close()
			provider, err := ai.NewCompatible(tc.baseURL(server.URL), "deepseek-chat", "secret-fixture", time.Second, 2, nil)
			if err != nil {
				t.Fatal(err)
			}
			if answer, err := provider.Ask(context.Background(), "fixture prompt", 1); err != nil || answer != "fixture answer" {
				t.Fatalf("answer = %q, error = %v", answer, err)
			}
			if answer, err := provider.ReadLatest(context.Background()); err != nil || answer != "fixture answer" {
				t.Fatalf("latest = %q, error = %v", answer, err)
			}
			if round, err := provider.BeginQuestion(context.Background()); err != nil || round != 1 {
				t.Fatalf("round = %d, error = %v", round, err)
			}
		})
	}
}

func TestCompatibleErrorsDoNotExposeAPIKeyOrResponseBody(t *testing.T) {
	const secret = "secret-fixture"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, secret, http.StatusUnauthorized)
	}))
	defer server.Close()
	provider, err := ai.NewCompatible(server.URL, "model", secret, time.Second, 2, nil)
	if err != nil {
		t.Fatal(err)
	}
	_, err = provider.Ask(context.Background(), "prompt", 0)
	if err == nil || !strings.Contains(err.Error(), "HTTP 401") || strings.Contains(err.Error(), secret) {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCompatibleCancellation(t *testing.T) {
	started := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		close(started)
		<-time.After(100 * time.Millisecond)
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"late"}}]}`))
	}))
	defer server.Close()
	provider, err := ai.NewCompatible(server.URL, "model", "key", 5*time.Second, 1, nil)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() { _, askErr := provider.Ask(ctx, "prompt", 1); result <- askErr }()
	<-started
	cancel()
	if err := <-result; err == nil || !strings.Contains(err.Error(), "context canceled") {
		t.Fatalf("cancellation error = %v", err)
	}
}

func TestNewCompatibleValidation(t *testing.T) {
	for _, tc := range []struct {
		baseURL, model, key string
	}{
		{baseURL: "file:///tmp/api", model: "model", key: "key"},
		{baseURL: "https://user:pass@example.com", model: "model", key: "key"},
		{baseURL: "https://example.com/api?token=hidden", model: "model", key: "key"},
		{baseURL: "https://example.com", model: "", key: "key"},
		{baseURL: "https://example.com", model: "model", key: ""},
	} {
		if _, err := ai.NewCompatible(tc.baseURL, tc.model, tc.key, time.Second, 1, nil); err == nil {
			t.Fatalf("accepted invalid provider config: %+v", tc)
		}
	}
}
