package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
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
	badCourses := false
	badSteps, multi := false, false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		var data any
		success := true
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
			if badCourses {
				data = []any{map[string]any{"coursesId": []string{"invalid"}}}
			}
		case "/gateway/tempo-talents/api/tt-course-challenge/challenge/list":
			course := r.URL.Query().Get("courseId")
			if (course != "course-2" && !(multi && course == "course-1")) || r.URL.Query().Get("classroomId") != "second" {
				unexpected = append(unexpected, "wrong experiment or classroom in step list")
			}
			data = map[string]any{"rows": []any{
				map[string]any{"stepId": "selected-step", "name": "目标关卡", "sequence": 1},
				map[string]any{"stepId": "other-step", "name": "未选关卡", "sequence": 2},
				map[string]any{"stepId": "completed-step", "name": "已完成关卡", "sequence": 3, "finished": true},
			}}
			if course == "course-1" {
				data = map[string]any{"rows": []any{
					map[string]any{"stepId": "course-1-other", "name": "基础第一关", "sequence": 1},
					map[string]any{"stepId": "course-1-selected", "name": "基础第二关", "sequence": 2},
				}}
			}
			if badSteps {
				success = false
			}
		case "/gateway/tempo-talents/api/tt-course-challenge/challenge":
			requestedSteps = append(requestedSteps, r.URL.Query().Get("courseStepId"))
			if r.URL.Query().Get("classroomId") != "second" {
				unexpected = append(unexpected, "wrong classroom in question")
			}
			data = map[string]any{"courseStepId": r.URL.Query().Get("courseStepId"), "questionType": "select", "questionInfo": map[string]any{"selectOrJudgeList": []any{map[string]any{"questionId": 1, "type": "single", "title": "fixture", "content": []string{"A", "B"}}}}}
		default:
			if r.URL.Path != "/favicon.ico" {
				unexpected = append(unexpected, r.URL.Path)
			}
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if err := json.NewEncoder(w).Encode(map[string]any{"success": success, "data": data}); err != nil {
			t.Error(err)
		}
	}))
	defer server.Close()
	for _, tc := range []struct {
		name, input, want string
		badCourses        bool
		badSteps, multi   bool
		questions         []string
	}{
		{name: "single step demo", input: "4\n2\n2\n1\n\n0\n", want: "生成答案 1 关", questions: []string{"selected-step"}},
		{name: "independent experiment choices", input: "4\n2\n1-2\n2\n1\n\n0\n", want: "生成答案 2 关", multi: true, questions: []string{"course-1-selected", "selected-step"}},
		{name: "cancel completed single trial at confirmation", input: "7\n2\n2\n3\n0\n0\n", want: "已取消"},
		{name: "cancel at step list", input: "4\n2\n2\n0\n0\n", want: "已取消"},
		{name: "unavailable list does not initialize environment", input: "4\n2\n2\n0\n", want: "关卡列表暂不可用", badSteps: true},
		{name: "friendly platform data failure", input: "4\n2\n0\n", want: "暂时无法读取平台数据", badCourses: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mu.Lock()
			requestedSteps, unexpected, badCourses = nil, nil, tc.badCourses
			badSteps, multi = tc.badSteps, tc.multi
			mu.Unlock()
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
			path := filepath.Join(dir, "config.json")
			if err = os.WriteFile(path, encoded, 0600); err != nil {
				t.Fatal(err)
			}
			output := &bytes.Buffer{}
			a := &app{reader: bufio.NewReader(strings.NewReader(tc.input)), writer: output, dataDir: dir}
			root := command()
			ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
			defer cancel()
			root.SetContext(ctx)
			if err = a.menu(root); err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(output.String(), tc.want) || strings.Contains(output.String(), "time=") || strings.Contains(output.String(), "数据格式错误:") {
				t.Fatal(output.String())
			}
			if !tc.badCourses && !tc.badSteps && !strings.Contains(output.String(), "1. 目标关卡") {
				t.Fatal("missing actual step names", output.String())
			}
			if tc.badCourses {
				log, err := os.ReadFile(a.logFile())
				if err != nil || !strings.Contains(string(log), "数据格式错误") {
					t.Fatal("missing diagnostic log", err)
				}
			}
			after, err := os.ReadFile(path)
			if err != nil || !bytes.Equal(after, encoded) {
				t.Fatal("menu modified personal configuration", err)
			}
			mu.Lock()
			defer mu.Unlock()
			if len(unexpected) != 0 || !reflect.DeepEqual(requestedSteps, tc.questions) {
				t.Fatalf("unexpected calls=%v, questions=%v", unexpected, requestedSteps)
			}
		})
	}
}
