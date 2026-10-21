package ai

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Maimai06/tempo-ai-answer/internal/browser"
)

func TestDeepSeekTimeoutIncludesSanitizedPageDiagnostics(t *testing.T) {
	if os.Getenv("TEMPO_BROWSER_TEST") != "1" {
		t.Skip("需要浏览器 fixture")
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`<html><body><textarea></textarea><button aria-label="发送">发送</button></body></html>`))
	}))
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	session, err := browser.Start(ctx, t.TempDir(), true)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := session.Close(); err != nil {
			t.Error(err)
		}
	}()
	page := session.NewPage()
	defer page.Close()
	if err := page.Navigate(ctx, server.URL); err != nil {
		t.Fatal(err)
	}

	web := New(page, 100*time.Millisecond, 1, nil)
	_, err = web.Ask(ctx, "fixture prompt", 1)
	if err == nil {
		t.Fatal("Ask succeeded without a response")
	}
	for _, want := range []string{"页面=", "输入框=1", "回答节点=0", "生成中=false"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("timeout error lacks %q: %v", want, err)
		}
	}
}
