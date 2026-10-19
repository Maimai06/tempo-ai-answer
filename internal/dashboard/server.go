// Package dashboard 保持 v1 /state 契约，用事件驱动本机只读监控。
package dashboard

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/Maimai06/tempo-ai-answer/internal/core"
)

//go:embed view.html
var view []byte

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
	mu    sync.RWMutex
	state State
	srv   *http.Server
	URL   string
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

// ServeHTTP 提供内嵌 HTML 和不缓存的状态 JSON。
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	switch r.URL.Path {
	case "/state":
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
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write(view)
	default:
		http.NotFound(w, r)
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
