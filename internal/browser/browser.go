// Package browser 负责可执行文件查找、CDP 会话与独立 profile。
package browser

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	cdpruntime "github.com/chromedp/cdproto/runtime"
	"github.com/chromedp/chromedp"
)

// Candidates 按 Edge、Chrome、随包 Chromium 排序；显式指定路径优先。
func Candidates() []string {
	out := []string{}
	if p := os.Getenv("TEMPO_BROWSER"); p != "" {
		out = append(out, p)
	}
	switch runtime.GOOS {
	case "darwin":
		for _, base := range []string{"/Applications", filepath.Join(os.Getenv("HOME"), "Applications")} {
			out = append(out, filepath.Join(base, "Microsoft Edge.app/Contents/MacOS/Microsoft Edge"))
		}
		for _, base := range []string{"/Applications", filepath.Join(os.Getenv("HOME"), "Applications")} {
			out = append(out, filepath.Join(base, "Google Chrome.app/Contents/MacOS/Google Chrome"))
		}
	case "windows":
		for _, key := range []string{"PROGRAMFILES", "PROGRAMFILES(X86)", "LOCALAPPDATA"} {
			if base := os.Getenv(key); base != "" {
				out = append(out, filepath.Join(base, "Microsoft/Edge/Application/msedge.exe"))
			}
		}
		for _, key := range []string{"PROGRAMFILES", "PROGRAMFILES(X86)", "LOCALAPPDATA"} {
			if base := os.Getenv(key); base != "" {
				out = append(out, filepath.Join(base, "Google/Chrome/Application/chrome.exe"))
			}
		}
	default:
		for _, name := range []string{"microsoft-edge", "microsoft-edge-stable", "google-chrome", "google-chrome-stable", "chromium", "chromium-browser"} {
			if p, e := exec.LookPath(name); e == nil {
				out = append(out, p)
			}
		}
	}
	exe, e := os.Executable()
	if e == nil {
		base := filepath.Dir(exe)
		for _, rel := range []string{"browser/chrome-linux64/chrome", "browser/chrome-win64/chrome.exe", "browser/chrome-mac-arm64/Google Chrome for Testing.app/Contents/MacOS/Google Chrome for Testing", "browser/chrome-mac-x64/Google Chrome for Testing.app/Contents/MacOS/Google Chrome for Testing"} {
			out = append(out, filepath.Join(base, rel))
		}
	}
	seen := map[string]bool{}
	valid := []string{}
	for _, p := range out {
		if st, e := os.Stat(p); e == nil && !st.IsDir() && !seen[p] {
			seen[p] = true
			valid = append(valid, p)
		}
	}
	return valid
}

// Page 封装标签页，上层不能获取或修改 chromedp 私有上下文。
type Page struct {
	ctx     context.Context
	cancel  context.CancelFunc
	once    sync.Once
	initErr error
}

// Session 拥有浏览器进程与独立 profile 生命周期。
type Session struct {
	Root   *Page
	Path   string
	cancel context.CancelFunc
}

// Start 依次尝试浏览器候选，不在运行时下载任何资源。
func Start(ctx context.Context, profile string, headless bool) (*Session, error) {
	if e := os.MkdirAll(profile, 0700); e != nil {
		return nil, e
	}
	candidates := Candidates()
	if len(candidates) == 0 {
		return nil, fmt.Errorf("没有可用浏览器：请使用含 Chromium 的完整发行包或安装 Edge/Chrome")
	}
	for _, p := range candidates {
		opts := append([]chromedp.ExecAllocatorOption{}, chromedp.DefaultExecAllocatorOptions[:]...)
		opts = append(opts, chromedp.ExecPath(p), chromedp.UserDataDir(profile), chromedp.Flag("headless", headless), chromedp.WindowSize(1440, 950), chromedp.Flag("disable-blink-features", "AutomationControlled"))
		alloc, ac := chromedp.NewExecAllocator(ctx, opts...)
		tab, tc := chromedp.NewContext(alloc)
		page := &Page{ctx: tab, cancel: tc}
		probe, cancel := context.WithTimeout(ctx, 20*time.Second)
		e := page.Run(probe)
		cancel()
		if e == nil {
			return &Session{Root: page, Path: p, cancel: func() { tc(); ac() }}, nil
		}
		tc()
		ac()
	}
	return nil, fmt.Errorf("%d 个浏览器均启动失败，请检查 profile 是否被另一实例占用或运行 check", len(candidates))
}

// Close 释放会话或标签页拥有的资源。
func (s *Session) Close() { s.cancel() }

// NewPage 创建同一独立 profile 内的新标签页。
func (s *Session) NewPage() *Page {
	ctx, cancel := chromedp.NewContext(s.Root.ctx)
	return &Page{ctx: ctx, cancel: cancel}
}

// Close 释放会话或标签页拥有的资源。
func (p *Page) Close() { p.cancel() }

// Run 在已初始化的标签页执行动作，并响应本次操作取消。
func (p *Page) Run(ctx context.Context, actions ...chromedp.Action) error {
	if e := ctx.Err(); e != nil {
		return e
	}
	p.once.Do(func() { stop := context.AfterFunc(ctx, p.cancel); defer stop(); p.initErr = chromedp.Run(p.ctx) })
	if p.initErr != nil {
		return p.initErr
	}
	c, cancel := context.WithCancel(p.ctx)
	stop := context.AfterFunc(ctx, cancel)
	defer stop()
	defer cancel()
	return chromedp.Run(c, actions...)
}

// Eval 等待 Promise 完成，所有动态参数须由调用者 JSON 编码。
func (p *Page) Eval(ctx context.Context, js string, out any) error {
	return p.Run(ctx, chromedp.Evaluate(js, out, func(ep *cdpruntime.EvaluateParams) *cdpruntime.EvaluateParams { return ep.WithAwaitPromise(true) }))
}

// Navigate 导航当前标签页。
func (p *Page) Navigate(ctx context.Context, url string) error {
	return p.Run(ctx, chromedp.Navigate(url))
}

// URL 读取当前页面地址。
func (p *Page) URL(ctx context.Context) (string, error) {
	var s string
	e := p.Run(ctx, chromedp.Location(&s))
	return s, e
}

// OpenLocal 只打开本机 loopback 监控面板。
func (p *Page) OpenLocal(ctx context.Context, url string) error {
	if !strings.HasPrefix(url, "http://127.0.0.1:") {
		return fmt.Errorf("非本地面板 URL")
	}
	return p.Navigate(ctx, url)
}

// PressEnter 使用 CDP 发送受浏览器信任的 Enter，避免 DOM 合成键盘事件失效。
func (p *Page) PressEnter(ctx context.Context) error { return p.Run(ctx, chromedp.KeyEvent("\r")) }
