// Package ai 驱动 DeepSeek 网页，公共接口不泄漏 CDP 私有字段。
package ai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/Maimai06/tempo-ai-answer/internal/browser"
	"github.com/Maimai06/tempo-ai-answer/internal/codemerge"
	"github.com/Maimai06/tempo-ai-answer/internal/core"
)

// Selectors 集中维护页面选择器，避免散布在 solver。
type Selectors struct{ Input, Send, Answer, Busy []string }

// DefaultSelectors 保存当前网页结构的默认选择器。
var DefaultSelectors = Selectors{Input: []string{"textarea", "div[contenteditable='true']"}, Send: []string{"div[role='button'][aria-label*='发送']", "button[aria-label*='发送']"}, Answer: []string{".ds-markdown", "div[class*='ds-markdown']", "div[class*='markdown']"}, Busy: []string{"button[aria-label*='停止']", "div[role='button'][aria-label*='停止']"}}

// ChatSnapshot 显式区分捕获失败与成功捕获空列表。
type ChatSnapshot struct {
	Captured bool
	IDs      map[string]bool
}

// Web 保存单次运行的 AI 标签页与对话状态。
type Web struct {
	Page      *browser.Page
	Selectors Selectors
	Timeout   time.Duration
	PerChat   int
	Sink      core.Sink
	Before    ChatSnapshot
	Created   map[string]bool
	count     int
}

// New 创建具有独立超时、轮换参数的网页 provider。
func New(page *browser.Page, timeout time.Duration, perChat int, sink core.Sink) *Web {
	return &Web{Page: page, Selectors: DefaultSelectors, Timeout: timeout, PerChat: max(1, perChat), Sink: sink}
}

// Open 打开新的 DeepSeek 对话页。
func (w *Web) Open(ctx context.Context) error {
	if e := w.Page.Navigate(ctx, "https://chat.deepseek.com/"); e != nil {
		return e
	}
	return core.Wait(ctx, 4*time.Second)
}
func jsonArg(v any) string { b, _ := json.Marshal(v); return string(b) }

// LoggedIn 检查可见输入框及登录提示。
func (w *Web) LoggedIn(ctx context.Context) (bool, error) {
	var ok bool
	js := `(sels=>{const visible=e=>{const r=e.getBoundingClientRect();return r.width>0&&r.height>0};if([...document.querySelectorAll('button,[role=button],a,span,div')].some(e=>!e.children.length&&['登录','立即登录','去登录','密码登录'].includes((e.innerText||'').trim())&&visible(e)))return false;return sels.some(s=>[...document.querySelectorAll(s)].some(visible))})(` + jsonArg(w.Selectors.Input) + `)`
	e := w.Page.Eval(ctx, js, &ok)
	return ok, e
}

// ListChats 读取侧栏对话链接；DOM 读取失败时返回错误。
func (w *Web) ListChats(ctx context.Context) (map[string]bool, error) {
	var ids []string
	e := w.Page.Eval(ctx, `[...document.querySelectorAll('a[href*="/a/chat/s/"]')].map(a=>a.getAttribute('href'))`, &ids)
	if e != nil {
		return nil, e
	}
	out := map[string]bool{}
	for _, id := range ids {
		if normalized := currentChatID(id); normalized != "" {
			out[normalized] = true
		}
	}
	return out, nil
}

// Capture 记录运行前历史，失败时明确清除 Captured。
func (w *Web) Capture(ctx context.Context) error {
	ids, e := w.ListChats(ctx)
	if e != nil {
		w.Before = ChatSnapshot{}
		return e
	}
	w.Before = ChatSnapshot{Captured: true, IDs: ids}
	if w.Created == nil {
		w.Created = map[string]bool{}
	}
	return nil
}

func currentChatID(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || !strings.Contains(u.Path, "/a/chat/s/") {
		return ""
	}
	return u.EscapedPath()
}

func (w *Web) trackCurrentChat(ctx context.Context, previous string) error {
	raw, err := w.Page.URL(ctx)
	if err != nil {
		return err
	}
	id := currentChatID(raw)
	if id == "" || id == previous || w.Before.IDs[id] {
		return nil
	}
	if w.Created == nil {
		w.Created = map[string]bool{}
	}
	w.Created[id] = true
	return nil
}

// CleanupSince 失败捕获时不删除任何历史对话，空列表成功时仍清理。
func CleanupSince(ctx context.Context, before ChatSnapshot, list func(context.Context) (map[string]bool, error), remove func(context.Context, string) error) (int, error) {
	if !before.Captured {
		return 0, nil
	}
	now, e := list(ctx)
	if e != nil {
		return 0, e
	}
	n := 0
	for id := range now {
		if !before.IDs[id] {
			if e = remove(ctx, id); e != nil {
				return n, e
			}
			n++
		}
	}
	return n, nil
}

// CleanupOwned 只删除本次 provider 确认创建的会话。
func CleanupOwned(ctx context.Context, owned map[string]bool, list func(context.Context) (map[string]bool, error), remove func(context.Context, string) error) (int, error) {
	if len(owned) == 0 {
		return 0, nil
	}
	now, err := list(ctx)
	if err != nil {
		return 0, err
	}
	removed := 0
	for id := range owned {
		if !now[id] {
			continue
		}
		if err := remove(ctx, id); err != nil {
			return removed, err
		}
		removed++
	}
	return removed, nil
}

// Cleanup 只清理成功捕获历史之后的新对话。
func (w *Web) Cleanup(ctx context.Context) (int, error) {
	return CleanupOwned(ctx, w.Created, w.ListChats, w.remove)
}
func (w *Web) remove(ctx context.Context, id string) error {
	var ok bool
	js := `(href=>{const a=[...document.querySelectorAll('a[href]')].find(a=>a.getAttribute('href')===href);if(!a)return false;a.scrollIntoView();a.dispatchEvent(new MouseEvent('mouseover',{bubbles:true}));const b=a.querySelector('div[role=button],button');if(!b)return false;b.click();return true})(` + jsonArg(id) + `)`
	if e := w.Page.Eval(ctx, js, &ok); e != nil {
		return e
	}
	if !ok {
		return errors.New("找不到对话删除按钮，保留对话")
	}
	if e := core.Wait(ctx, 600*time.Millisecond); e != nil {
		return e
	}
	click := func(label string) (bool, error) {
		var clicked bool
		e := w.Page.Eval(ctx, `(label=>{const els=[...document.querySelectorAll('button,[role=button],span,div')].filter(e=>!e.children.length&&(e.innerText||'').trim()===label&&e.getBoundingClientRect().width>0);if(!els.length)return false;els.at(-1).click();return true})(`+jsonArg(label)+`)`, &clicked)
		return clicked, e
	}
	ok, e := click("删除")
	if e != nil {
		return e
	}
	if !ok {
		return errors.New("删除菜单不可用，保留对话")
	}
	if e = core.Wait(ctx, 800*time.Millisecond); e != nil {
		return e
	}
	if _, e = click("删除该对话"); e != nil {
		return e
	}
	return core.Wait(ctx, 1200*time.Millisecond)
}

// BeginQuestion 开始新题，必要时回收本次对话并重开。
func (w *Web) BeginQuestion(ctx context.Context) (int, error) {
	if w.count >= w.PerChat {
		u, e := w.Page.URL(ctx)
		if e != nil {
			return 0, e
		}
		if href := currentChatID(u); href != "" && w.Created[href] {
			if e = w.remove(ctx, href); e != nil {
				w.Sink.Emit(core.Event{Kind: "warn", Text: e.Error()})
			} else {
				delete(w.Created, href)
			}
		}
		if e = w.Open(ctx); e != nil {
			return 0, e
		}
		w.count = 0
	}
	w.count++
	return w.count, nil
}

// ReadLatest 读取最新回答，移除代码工具栏噪声。
func (w *Web) ReadLatest(ctx context.Context) (string, error) {
	var text string
	js := `(sels=>{for(const s of sels){const els=[...document.querySelectorAll(s)].filter(e=>(e.innerText||e.textContent||'').trim().length>0);if(!els.length)continue;const clone=els.at(-1).cloneNode(true);clone.querySelectorAll('[class*="code-block-banner"],[class*="banner-wrap"],[class*="toolbar"]').forEach(n=>n.remove());const holder=document.createElement('div');holder.style.cssText='position:absolute;left:-99999px;top:0;width:900px';holder.appendChild(clone);document.body.appendChild(holder);const inner=(holder.innerText||'').trim(),tc=(holder.textContent||'').trim();holder.remove();const out=inner.length>=tc.length*.9?inner:tc;if(out)return out}return ''})(` + jsonArg(w.Selectors.Answer) + `)`
	e := w.Page.Eval(ctx, js, &text)
	return strings.TrimSpace(text), e
}

// pageDiagnostics 只记录页面可见状态与节点数量，不读取或输出回答正文。
func (w *Web) pageDiagnostics(ctx context.Context) string {
	var state struct {
		Visibility  string `json:"visibility"`
		Hidden      bool   `json:"hidden"`
		ReadyState  string `json:"readyState"`
		InputCount  int    `json:"inputCount"`
		AnswerCount int    `json:"answerCount"`
		Generating  bool   `json:"generating"`
		LoginPrompt bool   `json:"loginPrompt"`
		Captcha     bool   `json:"captcha"`
		RateLimited bool   `json:"rateLimited"`
	}
	args := map[string]any{"inputs": w.Selectors.Input, "answers": w.Selectors.Answer, "busy": w.Selectors.Busy}
	js := `(a=>{const visible=e=>{const r=e.getBoundingClientRect();return r.width>0&&r.height>0};const body=(document.body?.innerText||'').toLowerCase();const answers=new Set(a.answers.flatMap(s=>[...document.querySelectorAll(s)]));return {visibility:document.visibilityState,hidden:document.hidden,readyState:document.readyState,inputCount:a.inputs.reduce((n,s)=>n+[...document.querySelectorAll(s)].filter(visible).length,0),answerCount:answers.size,generating:a.busy.some(s=>[...document.querySelectorAll(s)].some(visible)),loginPrompt:/立即登录|去登录|密码登录/.test(body),captcha:/验证码|captcha/.test(body),rateLimited:/请求过于频繁|操作过于频繁|too many requests/.test(body)}})(` + jsonArg(args) + `)`
	if err := w.Page.Eval(ctx, js, &state); err != nil {
		return "页面状态无法读取"
	}
	visibility := "前台"
	if state.Hidden || state.Visibility != "visible" {
		visibility = "后台"
	}
	return fmt.Sprintf("页面=%s, ready=%s, 输入框=%d, 回答节点=%d, 生成中=%t, 登录提示=%t, 验证码=%t, 限流提示=%t", visibility, state.ReadyState, state.InputCount, state.AnswerCount, state.Generating, state.LoginPrompt, state.Captcha, state.RateLimited)
}

func (w *Web) messageCount(ctx context.Context) (int, error) {
	var n int
	e := w.Page.Eval(ctx, `(sels=>{for(const s of sels){const n=document.querySelectorAll(s).length;if(n)return n}return 0})(`+jsonArg(w.Selectors.Answer)+`)`, &n)
	return n, e
}
func (w *Web) busy(ctx context.Context) (bool, error) {
	var busy bool
	e := w.Page.Eval(ctx, `(sels=>sels.some(s=>[...document.querySelectorAll(s)].slice(0,4).some(e=>{const r=e.getBoundingClientRect();return r.width>0&&r.height>0})))(`+jsonArg(w.Selectors.Busy)+`)`, &busy)
	return busy, e
}
func (w *Web) send(ctx context.Context, prompt string) error {
	args := map[string]any{"prompt": prompt, "inputs": w.Selectors.Input, "send": w.Selectors.Send}
	var status int
	js := `(a=>{const visible=e=>{const r=e.getBoundingClientRect();return r.width>0&&r.height>0};let input;for(const s of a.inputs){input=[...document.querySelectorAll(s)].filter(visible).at(-1);if(input)break}if(!input)return 0;input.focus();if(input.tagName==='TEXTAREA'){Object.getOwnPropertyDescriptor(HTMLTextAreaElement.prototype,'value').set.call(input,a.prompt)}else{input.textContent=a.prompt}input.dispatchEvent(new InputEvent('input',{bubbles:true,inputType:'insertText',data:a.prompt}));for(const s of a.send){const el=[...document.querySelectorAll(s)].find(visible);if(el){el.click();return 1}}const button=[...document.querySelectorAll('button,[role=button]')].find(e=>(e.innerText||'').trim()==='发送'&&visible(e));if(button){button.click();return 1}return 2})(` + jsonArg(args) + `)`
	if e := w.Page.Eval(ctx, js, &status); e != nil {
		return e
	}
	if status == 0 {
		return errors.New("DeepSeek 输入框不存在，请先登录或检查页面选择器")
	}
	if status == 2 {
		return w.Page.PressEnter(ctx)
	}
	return nil
}
func (w *Web) wait(ctx context.Context, previous string, count, n int) (string, error) {
	parent := ctx
	ctx, cancel := context.WithTimeout(ctx, w.Timeout)
	defer cancel()
	last := ""
	stable := 0
	start := time.Now()
	for {
		if e := core.Wait(ctx, time.Second); e != nil {
			if parent.Err() == nil && last != "" && (n == 0 || codemerge.Complete(last, n)) {
				return last, nil
			}
			return "", fmt.Errorf("DeepSeek 回答超时或取消: %w；%s", e, w.pageDiagnostics(parent))
		}
		text, e := w.ReadLatest(ctx)
		if e != nil {
			return "", e
		}
		num, e := w.messageCount(ctx)
		if e != nil {
			return "", e
		}
		if text != "" && (text != previous || num > count) {
			if text != last {
				last = text
				stable = 0
				w.Sink.Emit(core.Event{Kind: "answer", Text: text})
			} else {
				stable++
				complete := n == 0 || codemerge.Complete(text, n)
				if stable >= 5 && complete {
					return text, nil
				}
			}
		} else if time.Since(start) > 60*time.Second {
			return "", fmt.Errorf("DeepSeek 60 秒未产生新回答；%s", w.pageDiagnostics(parent))
		}
	}
}

// Ask 发送问题并等待新回答稳定，n=0 表示选择题。
func (w *Web) Ask(ctx context.Context, prompt string, n int) (string, error) {
	for i := 0; i < 20; i++ {
		busy, e := w.busy(ctx)
		if e != nil {
			return "", e
		}
		if !busy {
			break
		}
		if e = core.Wait(ctx, time.Second); e != nil {
			return "", e
		}
	}
	previous, e := w.ReadLatest(ctx)
	if e != nil {
		return "", e
	}
	pageURL, e := w.Page.URL(ctx)
	if e != nil {
		return "", e
	}
	previousChat := currentChatID(pageURL)
	count, e := w.messageCount(ctx)
	if e != nil {
		return "", e
	}
	for attempt := 0; attempt < 2; attempt++ {
		if e = w.send(ctx, prompt); e != nil {
			return "", e
		}
		text, e := w.wait(ctx, previous, count, n)
		if e == nil {
			if trackErr := w.trackCurrentChat(ctx, previousChat); trackErr != nil {
				w.Sink.Emit(core.Event{Kind: "warn", Text: "记录本次 DeepSeek 会话失败，退出时可能无法清理"})
			}
			return text, nil
		}
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		if attempt == 1 {
			return "", e
		}
		w.Sink.Emit(core.Event{Kind: "warn", Text: "未收到可用的 DeepSeek 回复，重新发送一次：" + e.Error()})
	}
	return "", errors.New("AI 重试耗尽")
}

// Demo 不访问 AI 服务；提交保护由 solver 同时实施。
type Demo struct {
	Sink  core.Sink
	count int
}

// BeginQuestion 开始新题，必要时回收本次对话并重开。
func (d *Demo) BeginQuestion(ctx context.Context) (int, error) {
	d.count = d.count%3 + 1
	return d.count, ctx.Err()
}

// ReadLatest 读取最新回答，移除代码工具栏噪声。
func (d *Demo) ReadLatest(context.Context) (string, error) { return "", nil }

// Ask 发送问题并等待新回答稳定，n=0 表示选择题。
func (d *Demo) Ask(ctx context.Context, prompt string, n int) (string, error) {
	if e := ctx.Err(); e != nil {
		return "", e
	}
	text := "```java\nSystem.out.println(\"演示模式生成的示例代码\");\n```"
	if n > 1 {
		parts := []string{}
		for _, m := range regexp.MustCompile(`(?m)^===FILE: (.+)===$`).FindAllStringSubmatch(prompt, -1) {
			parts = append(parts, "===FILE: "+m[1]+"===\npublic class Demo {\n    // 演示模式不会提交\n}")
		}
		text = strings.Join(parts, "\n")
	}
	if n == 0 {
		parts := []string{}
		count := strings.Count(prompt, "【第")
		for i := 1; i <= count; i++ {
			parts = append(parts, fmt.Sprintf("%d. A", i))
		}
		text = strings.Join(parts, "\n")
	}
	d.Sink.Emit(core.Event{Kind: "answer", Text: text})
	return text, nil
}
