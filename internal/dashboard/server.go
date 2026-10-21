// Package dashboard 保持 v1 /state 契约，并提供本机范围选择与运行监控。
package dashboard

import (
	"context"
	"crypto/rand"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/Maimai06/tempo-ai-answer/internal/core"
)

//go:embed view.html
var view []byte

//go:embed selector.html
var selector []byte

// Log 保持 v1 面板的单条日志 JSON 结构。
type Log struct {
	Time string `json:"t"`
	Text string `json:"text"`
	Kind string `json:"kind"`
}

// State 对齐原 HTML，不将凭据或平台响应原文放入状态。
type State struct {
	Account     string  `json:"account"`
	Course      string  `json:"course"`
	PracticeURL string  `json:"practice_url"`
	ChatRound   string  `json:"chat_round"`
	StepNo      int     `json:"step_no"`
	StepName    string  `json:"step_name"`
	QType       string  `json:"qtype"`
	Index       int     `json:"index"`
	Total       int     `json:"total"`
	Desc        string  `json:"desc"`
	Framework   string  `json:"framework"`
	AIStatus    string  `json:"ai_status"`
	AIAnswer    string  `json:"ai_answer"`
	PromptLen   int     `json:"prompt_len"`
	Attempt     int     `json:"attempt"`
	FinalCode   string  `json:"final_code"`
	EvalPassed  *bool   `json:"eval_passed"`
	EvalDetail  string  `json:"eval_detail"`
	Logs        []Log   `json:"logs"`
	StartedAt   float64 `json:"started_at"`
	UpdatedAt   float64 `json:"updated_at"`
}

// Server 并发保护状态，并拥有本机 HTTP 服务。
type Server struct {
	mu            sync.RWMutex
	state         State
	srv           *http.Server
	URL           string
	selection     *SelectionAPI
	selectionKey  string
	selectionMu   sync.Mutex
	selectionDone bool
	taskFinished  bool
}

// New 创建与 v1 一致的空面板状态。
func New() *Server {
	now := float64(time.Now().UnixMilli()) / 1000
	return &Server{state: State{AIStatus: "idle", Logs: []Log{}, StartedAt: now, UpdatedAt: now}}
}

// Overview 更新运行期账号、课堂链接与进度。
func (s *Server) Overview(account, course, url string, index, total int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if account != "" {
		s.state.Account = account
	}
	s.state.Course = course
	s.state.PracticeURL = url
	s.state.Index = index
	s.state.Total = total
	s.state.UpdatedAt = float64(time.Now().UnixMilli()) / 1000
}

// Consume 同步消费类型化事件，锁保护 HTTP 并发读取。
func (s *Server) Consume(e core.Event) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v := &s.state
	v.UpdatedAt = float64(e.At.UnixMilli()) / 1000
	switch e.Kind {
	case "step":
		if e.Step == nil {
			return
		}
		step := e.Step
		v.StepNo = e.Index
		v.StepName = step.Title
		v.QType = step.TypeName
		if v.QType == "" {
			v.QType = step.Type
		}
		v.Desc = step.Description
		v.Framework = ""
		if len(step.Info.Codes) > 0 {
			v.Framework = step.Info.Codes[0].Code
		}
		v.AIStatus = "idle"
		v.AIAnswer = ""
		v.FinalCode = ""
		v.PromptLen = 0
		v.Attempt = 0
		v.EvalPassed = nil
		v.EvalDetail = ""
	case "round":
		v.ChatRound = e.Text
	case "asking":
		v.AIStatus = "asking"
		v.AIAnswer = ""
		v.Attempt = e.Attempt
		v.PromptLen = e.PromptLen
		v.EvalPassed = nil
		v.EvalDetail = ""
	case "answer":
		v.AIStatus = "answering"
		v.AIAnswer = e.Text
	case "generated":
		v.FinalCode = e.Text
		v.AIStatus = "done"
	case "result":
		v.EvalPassed = e.Passed
		v.EvalDetail = e.Text
		v.AIStatus = "done"
	case "error":
		v.EvalDetail = e.Text
		v.AIStatus = "error"
	case "finished":
		v.AIStatus = "finished"
		s.taskFinished = true
	case "log", "warn":
		kind := "info"
		if e.Kind == "warn" {
			kind = "err"
		}
		v.Logs = append(v.Logs, Log{Time: e.At.Format("15:04:05"), Text: e.Text, Kind: kind})
		if len(v.Logs) > 200 {
			v.Logs = v.Logs[len(v.Logs)-200:]
		}
	}
}

// resetForNextSelection 清除上一段进度，同时保留累计日志与登录信息。
func (s *Server) resetForNextSelection() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.taskFinished = false
	s.state.Course = ""
	s.state.PracticeURL = ""
	s.state.ChatRound = ""
	s.state.StepNo = 0
	s.state.StepName = ""
	s.state.QType = ""
	s.state.Index = 0
	s.state.Total = 0
	s.state.Desc = ""
	s.state.Framework = ""
	s.state.AIStatus = "idle"
	s.state.AIAnswer = ""
	s.state.PromptLen = 0
	s.state.Attempt = 0
	s.state.FinalCode = ""
	s.state.EvalPassed = nil
	s.state.EvalDetail = ""
	s.state.StartedAt = float64(time.Now().UnixMilli()) / 1000
	s.state.UpdatedAt = s.state.StartedAt
}

// ServeHTTP 提供内嵌 HTML 和不缓存的状态 JSON。
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	switch r.URL.Path {
	case "/state":
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		s.mu.RLock()
		b, err := json.Marshal(s.state)
		s.mu.RUnlock()
		if err != nil {
			http.Error(w, "状态编码失败", 500)
			return
		}
		_, _ = w.Write(b)
	case "/":
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if s.selection != nil {
			s.writeSelectionPage(w)
			return
		}
		_, _ = w.Write(view)
	case "/select":
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		if s.selection == nil {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		s.writeSelectionPage(w)
	case "/api/select/classrooms", "/api/select/courses", "/api/select/steps", "/api/select/start", "/api/select/cancel", "/api/select/continue", "/api/select/close":
		s.serveSelectionAPI(w, r)
	default:
		http.NotFound(w, r)
	}
}

func (s *Server) writeSelectionPage(w http.ResponseWriter) {
	_, _ = w.Write([]byte(strings.Replace(string(selector), "__SELECTION_KEY__", s.selectionKey, 1)))
}

// EnableSelection 为默认浏览器工作流开放带随机会话密钥的选择 API。
func (s *Server) EnableSelection(api SelectionAPI) error {
	if api.Classrooms == nil || api.Courses == nil || api.Steps == nil || api.Start == nil || api.Cancel == nil {
		return fmt.Errorf("面板选择接口不完整")
	}
	var key [32]byte
	if _, err := rand.Read(key[:]); err != nil {
		return fmt.Errorf("创建面板会话密钥失败: %w", err)
	}
	s.selectionKey = hex.EncodeToString(key[:])
	s.selection = &api
	return nil
}

func (s *Server) serveSelectionAPI(w http.ResponseWriter, r *http.Request) {
	if s.selection == nil {
		http.NotFound(w, r)
		return
	}
	if !sameOrigin(r) {
		http.Error(w, "请求来源无效", http.StatusForbidden)
		return
	}
	api := s.selection
	s.selectionMu.Lock()
	defer s.selectionMu.Unlock()
	ctx := r.Context()
	if r.URL.Path == "/api/select/close" {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		if r.Header.Get("X-Selection-Key") != s.selectionKey {
			http.Error(w, "面板会话无效", http.StatusForbidden)
			return
		}
		s.mu.RLock()
		finished := s.taskFinished
		s.mu.RUnlock()
		if !finished {
			http.Error(w, "任务尚未结束", http.StatusConflict)
			return
		}
		if api.Close == nil {
			http.Error(w, "退出接口不可用", http.StatusNotFound)
			return
		}
		if err := api.Close(ctx); err != nil {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		w.WriteHeader(http.StatusAccepted)
		return
	}
	if r.URL.Path == "/api/select/continue" {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		if r.Header.Get("X-Selection-Key") != s.selectionKey {
			http.Error(w, "面板会话无效", http.StatusForbidden)
			return
		}
		s.mu.RLock()
		finished := s.taskFinished
		s.mu.RUnlock()
		if !finished || !s.selectionDone {
			http.Error(w, "当前范围尚未完成", http.StatusConflict)
			return
		}
		if api.Continue == nil {
			http.Error(w, "继续选择接口不可用", http.StatusNotFound)
			return
		}
		if err := api.Continue(ctx); err != nil {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		s.selectionDone = false
		s.resetForNextSelection()
		w.WriteHeader(http.StatusAccepted)
		return
	}
	if s.selectionDone {
		http.Error(w, "本次范围选择已结束", http.StatusGone)
		return
	}
	switch r.URL.Path {
	case "/api/select/classrooms":
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		v, err := api.Classrooms(ctx)
		writeSelectionResult(w, v, err)
	case "/api/select/courses":
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		v, err := api.Courses(ctx, r.URL.Query().Get("classroom_id"))
		writeSelectionResult(w, v, err)
	case "/api/select/steps":
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		v, err := api.Steps(ctx, r.URL.Query().Get("classroom_id"), r.URL.Query().Get("course_id"))
		writeSelectionResult(w, v, err)
	case "/api/select/start":
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		if r.Header.Get("X-Selection-Key") != s.selectionKey {
			http.Error(w, "面板会话无效", http.StatusForbidden)
			return
		}
		var request RunSelection
		if err := decodeSelectionRequest(w, r, &request); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := request.Validate(); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := api.Start(ctx, request); err != nil {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		s.selectionDone = true
		w.WriteHeader(http.StatusAccepted)
	case "/api/select/cancel":
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		if r.Header.Get("X-Selection-Key") != s.selectionKey {
			http.Error(w, "面板会话无效", http.StatusForbidden)
			return
		}
		if err := api.Cancel(ctx); err != nil {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		s.selectionDone = true
		w.WriteHeader(http.StatusAccepted)
	}
}

func sameOrigin(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true
	}
	u, err := url.Parse(origin)
	return err == nil && u.Scheme == "http" && u.Host == r.Host && (u.Hostname() == "127.0.0.1" || u.Hostname() == "localhost")
}

func decodeSelectionRequest(w http.ResponseWriter, r *http.Request, out any) error {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(out); err != nil {
		return fmt.Errorf("选择数据格式错误: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return fmt.Errorf("选择数据包含多余内容")
	}
	return nil
}

func writeSelectionResult(w http.ResponseWriter, value any, err error) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	if err != nil {
		w.WriteHeader(http.StatusConflict)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	if err = json.NewEncoder(w).Encode(value); err != nil {
		http.Error(w, "数据编码失败", http.StatusInternalServerError)
	}
}

// Start 仅绑定 loopback，尝试连续 12 个端口，返回实际 URL。
func (s *Server) Start(preferred int) error {
	var ln net.Listener
	var err error
	for i := 0; i < 12; i++ {
		ln, err = net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", preferred+i))
		if err == nil {
			break
		}
	}
	if err != nil {
		return fmt.Errorf("监控面板端口不可用: %w", err)
	}
	s.URL = "http://" + ln.Addr().String() + "/"
	s.srv = &http.Server{Handler: s, ReadHeaderTimeout: 5 * time.Second}
	go func() { _ = s.srv.Serve(ln) }()
	return nil
}

// Close 在调用者限定时间内关闭面板服务。
func (s *Server) Close(ctx context.Context) error {
	if s.srv == nil {
		return nil
	}
	return s.srv.Shutdown(ctx)
}
