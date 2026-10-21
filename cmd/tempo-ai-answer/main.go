// tempo-ai-answer 是纯 Go 程序，交互面板与配置模板内嵌，无生产 Python 依赖。
package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
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
	reader                            *bufio.Reader
	writer                            io.Writer
	dataDir, configPath               string
	logPath                           string
	scope, course, steps, classroomID string
	limit                             int
	headless, demo, dry, yes, local   bool
	inMenu                            bool
	includePassed                     bool
	webPanel, webConfirmed            bool
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	root := command()
	if e := root.ExecuteContext(ctx); e != nil {
		if errors.Is(e, context.Canceled) {
			fmt.Fprintln(os.Stderr, "操作已停止，可以重新启动软件。")
		} else {
			fmt.Fprintln(os.Stderr, "错误：", e)
		}
		os.Exit(1)
	}
}
func command() *cobra.Command {
	a := &app{reader: bufio.NewReader(os.Stdin)}
	root := &cobra.Command{Use: "tempo-ai-answer", Short: "Tempo 实训助手 · Go 跨平台版", Version: version + " (" + commit + ")", SilenceUsage: true, SilenceErrors: true, RunE: func(cmd *cobra.Command, _ []string) error {
		a.reader, a.writer = bufio.NewReader(cmd.InOrStdin()), cmd.OutOrStdout()
		return a.execute(cmd.Context(), "panel")
	}}
	root.PersistentFlags().StringVar(&a.dataDir, "data-dir", "", "profile 和日志目录；显式指定且未传 --config 时也从此处读取配置")
	root.PersistentFlags().StringVar(&a.configPath, "config", "", "配置文件路径（默认程序同目录的 config.json），兼容 v1 配置")
	root.PersistentFlags().StringVar(&a.classroomID, "classroom", "", "课堂 ID（使用 classrooms 查看），优先于配置文件")
	for _, name := range []string{"login", "classrooms", "list", "ai-test"} {
		n := name
		root.AddCommand(&cobra.Command{Use: n, Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error { return a.execute(cmd.Context(), n) }})
	}
	root.AddCommand(&cobra.Command{Use: "menu", Short: "打开终端交互菜单", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		a.reader, a.writer = bufio.NewReader(cmd.InOrStdin()), cmd.OutOrStdout()
		return a.menu(cmd)
	}})
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
	fmt.Fprintf(a.output(), "[就绪] 版本：%s\n[就绪] 数据目录：%s\n[配置路径] %s\n[就绪] 内嵌配置和面板资源\n", version, a.dataDir, a.configPath)
	cs := browser.Candidates()
	if len(cs) == 0 {
		return errors.New("未找到浏览器；当前为轻量包，需要 Edge/Chrome 或完整 Chromium 包")
	}
	for _, p := range cs {
		fmt.Fprintln(a.output(), "[就绪] 浏览器：", p)
	}
	return nil
}
func (a *app) execute(ctx context.Context, name string) (runErr error) {
	a.logPath = ""
	if name == "run" {
		if err := a.validateRunSelection(); err != nil {
			return err
		}
	}
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
		a.showConfigCreated()
		return nil
	}
	if e != nil {
		return &configFileError{path: a.configPath, err: e}
	}
	if a.includePassed {
		cfg.Run.SkipPassed = false
	}
	fmt.Fprintf(a.output(), "[就绪] 配置文件：%s\n", a.configPath)
	secrets := []string{cfg.Platform.Password, cfg.Platform.Username, cfg.AI.APIKey, os.Getenv("GITHUB_TOKEN")}
	if cfg.AI.APIKeyEnv != "" {
		secrets = append(secrets, os.Getenv(cfg.AI.APIKeyEnv))
	}
	l, e := logger.OpenWithConsole(filepath.Join(a.dataDir, "logs"), a.output(), secrets...)
	if e != nil {
		return e
	}
	a.logPath = a.logFile()
	defer func() {
		if runErr != nil && !errors.Is(runErr, errSelectionCanceled) {
			l.RecordError(runErr)
		}
		if err := l.Close(); err != nil {
			fmt.Fprintln(os.Stderr, "日志关闭失败：", err)
		}
	}()
	panel := dashboard.New()
	sink := core.Sink(func(e core.Event) {
		e.Text = l.Redact(e.Text)
		if e.Kind == "error" {
			l.RecordError(errors.New(e.Text))
			message := describeFailure(errors.New(e.Text))
			e.Text = message.title + "。" + message.hint
			fmt.Fprintln(a.output(), "[未完成]", e.Text)
		} else {
			l.Consume(e)
		}
		panel.Consume(e)
	})
	if name == "ai-test" && cfg.AI.Provider == "openai_compatible" {
		return (&runtimeState{cfg: cfg, log: l, panel: panel, sink: sink}).aiTest(ctx)
	}
	session, e := browser.Start(context.WithoutCancel(ctx), filepath.Join(a.dataDir, "profile"), name == "classrooms" || name == "list" || name == "check" || name == "run" && a.headless)
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
	fmt.Fprintf(a.output(), "[就绪] 平台已登录：%s\n", me.Name)
	r.me = me
	if name == "panel" || name == "run" {
		stopSessionMonitor := r.startPlatformSessionMonitor(ctx)
		defer stopSessionMonitor()
	}
	if name == "login" {
		return r.loginAI(ctx, a.enter)
	}
	if name == "classrooms" {
		return r.classrooms(ctx)
	}
	if name == "run" {
		fmt.Fprintln(a.output(), "\n第 1 步 · 选择课堂")
	}
	if name == "panel" {
		return r.panelRun(ctx, a)
	}
	classroom, e := a.selectClassroom(ctx, plat, cfg.Platform.ClassroomID)
	if errors.Is(e, errSelectionCanceled) {
		fmt.Fprintln(a.output(), "已取消，未开始任务。")
		return nil
	}
	if e != nil {
		return e
	}
	plat.ClassroomID = string(classroom.ID)
	r.classroom = strings.TrimSpace(classroom.Name + " " + classroom.Class)
	fmt.Fprintf(a.output(), "[OK] 已选课堂：%s\n", classroom.Name)
	if name == "list" {
		return r.list(ctx)
	}
	if name == "check" {
		return r.check(ctx)
	}
	e = r.run(ctx, a, selection, pick)
	if errors.Is(e, errSelectionCanceled) {
		fmt.Fprintln(a.output(), "已取消，未开始任务。")
		return nil
	}
	return e
}

func (a *app) enter(ctx context.Context) error {
	_, err := a.readLine(ctx, "")
	return err
}
func (a *app) menu(cmd *cobra.Command) error {
	a.inMenu = true
	if e := a.paths(); e != nil {
		return e
	}
	a.showWelcome()
	for {
		a.showMenu()
		choice, e := a.readLine(cmd.Context(), "输入功能编号 > ")
		if e != nil {
			if errors.Is(e, io.EOF) {
				return nil
			}
			return e
		}
		choice = strings.TrimSpace(choice)
		if choice == "0" {
			fmt.Fprintln(a.output(), "已退出，可以关闭窗口。")
			return nil
		}
		name, ok := a.menuCommand(choice)
		if !ok {
			fmt.Fprintln(a.output(), "请输入 0 到 7 的功能编号，例如输入 7 测试一关。")
			continue
		}
		a.logPath = ""
		if name == "check" {
			e = a.localCheck()
		} else {
			if name == "run" {
				fmt.Fprintln(a.output(), "\n本次模式：", a.modeLabel())
				if a.limit > 0 {
					fmt.Fprintf(a.output(), "最多处理 %d 关。\n", a.limit)
				}
			}
			e = a.execute(cmd.Context(), name)
		}
		if e != nil {
			a.showFailure(e)
		}
		if cmd.Context().Err() != nil {
			return cmd.Context().Err()
		}
	}
}
