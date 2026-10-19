package browser_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Maimai06/tempo-ai-answer/internal/browser"
	"github.com/Maimai06/tempo-ai-answer/internal/platform"
)

// TestCDPFixture 仅访问 loopback，不读取用户 profile 或任何真实账号。
func TestCDPFixture(t *testing.T) {
	if os.Getenv("TEMPO_BROWSER_TEST") != "1" {
		t.Skip("设置 TEMPO_BROWSER_TEST=1 运行真实 CDP fixture")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/favicon.ico" {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		if r.URL.Path == "/tempo-talents/" {
			http.SetCookie(w, &http.Cookie{Name: "fixture_session", Value: "ok", HttpOnly: true, Path: "/"})
			_, _ = w.Write([]byte("<html><body>fixture</body></html>"))
			return
		}
		if r.Header.Get("X-Requested-With") != "XMLHttpRequest" {
			t.Error("missing gateway header")
		}
		if cookie, e := r.Cookie("fixture_session"); e != nil || cookie.Value != "ok" {
			t.Error("missing cookie")
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/gateway/tempo-talents/api/tt-course-challenge/challenge":
			if r.URL.Query().Get("courseStepId") != "中文 & id" {
				t.Error("GET encoding")
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "data": map[string]any{"courseStepId": "fixture", "questionType": "code"}})
		case "/gateway/tempo-talents/api/tt-classroom/list":
			if r.Method != "POST" {
				t.Error("method")
			}
			if e := r.ParseForm(); e != nil {
				t.Error(e)
			}
			if r.Form.Get("classroomStatus") != "started" {
				t.Error("POST form")
			}
			_, _ = w.Write([]byte(`{"success":true,"data":{"classroomListInfo":{"rows":[]}}}`))
		default:
			t.Errorf("unexpected URL %s", r.URL.Path)
		}
	}))
	defer ts.Close()
	s, e := browser.Start(ctx, filepath.Join(t.TempDir(), "中文 profile"), true)
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	if e = s.Root.Navigate(ctx, ts.URL+"/tempo-talents/"); e != nil {
		t.Fatal(e)
	}
	c := platform.Client{Page: s.Root, BaseURL: ts.URL + "/tempo-talents/"}
	step, e := c.GetStep(ctx, "中文 & id")
	if e != nil || step.ID != "fixture" {
		t.Fatal(step, e)
	}
	if _, e = c.ListClassrooms(ctx); e != nil {
		t.Fatal(e)
	}
	p := s.NewPage()
	defer p.Close()
	if e = p.Navigate(ctx, ts.URL+"/tempo-talents/"); e != nil {
		t.Fatal(e)
	}
	var value int
	if e = p.Eval(ctx, "Promise.resolve(42)", &value); e != nil || value != 42 {
		t.Fatal(value, e)
	}
	if e = s.Root.Eval(ctx, "1+1", &value); e != nil || value != 2 {
		t.Fatal("root page died after new tab", value, e)
	}
}
