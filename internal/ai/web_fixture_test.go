package ai_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Maimai06/tempo-ai-answer/internal/ai"
	"github.com/Maimai06/tempo-ai-answer/internal/browser"
)

func TestDeepSeekDOMFixture(t *testing.T) {
	if os.Getenv("TEMPO_BROWSER_TEST") != "1" {
		t.Skip("需要浏览器 fixture")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`<html><body><textarea></textarea><button aria-label="发送">发送</button><script>document.querySelector('button').onclick=()=>{const e=document.createElement('div');e.className='ds-markdown';e.innerHTML='<div class="toolbar">java 复制 下载</div><pre>class Fixture {}</pre>';document.body.appendChild(e)}</script></body></html>`))
	}))
	defer ts.Close()
	s, e := browser.Start(ctx, t.TempDir(), true)
	if e != nil {
		t.Fatal(e)
	}
	defer func() {
		if err := s.Close(); err != nil {
			t.Error(err)
		}
	}()
	if e = s.Root.Navigate(ctx, ts.URL); e != nil {
		t.Fatal(e)
	}
	web := ai.New(s.Root, 15*time.Second, 3, nil)
	ok, e := web.LoggedIn(ctx)
	if e != nil || !ok {
		t.Fatal(ok, e)
	}
	answer, e := web.Ask(ctx, "fixture prompt", 1)
	if e != nil || !strings.Contains(answer, "class Fixture {}") || strings.Contains(answer, "复制") {
		t.Fatal(answer, e)
	}
	if e = web.Capture(ctx); e != nil || !web.Before.Captured || len(web.Before.IDs) != 0 {
		t.Fatal(web.Before, e)
	}
	n, e := web.Cleanup(ctx)
	if e != nil || n != 0 {
		t.Fatal(n, e)
	}
}
