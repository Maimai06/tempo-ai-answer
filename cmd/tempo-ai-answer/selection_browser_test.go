package main

import (
	"bufio"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Maimai06/tempo-ai-answer/internal/config"
)

// TestSelectionBrowserFixture 验证真实浏览器从第二课堂到指定实验、指定单关的完整演示。
func TestSelectionBrowserFixture(t *testing.T) {
	if os.Getenv("TEMPO_BROWSER_TEST") != "1" {
		t.Skip("需要本机浏览器 fixture")
	}
	var mu sync.Mutex
	var requestedSteps []string
	var unexpected []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		var data any
		switch r.URL.Path {
		case "/tempo-talents/":
			w.Header().Set("Content-Type", "text/html")
			_, _ = w.Write([]byte("<html><body>fixture</body></html>"))
			return
		case "/gateway/tempo-talents/api/tt-common/user/current":
			data = map[string]any{"name": "fixture"}
		case "/gateway/tempo-talents/api/tt-classroom/list":
			data = map[string]any{"classroomListInfo": map[string]any{"rows": []any{map[string]any{"id": "first", "classroomName": "第一课堂"}, map[string]any{"id": "second", "classroomName": "第二课堂"}}}}
		case "/gateway/tempo-talents/api/tt-classroom/classroomCourseListDetail":
			if err := r.ParseForm(); err != nil || r.Form.Get("classroomId") != "second" {
				unexpected = append(unexpected, "wrong classroom")
			}
			data = []any{map[string]any{"sort": 1, "lastList": []any{
				map[string]any{"coursesId": "course-1", "coursesName": "跳过的实验", "sort": 1},
				map[string]any{"coursesId": "course-2", "coursesName": "选定实验", "sort": 2},
				map[string]any{"coursesId": "course-3", "coursesName": "另一个未选实验", "sort": 3},
			}}}
		case "/gateway/tempo-talents/api/tt-course-challenge/challenge/list":
			if r.URL.Query().Get("courseId") != "course-2" || r.URL.Query().Get("classroomId") != "second" {
				unexpected = append(unexpected, "wrong experiment or classroom in step list")
			}
			data = map[string]any{"rows": []any{
				map[string]any{"stepId": "selected-step", "name": "目标关卡", "sequence": 1},
				map[string]any{"stepId": "other-step", "name": "未选关卡", "sequence": 2},
			}}
		case "/gateway/tempo-talents/api/tt-course-challenge/challenge":
			requestedSteps = append(requestedSteps, r.URL.Query().Get("courseStepId"))
			if r.URL.Query().Get("classroomId") != "second" {
				unexpected = append(unexpected, "wrong classroom in question")
			}
			data = map[string]any{"courseStepId": "selected-step", "questionType": "select", "questionInfo": map[string]any{"selectOrJudgeList": []any{map[string]any{"questionId": 1, "type": "single", "title": "fixture", "content": []string{"A", "B"}}}}}
		default:
			if r.URL.Path != "/favicon.ico" {
				unexpected = append(unexpected, r.URL.Path)
			}
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if err := json.NewEncoder(w).Encode(map[string]any{"success": true, "data": data}); err != nil {
			t.Error(err)
		}
	}))
	defer server.Close()
	dir := t.TempDir()
	cfg := config.Defaults()
	cfg.Platform.BaseURL, cfg.Platform.Username, cfg.Platform.Password = server.URL+"/tempo-talents/", "fixture", "fixture"
	// 菜单允许覆盖配置里的默认课堂。
	cfg.Platform.ClassroomID = "first"
	cfg.Run.Delay = 0
	encoded, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(dir, "config.json"), encoded, 0600); err != nil {
		t.Fatal(err)
	}
	a := &app{reader: bufio.NewReader(strings.NewReader("4\n2\n2\n1\n\n0\n")), dataDir: dir}
	root := command()
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	root.SetContext(ctx)
	if err = a.menu(root); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(unexpected) != 0 || len(requestedSteps) != 1 || requestedSteps[0] != "selected-step" {
		t.Fatalf("unexpected calls=%v, questions=%v", unexpected, requestedSteps)
	}
}
