package main

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMenuRunModesDoNotLeak(t *testing.T) {
	a, _ := selectionApp("")
	for _, tc := range []struct {
		choice        string
		demo, dry     bool
		limit         int
		includePassed bool
	}{
		{"7", false, true, 1, true},
		{"3", false, false, 0, false},
		{"4", true, false, 0, false},
		{"7", false, true, 1, true},
	} {
		a.scope, a.course, a.steps = "第1章", "old course", "2-5"
		name, ok := a.menuCommand(tc.choice)
		if !ok || name != "run" || a.demo != tc.demo || a.dry != tc.dry || a.limit != tc.limit || a.includePassed != tc.includePassed || a.scope != "" || a.course != "" || a.steps != "" {
			t.Fatalf("choice %s retained another mode: %+v", tc.choice, a)
		}
	}
}

func TestMenuTrialCreatesPortableConfigAndReturns(t *testing.T) {
	dir := t.TempDir()
	root := command()
	root.SetIn(strings.NewReader("7\n0\n"))
	output := &bytes.Buffer{}
	root.SetOut(output)
	root.SetArgs([]string{"--data-dir", dir})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "config.json")
	if _, err := os.Stat(path); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{path, "单关试运行", "最多处理 1 关", "不提交答案", "已生成配置文件", "已退出"} {
		if !strings.Contains(output.String(), want) {
			t.Errorf("missing %q in %s", want, output.String())
		}
	}
}

func TestRunConfirmationCanReturnBeforeStarting(t *testing.T) {
	a, output := selectionApp("mistyped\n0\n")
	if err := a.confirmRun(context.Background()); !errors.Is(err, errSelectionCanceled) {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "尚未开始") {
		t.Fatal(output.String())
	}
}

func TestHumanRangeInputAndErrors(t *testing.T) {
	a, output := selectionApp("\n4\n3-1\n1—2，3\n")
	selected, err := a.chooseNumbers(context.Background(), "range > ", 3, true)
	if err != nil || formatNumbers(selected) != "1-3" {
		t.Fatal(selected, err)
	}
	for _, want := range []string{"留空不会选择全部", "序号超出列表", "范围格式不正确"} {
		if !strings.Contains(output.String(), want) {
			t.Errorf("missing %q", want)
		}
	}
}

func TestRunSummaryDistinguishesGeneratedAndGraded(t *testing.T) {
	for _, tc := range []struct {
		mode, want, absent string
	}{
		{"7", "生成答案 1 关", "评测通过 1 关"},
		{"4", "本次使用模拟 AI", "评测通过 1 关"},
		{"3", "评测通过 1 关", "生成答案 1 关"},
	} {
		a, output := selectionApp("")
		a.menuCommand(tc.mode)
		a.showRunSummary(1, 1)
		if !strings.Contains(output.String(), tc.want) || strings.Contains(output.String(), tc.absent) {
			t.Fatal(output.String())
		}
	}
}

func TestFriendlyFailureShowsAdviceAndLogLocation(t *testing.T) {
	for _, tc := range []struct {
		err  error
		want string
	}{
		{errors.New("平台接口 /private-api 数据格式错误: strconv.ParseInt: invalid syntax"), "暂时无法读取平台数据"},
		{errors.New("打开平台失败: net::ERR_CONNECTION_RESET"), "校园网"},
		{context.DeadlineExceeded, "连接未成功"},
		{errors.New("DeepSeek 尚未登录"), "先选 1"},
	} {
		a, output := selectionApp("")
		a.logPath = filepath.Join(t.TempDir(), "tempo.log")
		a.showFailure(tc.err)
		if !strings.Contains(output.String(), tc.want) || !strings.Contains(output.String(), a.logPath) || strings.Contains(output.String(), "strconv.ParseInt") || strings.Contains(output.String(), "/private-api") {
			t.Fatal(output.String())
		}
	}
}
