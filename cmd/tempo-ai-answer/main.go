// tempo-ai-answer 是纯 Go CLI，面板与配置模板内嵌，无生产 Python 依赖。
package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"

	"github.com/Maimai06/tempo-ai-answer/internal/browser"
	"github.com/Maimai06/tempo-ai-answer/internal/config"
	"github.com/Maimai06/tempo-ai-answer/internal/core"
	"github.com/Maimai06/tempo-ai-answer/internal/dashboard"
	"github.com/Maimai06/tempo-ai-answer/internal/logger"
	"github.com/Maimai06/tempo-ai-answer/internal/platform"
	"github.com/Maimai06/tempo-ai-answer/internal/scope"
	"github.com/spf13/cobra"
)

var version = "dev"
var commit = "unknown"

type app struct {
	reader                          *bufio.Reader
	dataDir, configPath             string
	scope, course, steps            string
	limit                           int
	headless, demo, dry, yes, local bool
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	root := command()
	if e := root.ExecuteContext(ctx); e != nil {
		fmt.Fprintln(os.Stderr, "错误：", e)
		os.Exit(1)
	}
}
func command() *cobra.Command {
	a := &app{reader: bufio.NewReader(os.Stdin)}
	root := &cobra.Command{Use: "tempo-ai-answer", Short: "Tempo 实训助手 · Go 跨平台版", Version: version + " (" + commit + ")", SilenceUsage: true, SilenceErrors: true, RunE: func(cmd *cobra.Command, _ []string) error { return a.menu(cmd) }}
	root.PersistentFlags().StringVar(&a.dataDir, "data-dir", "", "profile 和日志目录；显式指定且未传 --config 时也从此处读取配置")
	root.PersistentFlags().StringVar(&a.configPath, "config", "", "配置文件路径（默认程序同目录的 config.json），兼容 v1 配置")
	for _, name := range []string{"login", "list", "ai-test"} {
		n := name
		root.AddCommand(&cobra.Command{Use: n, Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error { return a.execute(cmd.Context(), n) }})
	}
	check := &cobra.Command{Use: "check", Short: "本地检查；不加 --local 时另检查远端登录", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		if e := a.localCheck(); e != nil {
			return e
		}
		if a.local {
			return nil
		}
		return a.execute(cmd.Context(), "check")
	}}
	check.Flags().BoolVar(&a.local, "local", false, "仅本地检查，不读取账号、不访问平台或 AI")
	root.AddCommand(check)
	run := &cobra.Command{Use: "run", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error { return a.execute(cmd.Context(), "run") }}
	f := run.Flags()
	f.StringVar(&a.scope, "scope", "", "如 第1章 1-5 / 1-3")
	f.StringVar(&a.course, "course", "", "实验名包含关键字")
	f.StringVar(&a.steps, "steps", "", "关卡范围，如 1-3,8")
	f.IntVar(&a.limit, "limit", 0, "最多处理关卡数，0 为不限")
	f.BoolVar(&a.headless, "headless", false, "后台运行，不自动打开面板")
	f.BoolVar(&a.demo, "demo", false, "假 AI，强制不提交")
	f.BoolVar(&a.dry, "dry-run", false, "真 AI，仅生成不提交")
	f.BoolVar(&a.yes, "yes", false, "已核对计划，直接开始")
	root.AddCommand(run)
	return root
}
func (a *app) paths() error {
	if a.configPath == "" {
		if a.dataDir != "" {
			a.configPath = filepath.Join(a.dataDir, "config.json")
		} else {
			exe, e := os.Executable()
			if e != nil {
				return fmt.Errorf("定位程序目录失败: %w", e)
			}
			a.configPath = filepath.Join(filepath.Dir(exe), "config.json")
		}
	}
	if a.dataDir == "" {
		d, e := config.DataDir()
		if e != nil {
			return e
		}
		a.dataDir = d
	}
	var e error
	if a.configPath, e = filepath.Abs(a.configPath); e != nil {
		return fmt.Errorf("定位配置文件失败: %w", e)
	}
	a.dataDir, e = filepath.Abs(a.dataDir)
	return e
}
func (a *app) localCheck() error {
	if e := a.paths(); e != nil {
		return e
	}
	if e := os.MkdirAll(a.dataDir, 0700); e != nil {
		return e
	}
	d, e := os.MkdirTemp(a.dataDir, ".write-check-")
	if e != nil {
		return e
	}
	if e = os.Remove(d); e != nil {
		return e
	}
	fmt.Printf("[OK] %s (%s)\n[OK] 数据目录：%s\n[配置路径] %s\n[OK] 内嵌配置和面板资源\n", version, commit, a.dataDir, a.configPath)
	cs := browser.Candidates()
	if len(cs) == 0 {
		return errors.New("未找到浏览器；当前为轻量包，需要 Edge/Chrome 或完整 Chromium 包")
	}
	for _, p := range cs {
		fmt.Println("[OK] 浏览器候选：", p)
	}
	return nil
}
func (a *app) execute(ctx context.Context, name string) error {
	if e := a.paths(); e != nil {
		return e
	}
	selection, e := scope.Parse(a.scope)
	if e != nil {
		return e
	}
	pick, e := scope.ParseRange(a.steps)
	if e != nil {
		return e
	}
	if a.limit < 0 {
		return errors.New("limit 不能小于 0")
	}
	cfg, e := config.Load(a.configPath)
	if errors.Is(e, config.ErrCreated) {
		fmt.Printf("%s\n%s\n", e, a.configPath)
		return nil
	}
	if e != nil {
		return fmt.Errorf("配置文件 %s: %w", a.configPath, e)
	}
	fmt.Printf("[OK] 配置文件：%s\n", a.configPath)
	l, e := logger.Open(filepath.Join(a.dataDir, "logs"), cfg.Platform.Password, cfg.Platform.Username, os.Getenv("GITHUB_TOKEN"))
	if e != nil {
		return e
	}
	defer func() {
		if err := l.Close(); err != nil {
			fmt.Fprintln(os.Stderr, "日志关闭失败：", err)
		}
	}()
	panel := dashboard.New()
	sink := core.Sink(func(e core.Event) { e.Text = l.Redact(e.Text); panel.Consume(e); l.Consume(e) })
	session, e := browser.Start(context.WithoutCancel(ctx), filepath.Join(a.dataDir, "profile"), name == "list" || name == "check" || name == "run" && a.headless)
	if e != nil {
		return e
	}
	defer func() {
		if err := session.Close(); err != nil {
			l.Log.Warn("浏览器关闭未完成：" + err.Error())
		}
	}()
	plat := &platform.Client{Page: session.Root, BaseURL: cfg.Platform.BaseURL, Username: cfg.Platform.Username, Password: cfg.Platform.Password, ClassroomID: cfg.Platform.ClassroomID, Log: func(s string) { sink.Emit(core.Event{Kind: "log", Text: s}) }}
	r := &runtimeState{cfg: cfg, log: l, panel: panel, session: session, plat: plat, sink: sink}
	if name == "ai-test" {
		return r.aiTest(ctx)
	}
	me, e := plat.Login(ctx)
	if e != nil {
		return e
	}
	fmt.Printf("[OK] 平台登录：%s\n", me.Name)
	r.me = me
	if name == "login" {
		return r.loginAI(ctx, a.enter)
	}
	if e = plat.EnsureClassroom(ctx); e != nil {
		return e
	}
	if name == "list" {
		return r.list(ctx)
	}
	if name == "check" {
		return r.check(ctx)
	}
	return r.run(ctx, a, selection, pick)
}

func (a *app) enter(ctx context.Context) error {
	result := make(chan error, 1)
	go func() { _, e := a.reader.ReadString('\n'); result <- e }()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case e := <-result:
		return e
	}
}
func (a *app) menu(cmd *cobra.Command) error {
	reader := a.reader
	for {
		fmt.Println("\nTempo AI 助手\n1 首次登录  2 课程清单  3 自动答题  4 演示  5 AI 测试  6 环境自检  0 退出")
		fmt.Print("请选择 > ")
		choice, e := reader.ReadString('\n')
		if e != nil {
			return e
		}
		choice = strings.TrimSpace(choice)
		name := map[string]string{"1": "login", "2": "list", "3": "run", "4": "run", "5": "ai-test", "6": "check"}[choice]
		if choice == "0" {
			return nil
		}
		if name == "" {
			fmt.Println("无效选项")
			continue
		}
		a.demo = choice == "4"
		if name == "run" {
			fmt.Print("范围（空为全部） > ")
			text, e := reader.ReadString('\n')
			if e != nil {
				return e
			}
			a.scope = strings.TrimSpace(text)
		}
		if name == "check" {
			e = a.localCheck()
		} else {
			e = a.execute(cmd.Context(), name)
		}
		if e != nil {
			fmt.Println("操作失败：", e)
		}
		if cmd.Context().Err() != nil {
			return cmd.Context().Err()
		}
	}
}
