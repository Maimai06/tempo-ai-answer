package main

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/Maimai06/tempo-ai-answer/internal/ai"
	"github.com/Maimai06/tempo-ai-answer/internal/browser"
	"github.com/Maimai06/tempo-ai-answer/internal/config"
	"github.com/Maimai06/tempo-ai-answer/internal/core"
	"github.com/Maimai06/tempo-ai-answer/internal/dashboard"
	"github.com/Maimai06/tempo-ai-answer/internal/logger"
	"github.com/Maimai06/tempo-ai-answer/internal/platform"
	"github.com/Maimai06/tempo-ai-answer/internal/scope"
	"github.com/Maimai06/tempo-ai-answer/internal/solver"
)

// runtimeState 集中持有本次命令的依赖，不使用全局配置。
type runtimeState struct {
	cfg       config.Config
	log       *logger.Logger
	panel     *dashboard.Server
	session   *browser.Session
	plat      *platform.Client
	sink      core.Sink
	me        core.User
	classroom string
}

func (r *runtimeState) aiTest(ctx context.Context) error {
	cfg, session, sink := r.cfg, r.session, r.sink
	var e error
	web := ai.New(session.NewPage(), time.Duration(cfg.AI.AnswerTimeout)*time.Second, cfg.Run.PerChat, sink)
	if e = web.Open(ctx); e != nil {
		return e
	}
	ok, e := web.LoggedIn(ctx)
	if e != nil {
		return e
	}
	if !ok {
		return errors.New("DeepSeek 尚未登录，请先运行 login")
	}
	ans, e := web.Ask(ctx, "请用 Java 输出 Hello World，只输出一个 ```java 代码块，不要解释。", 1)
	if e != nil {
		return e
	}
	fmt.Println(ans)
	return nil
}
func (r *runtimeState) loginAI(ctx context.Context, confirm func(context.Context) error) error {
	cfg, session, sink := r.cfg, r.session, r.sink
	var e error
	web := ai.New(session.NewPage(), time.Duration(cfg.AI.AnswerTimeout)*time.Second, cfg.Run.PerChat, sink)
	if e = web.Open(ctx); e != nil {
		return e
	}
	fmt.Println("请在浏览器中完成 DeepSeek 登录，然后按回车继续。")
	if e = confirm(ctx); e != nil {
		return e
	}
	ok, e := web.LoggedIn(ctx)
	if e != nil {
		return e
	}
	if !ok {
		return errors.New("未检测到 DeepSeek 登录态")
	}
	fmt.Println("[OK] 登录态已保存在本工具独立 profile")
	return nil
}
func (r *runtimeState) list(ctx context.Context) error {
	plat := r.plat
	for _, kind := range []string{"practice", "training", "combat"} {
		courses, e := plat.ListCourses(ctx, kind)
		if e != nil {
			return e
		}
		fmt.Printf("\n【%s】共 %d 个\n", kind, len(courses))
		for _, c := range courses {
			fmt.Printf("  %s %s\n", c.Number(), c.Label())
			steps, e := plat.ListSteps(ctx, c.Identifier())
			if e != nil {
				fmt.Println("    （实践环境未启动或暂不可读，run 会自动初始化）")
				continue
			}
			for _, s := range steps {
				flag := "·"
				if s.Finished {
					flag = "✓"
				}
				fmt.Printf("    %s 第%d关 %s [%s]\n", flag, s.Sequence, s.Name, s.ID)
			}
		}
	}
	return nil
}
func (r *runtimeState) check(ctx context.Context) error {
	cfg, session, sink, plat := r.cfg, r.session, r.sink, r.plat
	courses, e := plat.ListCourses(ctx, "practice")
	if e != nil {
		return e
	}
	fmt.Printf("[OK] 课堂：%s，实验数：%d\n", plat.ClassroomID, len(courses))
	web := ai.New(session.NewPage(), time.Duration(cfg.AI.AnswerTimeout)*time.Second, cfg.Run.PerChat, sink)
	if e = web.Open(ctx); e != nil {
		return e
	}
	ok, e := web.LoggedIn(ctx)
	if e != nil {
		return e
	}
	fmt.Println("DeepSeek 登录态：", ok)
	return nil
}

func (r *runtimeState) classrooms(ctx context.Context) error {
	classrooms, err := r.plat.ListClassrooms(ctx)
	if err != nil {
		return err
	}
	if len(classrooms) == 0 {
		return errors.New("没有找到已加入的课堂")
	}
	for _, classroom := range classrooms {
		fmt.Printf("%s  %s  %s\n", classroom.ID, classroom.Name, classroom.Class)
	}
	return nil
}
func (r *runtimeState) run(ctx context.Context, a *app, selection scope.Scope, pick []int) error {
	cfg, session, plat, me, panel, sink, l := r.cfg, r.session, r.plat, r.me, r.panel, r.sink, r.log
	courses, e := plat.ListCourses(ctx, "practice")
	if e != nil {
		return e
	}
	plan, pick, e := a.selectPlan(ctx, courses, selection, pick)
	if e != nil {
		return e
	}
	a.showRunPlan(r.classroom, plan, pick)
	if !a.yes {
		if e = a.confirmRun(ctx); e != nil {
			return e
		}
	}
	if e = panel.Start(8877); e != nil {
		return e
	}
	defer func() {
		closeCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		if e := panel.Close(closeCtx); e != nil {
			l.Log.Warn("面板关闭失败")
		}
	}()
	fmt.Fprintln(a.output(), "监控面板：", panel.URL)
	if !a.headless {
		if e = session.NewPage().OpenLocal(ctx, panel.URL); e != nil {
			l.Log.Warn("面板未能自动打开，请访问上述地址")
		}
	}
	var provider core.AIProvider
	if a.demo {
		provider = &ai.Demo{Sink: sink}
	} else {
		web := ai.New(session.NewPage(), time.Duration(cfg.AI.AnswerTimeout)*time.Second, cfg.Run.PerChat, sink)
		if e = web.Open(ctx); e != nil {
			return e
		}
		ok, e := web.LoggedIn(ctx)
		if e != nil {
			return e
		}
		if !ok {
			if a.headless {
				return errors.New("DeepSeek 尚未登录，请先运行 login")
			}
			fmt.Println("请在浏览器完成 DeepSeek 登录，然后按回车继续。")
			if e = a.enter(ctx); e != nil {
				return e
			}
			ok, e = web.LoggedIn(ctx)
			if e != nil {
				return e
			}
			if !ok {
				return errors.New("未检测到 DeepSeek 登录态")
			}
		}
		if e = web.Capture(ctx); e != nil {
			sink.Emit(core.Event{Kind: "warn", Text: "获取历史对话失败；本次禁用删除清理"})
		}
		if cfg.Run.CleanupChat {
			defer func() {
				cleanupCtx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
				defer cancel()
				n, e := web.Cleanup(cleanupCtx)
				if e != nil {
					sink.Emit(core.Event{Kind: "warn", Text: "对话清理失败：" + e.Error()})
				} else {
					sink.Emit(core.Event{Kind: "log", Text: fmt.Sprintf("已清理 %d 个新对话", n)})
				}
			}()
		}
		provider = web
	}
	engine := solver.Engine{Platform: plat, AI: provider, Sink: sink, MaxRetry: cfg.Run.MaxRetry, DryRun: a.dry, Demo: a.demo, PerChat: cfg.Run.PerChat}
	keep := func(s core.StepRef) bool {
		if cfg.Run.SkipPassed && s.Finished {
			return false
		}
		if len(pick) == 0 {
			return true
		}
		for _, n := range pick {
			if n == s.Sequence {
				return true
			}
		}
		return false
	}
	total := 0
	for _, c := range plan {
		steps, e := plat.ListSteps(ctx, c.Identifier())
		if e != nil {
			continue
		}
		for _, s := range steps {
			if keep(s) {
				total++
			}
		}
	}
	if a.limit > 0 && (total == 0 || total > a.limit) {
		total = a.limit
	}
	index, passed := 0, 0
	for _, c := range plan {
		if a.limit > 0 && index >= a.limit {
			break
		}
		if e = ctx.Err(); e != nil {
			return e
		}
		fmt.Fprintf(a.output(), "\n正在准备实验：[%s] %s\n", c.Number(), c.Label())
		steps, e := plat.Prepare(ctx, c.Identifier())
		if e != nil {
			sink.Emit(core.Event{Kind: "warn", Text: "实践环境启动失败：" + e.Error()})
			continue
		}
		sort.SliceStable(steps, func(i, j int) bool { return steps[i].Sequence < steps[j].Sequence })
		practice := strings.TrimRight(cfg.Platform.BaseURL, "/") + "/#/practicum/practiceDetail?id=" + c.Identifier() + "&classroomId=" + plat.ClassroomID + "&tenancyId=" + plat.ClassroomID
		for _, s := range steps {
			if !keep(s) {
				continue
			}
			if a.limit > 0 && index >= a.limit {
				break
			}
			index++
			fmt.Fprintf(a.output(), "[进度] 正在处理第 %d 关 · %s\n", s.Sequence, s.Name)
			panel.Overview(me.Name, c.Number()+" "+c.Label(), practice, index, total)
			ok, e := engine.Solve(ctx, string(s.ID), s.Sequence)
			if e != nil {
				if ctx.Err() != nil {
					return ctx.Err()
				}
				sink.Emit(core.Event{Kind: "error", Text: e.Error()})
			}
			if ok {
				passed++
			}
			if e = core.Wait(ctx, time.Duration(cfg.Run.Delay)*time.Second); e != nil {
				return e
			}
		}
	}
	a.showRunSummary(index, passed)
	return nil
}
