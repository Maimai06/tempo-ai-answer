package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

type menuAction struct {
	key, title, detail, command string
	demo, dry                   bool
	includePassed               bool
	limit                       int
}

var menuActions = []menuAction{
	{key: "1", title: "登录账号", detail: "完成平台和 DeepSeek 登录", command: "login"},
	{key: "2", title: "查看课程", detail: "查看实验、关卡及完成状态", command: "list"},
	{key: "3", title: "正式运行", detail: "真实 AI 作答，会提交答案和评测", command: "run"},
	{key: "4", title: "流程演示", detail: "模拟 AI 回答，不提交答案", command: "run", demo: true},
	{key: "5", title: "AI 连接测试", detail: "向 DeepSeek 发送一条测试消息", command: "ai-test"},
	{key: "6", title: "本机自检", detail: "检查浏览器与本地文件，无需校园网", command: "check"},
	{key: "7", title: "单关试运行（推荐）", detail: "真实 AI，只测 1 关，不提交答案", command: "run", dry: true, limit: 1, includePassed: true},
}

func (a *app) showWelcome() {
	fmt.Fprintf(a.output(), "\nTempo AI 助手  ·  %s\n", version)
	fmt.Fprintf(a.output(), "配置文件：%s\n", a.configPath)
	fmt.Fprintln(a.output(), "首次使用可先选 6 自检，再选 7 测试一关。")
}

func (a *app) showMenu() {
	fmt.Fprint(a.output(), "\n────────────────────────────────────────\n你想做什么？\n\n")
	for _, action := range menuActions {
		fmt.Fprintf(a.output(), "  %s  %s\n     %s\n", action.key, action.title, action.detail)
	}
	fmt.Fprint(a.output(), "  0  退出\n\n")
}

// menuCommand 每次重置运行模式，避免试运行与正式运行之间残留参数。
func (a *app) menuCommand(choice string) (string, bool) {
	for _, action := range menuActions {
		if action.key == choice {
			a.demo, a.dry, a.limit = action.demo, action.dry, action.limit
			a.includePassed = action.includePassed
			a.scope, a.course, a.steps = "", "", ""
			return action.command, true
		}
	}
	return "", false
}

func (a *app) modeLabel() string {
	if a.demo {
		return "流程演示 · 模拟 AI，不提交答案"
	}
	if a.dry {
		return "真实 AI 试运行 · 不提交答案和评测"
	}
	return "正式运行 · 会提交答案和评测"
}

func (a *app) showRunPlan(classroom string, plan []coursePlan) {
	fmt.Fprintln(a.output(), "\n────────────────────────────────────────\n确认本次运行")
	fmt.Fprintln(a.output(), "模式：", a.modeLabel())
	fmt.Fprintln(a.output(), "课堂：", classroom)
	fmt.Fprintf(a.output(), "实验：已选 %d 个\n", len(plan))
	for _, entry := range plan {
		course := entry.course
		fmt.Fprintf(a.output(), "  [%s] %s\n", course.Number(), course.Label())
		if entry.steps != nil {
			for _, step := range entry.steps {
				fmt.Fprintf(a.output(), "    - %s [%s]\n", stepName(step), stepStatus(step, false))
			}
		} else if len(entry.numbers) > 0 {
			fmt.Fprintf(a.output(), "    关卡编号：%s\n", formatNumbers(entry.numbers))
		} else {
			fmt.Fprintln(a.output(), "    关卡：本实验内所有未跳过的关卡")
		}
	}
	if a.limit > 0 {
		fmt.Fprintf(a.output(), "数量：本次最多处理 %d 关\n", a.limit)
	}
	if a.includePassed {
		fmt.Fprintln(a.output(), "已完成的关卡也可试运行，不会提交答案。")
	}
}

func (a *app) confirmRun(ctx context.Context) error {
	prompt := "\n回车开始；输入 0 取消并退出 > "
	if a.inMenu {
		prompt = "\n回车开始；输入 0 返回主菜单 > "
	}
	for {
		input, err := a.readLine(ctx, prompt)
		if err != nil {
			return err
		}
		switch input {
		case "":
			return nil
		case "0":
			return errSelectionCanceled
		default:
			fmt.Fprintln(a.output(), "尚未开始。请直接按回车，或输入 0 返回。")
		}
	}
}

func (a *app) showRunSummary(processed, succeeded int) {
	fmt.Fprintln(a.output(), "\n本次运行结束")
	if processed == 0 {
		fmt.Fprintln(a.output(), "没有处理任何关卡。请查看课程清单，确认所选关卡编号、完成状态及实践环境。")
		if !a.includePassed {
			fmt.Fprintln(a.output(), "默认会跳过已完成关卡，可以选择一个尚未完成的关卡再试。")
		}
		return
	}
	label := "评测通过"
	if a.dry || a.demo {
		label = "生成答案"
	}
	fmt.Fprintf(a.output(), "处理 %d 关 · %s %d 关 · 未完成 %d 关\n", processed, label, succeeded, processed-succeeded)
	if a.demo {
		fmt.Fprintln(a.output(), "本次使用模拟 AI，未提交答案；可选 7 验证真实 AI。")
	} else if a.dry {
		fmt.Fprintln(a.output(), "本次未提交答案或评测，生成答案不代表平台判分通过。")
	}
}

func formatNumbers(numbers []int) string {
	var parts []string
	for i := 0; i < len(numbers); i++ {
		start := numbers[i]
		end := start
		for i+1 < len(numbers) && numbers[i+1] == end+1 {
			i++
			end = numbers[i]
		}
		part := fmt.Sprint(start)
		if end > start {
			part += fmt.Sprintf("-%d", end)
		}
		parts = append(parts, part)
	}
	return strings.Join(parts, ", ")
}

type configFileError struct {
	path string
	err  error
}

func (e *configFileError) Error() string { return fmt.Sprintf("配置文件 %s: %v", e.path, e.err) }
func (e *configFileError) Unwrap() error { return e.err }

type failureMessage struct{ title, hint string }

// describeFailure 将故障翻译为下一步操作，底层接口和解析错误留在日志中。
func describeFailure(err error) failureMessage {
	var cfg *configFileError
	if errors.As(err, &cfg) {
		return failureMessage{"配置还没有准备好", "请编辑配置文件：" + cfg.path + "\n" + cfg.err.Error()}
	}
	text := strings.ToLower(err.Error())
	var stepErr *stepListError
	switch {
	case errors.Is(err, context.Canceled), errors.Is(err, io.EOF):
		return failureMessage{"操作已停止", "可以重新启动软件并选择需要的功能。"}
	case strings.Contains(text, "数据格式错误"):
		return failureMessage{"暂时无法读取平台数据", "请使用最新版本重试；仍失败时，把下方日志中的错误信息反馈给开发者。"}
	case errors.Is(err, context.DeadlineExceeded), strings.Contains(text, "err_connection"), strings.Contains(text, "failed to fetch"), strings.Contains(text, "err_name_not_resolved"), strings.Contains(text, "timeout"), strings.Contains(text, "超时"):
		return failureMessage{"连接未成功", "请确认已接入校园网，并在浏览器中检查平台和 DeepSeek 是否能打开，再重试。"}
	case errors.As(err, &stepErr):
		return failureMessage{"关卡列表暂不可用", "请在平台检查实验「" + stepErr.course + "」能否打开、实践环境是否已启动，再重新选择。仍失败时请提供日志。"}
	case strings.Contains(text, "deepseek"), strings.Contains(text, "ai 回复"):
		return failureMessage{"AI 暂时不可用", "请先选 1 完成 DeepSeek 登录，再选 5 检查 AI 连接。"}
	case strings.Contains(text, "登录"):
		return failureMessage{"平台登录未完成", "请检查配置中的账号，在弹出的浏览器中完成登录和验证码。"}
	case strings.Contains(text, "未找到浏览器"), strings.Contains(text, "chrome failed to start"):
		return failureMessage{"浏览器未能启动", "当前轻量包需要本机已有 Chrome 或 Edge；请确认浏览器可以正常打开，再选 6 自检。"}
	case strings.Contains(text, "课堂"), strings.Contains(text, "没有匹配到实验"):
		return failureMessage{"没有找到可用的课堂或实验", "请选 2 查看课程，并确认账号已加入进行中的课堂。"}
	case errors.Is(err, os.ErrPermission):
		return failureMessage{"文件没有读写权限", "请将软件解压到你可以读写的目录，再重新启动。"}
	default:
		return failureMessage{"本次操作未完成", "可以重试或选 6 检查本机环境；仍失败时，请提供下方日志中的错误信息。"}
	}
}

func (a *app) showFailure(err error) {
	message := describeFailure(err)
	fmt.Fprintf(a.output(), "\n%s\n%s\n", message.title, message.hint)
	if a.logPath != "" {
		fmt.Fprintln(a.output(), "详细日志：", a.logPath)
	}
}

func (a *app) showConfigCreated() {
	fmt.Fprintln(a.output(), "\n已生成配置文件：", a.configPath)
	fmt.Fprintln(a.output(), "请用文本编辑器打开，填写平台地址、账号和密码，保存后重新选择功能。")
	fmt.Fprintln(a.output(), "配置与软件放在同一目录；换机器时可以一起复制。")
}

func (a *app) logFile() string { return filepath.Join(a.dataDir, "logs", "tempo.log") }
