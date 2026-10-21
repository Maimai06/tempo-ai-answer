package dashboard

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/Maimai06/tempo-ai-answer/internal/core"
)

func TestStateContractConcurrent(t *testing.T) {
	s := New()
	ts := httptest.NewServer(s)
	defer ts.Close()
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 40; j++ {
				core.Sink(s.Consume).Emit(core.Event{Kind: "log", Text: "fixture"})
				r, e := http.Get(ts.URL + "/state")
				if e != nil {
					t.Error(e)
					return
				}
				var data map[string]any
				e = json.NewDecoder(r.Body).Decode(&data)
				r.Body.Close()
				if e != nil || len(data) != 21 {
					t.Errorf("contract: %v, keys %d", e, len(data))
				}
			}
		}()
	}
	wg.Wait()
	if len(s.state.Logs) != 200 {
		t.Fatal(len(s.state.Logs))
	}
	step := core.Step{Title: "next"}
	s.Consume(core.Event{Kind: "step", Step: &step})
	if s.state.AIAnswer != "" || s.state.EvalPassed != nil {
		t.Fatal(s.state)
	}
	r, e := http.Get(ts.URL)
	if e != nil {
		t.Fatal(e)
	}
	defer r.Body.Close()
	b, e := io.ReadAll(r.Body)
	if e != nil || len(b) < 1000 {
		t.Fatal(e, len(b))
	}
}
func TestPortFallback(t *testing.T) {
	ln, e := net.Listen("tcp", "127.0.0.1:0")
	if e != nil {
		t.Fatal(e)
	}
	defer ln.Close()
	port := ln.Addr().(*net.TCPAddr).Port
	if port > 65520 {
		t.Skip("port boundary")
	}
	s := New()
	if e = s.Start(port); e != nil {
		t.Fatal(e)
	}
	defer s.Close(context.Background())
	if s.URL == "http://"+ln.Addr().String()+"/" {
		t.Fatal("bound occupied port")
	}
}

func TestSelectionCloseRequiresCompletedTaskAndSessionKey(t *testing.T) {
	closed := make(chan struct{}, 1)
	s := New()
	if err := s.EnableSelection(SelectionAPI{
		Classrooms: func(context.Context) ([]core.Classroom, error) { return nil, nil },
		Courses:    func(context.Context, string) ([]CourseOption, error) { return nil, nil },
		Steps:      func(context.Context, string, string) ([]StepOption, error) { return nil, nil },
		Start:      func(context.Context, RunSelection) error { return nil },
		Cancel:     func(context.Context) error { return nil },
		Close:      func(context.Context) error { closed <- struct{}{}; return nil },
	}); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(s)
	defer server.Close()
	page, err := http.Get(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	body, err := io.ReadAll(page.Body)
	page.Body.Close()
	if err != nil {
		t.Fatal(err)
	}
	html := string(body)
	keyStart := strings.Index(html, `name="selection-key" content="`) + len(`name="selection-key" content="`)
	keyEnd := strings.Index(html[keyStart:], `"`)
	if keyStart < len(`name="selection-key" content="`) || keyEnd < 0 {
		t.Fatal("missing selection key")
	}
	key := html[keyStart : keyStart+keyEnd]
	post := func(path, sessionKey, payload string) int {
		req, err := http.NewRequest(http.MethodPost, server.URL+path, strings.NewReader(payload))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Content-Type", "application/json")
		if sessionKey != "" {
			req.Header.Set("X-Selection-Key", sessionKey)
		}
		response, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		response.Body.Close()
		return response.StatusCode
	}
	if status := post("/api/select/close", key, "{}"); status != http.StatusConflict {
		t.Fatalf("close before completion status = %d", status)
	}
	request := `{"mode":"formal","classroom_id":"class","courses":[{"course_id":"course","step_ids":["step"]}]}`
	if status := post("/api/select/start", key, request); status != http.StatusAccepted {
		t.Fatalf("start status = %d", status)
	}
	if status := post("/api/select/close", "wrong", "{}"); status != http.StatusForbidden {
		t.Fatalf("invalid key status = %d", status)
	}
	s.Consume(core.Event{Kind: "finished"})
	if status := post("/api/select/close", key, "{}"); status != http.StatusAccepted {
		t.Fatalf("close status = %d", status)
	}
	select {
	case <-closed:
	default:
		t.Fatal("close callback was not invoked")
	}
	if s.state.AIStatus != "finished" {
		t.Fatalf("finished state = %q", s.state.AIStatus)
	}
}

func TestSelectionCanStartAnotherRangeAfterContinue(t *testing.T) {
	started := make(chan RunSelection, 2)
	continued := make(chan struct{}, 1)
	s := New()
	if err := s.EnableSelection(SelectionAPI{
		Classrooms: func(context.Context) ([]core.Classroom, error) {
			return []core.Classroom{{ID: "class", Name: "Java"}}, nil
		},
		Courses: func(context.Context, string) ([]CourseOption, error) {
			return []CourseOption{{ID: "course", Name: "基础实验"}}, nil
		},
		Steps: func(context.Context, string, string) ([]StepOption, error) {
			return []StepOption{{ID: "step", Sequence: 1, Name: "第一关"}}, nil
		},
		Start:    func(_ context.Context, selection RunSelection) error { started <- selection; return nil },
		Cancel:   func(context.Context) error { return nil },
		Continue: func(context.Context) error { continued <- struct{}{}; return nil },
	}); err != nil {
		t.Fatal(err)
	}
	page := httptest.NewRecorder()
	s.ServeHTTP(page, httptest.NewRequest(http.MethodGet, "/", nil))
	html := page.Body.String()
	if page.Code != http.StatusOK || !strings.Contains(html, `id="continue-run"`) {
		t.Fatalf("continue control missing: status=%d", page.Code)
	}
	keyStart := strings.Index(html, `name="selection-key" content="`) + len(`name="selection-key" content="`)
	keyEnd := strings.Index(html[keyStart:], `"`)
	if keyStart < len(`name="selection-key" content="`) || keyEnd < 0 {
		t.Fatal("selection key missing")
	}
	key := html[keyStart : keyStart+keyEnd]
	post := func(path, payload string) int {
		req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(payload))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Selection-Key", key)
		response := httptest.NewRecorder()
		s.ServeHTTP(response, req)
		return response.Code
	}
	selection := `{"mode":"formal","classroom_id":"class","courses":[{"course_id":"course","step_ids":["step"]}]}`
	if status := post("/api/select/start", selection); status != http.StatusAccepted {
		t.Fatalf("first start status = %d", status)
	}
	<-started
	if status := post("/api/select/continue", "{}"); status != http.StatusConflict {
		t.Fatalf("continue before completion status = %d", status)
	}
	s.Consume(core.Event{Kind: "finished"})
	if status := post("/api/select/continue", "{}"); status != http.StatusAccepted {
		t.Fatalf("continue status = %d", status)
	}
	select {
	case <-continued:
	default:
		t.Fatal("continue callback was not invoked")
	}
	if s.selectionDone || s.taskFinished || s.state.AIStatus != "idle" {
		t.Fatalf("selection state was not reset: done=%v finished=%v status=%q", s.selectionDone, s.taskFinished, s.state.AIStatus)
	}
	if status := post("/api/select/start", selection); status != http.StatusAccepted {
		t.Fatalf("second start status = %d", status)
	}
	if got := <-started; got.ClassroomID != "class" {
		t.Fatalf("second range was not delivered: %+v", got)
	}
}

func TestSelectionAPIRequiresSessionKeyAndAcceptsExplicitPlan(t *testing.T) {
	started := make(chan RunSelection, 1)
	canceled := make(chan bool, 1)
	s := New()
	if err := s.EnableSelection(SelectionAPI{
		Classrooms: func(context.Context) ([]core.Classroom, error) {
			return []core.Classroom{{ID: "class-1", Name: "Java"}}, nil
		},
		Courses: func(context.Context, string) ([]CourseOption, error) {
			return []CourseOption{{ID: "course-1", Number: "1-1", Chapter: "第一章", Name: "基础实验"}}, nil
		},
		Steps: func(context.Context, string, string) ([]StepOption, error) {
			return []StepOption{{ID: "step-1", Sequence: 1, Name: "第一关"}}, nil
		},
		Start:  func(_ context.Context, selection RunSelection) error { started <- selection; return nil },
		Cancel: func(context.Context) error { canceled <- true; return nil },
	}); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(s)
	defer server.Close()

	page, err := http.Get(server.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	body, err := io.ReadAll(page.Body)
	page.Body.Close()
	if err != nil || page.StatusCode != http.StatusOK || !strings.Contains(string(body), `id="scope-view"`) || !strings.Contains(string(body), `id="monitor-view"`) || !strings.Contains(string(body), "运行监控") {
		t.Fatalf("selection page: status=%d err=%v", page.StatusCode, err)
	}
	keyStart := strings.Index(string(body), `name="selection-key" content="`) + len(`name="selection-key" content="`)
	keyEnd := strings.Index(string(body)[keyStart:], `"`)
	if keyStart < len(`name="selection-key" content="`) || keyEnd < 0 {
		t.Fatal("page did not receive its session key")
	}
	key := string(body)[keyStart : keyStart+keyEnd]
	alias, err := http.Get(server.URL + "/select")
	if err != nil {
		t.Fatal(err)
	}
	aliasBody, err := io.ReadAll(alias.Body)
	alias.Body.Close()
	if err != nil || alias.StatusCode != http.StatusOK || !strings.Contains(string(aliasBody), `id="monitor-view"`) {
		t.Fatalf("selection alias page: status=%d err=%v", alias.StatusCode, err)
	}

	request := RunSelection{Mode: ModeFormal, ClassroomID: "class-1", Courses: []CourseSelection{{CourseID: "course-1", StepIDs: []string{"step-1"}}}}
	encoded, _ := json.Marshal(request)
	post := func(sessionKey, origin string, payload []byte) *http.Response {
		req, err := http.NewRequest(http.MethodPost, server.URL+"/api/select/start", strings.NewReader(string(payload)))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Content-Type", "application/json")
		if sessionKey != "" {
			req.Header.Set("X-Selection-Key", sessionKey)
		}
		if origin != "" {
			req.Header.Set("Origin", origin)
		}
		response, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		return response
	}

	response := post("", "", encoded)
	response.Body.Close()
	if response.StatusCode != http.StatusForbidden {
		t.Fatalf("missing key status=%d", response.StatusCode)
	}
	response = post(key, "https://attacker.example", encoded)
	response.Body.Close()
	if response.StatusCode != http.StatusForbidden {
		t.Fatalf("cross-origin status=%d", response.StatusCode)
	}
	response = post(key, "", encoded)
	response.Body.Close()
	if response.StatusCode != http.StatusAccepted {
		t.Fatalf("valid plan status=%d", response.StatusCode)
	}
	if got := <-started; got.ClassroomID != "class-1" || got.Courses[0].StepIDs[0] != "step-1" {
		t.Fatalf("wrong selection delivered: %+v", got)
	}

	cancelRequest, err := http.NewRequest(http.MethodPost, server.URL+"/api/select/cancel", strings.NewReader("{}"))
	if err != nil {
		t.Fatal(err)
	}
	cancelRequest.Header.Set("X-Selection-Key", key)
	response, err = http.DefaultClient.Do(cancelRequest)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusGone {
		t.Fatalf("cancel status=%d", response.StatusCode)
	}
	select {
	case <-canceled:
		t.Fatal("completed session was canceled twice")
	default:
	}

	cancelServer := New()
	if err := cancelServer.EnableSelection(SelectionAPI{
		Classrooms: func(context.Context) ([]core.Classroom, error) { return nil, nil },
		Courses:    func(context.Context, string) ([]CourseOption, error) { return nil, nil },
		Steps:      func(context.Context, string, string) ([]StepOption, error) { return nil, nil },
		Start:      func(context.Context, RunSelection) error { return nil },
		Cancel:     func(context.Context) error { canceled <- true; return nil },
	}); err != nil {
		t.Fatal(err)
	}
	cancelHTTP := httptest.NewServer(cancelServer)
	defer cancelHTTP.Close()
	cancelPage, err := http.Get(cancelHTTP.URL + "/select")
	if err != nil {
		t.Fatal(err)
	}
	cancelBody, err := io.ReadAll(cancelPage.Body)
	cancelPage.Body.Close()
	if err != nil {
		t.Fatal(err)
	}
	cancelHTML := string(cancelBody)
	cancelKeyStart := strings.Index(cancelHTML, `name="selection-key" content="`) + len(`name="selection-key" content="`)
	cancelKeyEnd := strings.Index(cancelHTML[cancelKeyStart:], `"`)
	cancelKey := cancelHTML[cancelKeyStart : cancelKeyStart+cancelKeyEnd]
	cancelRequest, err = http.NewRequest(http.MethodPost, cancelHTTP.URL+"/api/select/cancel", strings.NewReader("{}"))
	if err != nil {
		t.Fatal(err)
	}
	cancelRequest.Header.Set("X-Selection-Key", cancelKey)
	response, err = http.DefaultClient.Do(cancelRequest)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusAccepted || !<-canceled {
		t.Fatalf("pending session cancel status=%d", response.StatusCode)
	}
}

func TestRunSelectionRejectsMissingAndDuplicateScope(t *testing.T) {
	for _, selection := range []RunSelection{
		{Mode: ModeFormal, ClassroomID: "class-1"},
		{Mode: ModeFormal, ClassroomID: "class-1", Courses: []CourseSelection{{CourseID: "course-1", StepIDs: []string{"step-1", "step-1"}}}},
		{Mode: ModeTrial, ClassroomID: "class-1", Courses: []CourseSelection{{CourseID: "course-1", StepIDs: []string{"step-1", "step-2"}}}},
	} {
		if err := selection.Validate(); err == nil {
			t.Fatalf("accepted invalid selection: %+v", selection)
		}
	}
}
