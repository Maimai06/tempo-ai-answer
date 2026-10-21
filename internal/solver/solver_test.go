package solver

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/Maimai06/tempo-ai-answer/internal/core"
)

type fakePlatform struct {
	step      core.Step
	writes    int
	passAt    int
	submitted [][]core.Code
}

func (f *fakePlatform) GetStep(context.Context, string) (core.Step, error) { return f.step, nil }
func (f *fakePlatform) Evaluate(_ context.Context, _ string, c []core.Code) (core.Result, error) {
	f.writes++
	f.submitted = append(f.submitted, append([]core.Code(nil), c...))
	c[0].Code = "platform mutated caller slice"
	return core.Result{Passed: f.writes >= f.passAt, Message: "retry"}, nil
}
func (f *fakePlatform) EvaluateSelect(_ context.Context, _ string, c []core.Choice) (core.Result, error) {
	f.writes++
	return core.Result{Passed: f.writes >= f.passAt, Message: "retry"}, nil
}

type fakeAI struct {
	answers []string
	prompts []string
	err     error
	latest  string
}

type finalFakeAI struct{ *fakeAI }

func (finalFakeAI) ResponsesAreFinal() bool { return true }

func (f *fakeAI) BeginQuestion(context.Context) (int, error) { return 1, nil }
func (f *fakeAI) Ask(_ context.Context, p string, _ int) (string, error) {
	f.prompts = append(f.prompts, p)
	if f.err != nil {
		return "", f.err
	}
	i := len(f.prompts) - 1
	if i >= len(f.answers) {
		return "", nil
	}
	return f.answers[i], nil
}
func (f *fakeAI) ReadLatest(context.Context) (string, error) { return f.latest, f.err }
func codeStep() core.Step {
	return core.Step{ID: "one", Type: "code", Info: core.QuestionInfo{Codes: []core.Code{{Path: "A.java", Code: "class A {\n/********* Begin *********/\nORIGINAL\n/********* End *********/\n}"}}}}
}
func choiceStep() core.Step {
	return core.Step{ID: "one", Type: "select", Info: core.QuestionInfo{Choices: []core.Choice{{ID: []byte(`1`), Type: "single_choice", Content: []string{"x", "y"}}}}}
}
func TestRetryOriginalSnapshot(t *testing.T) {
	p := &fakePlatform{step: codeStep(), passAt: 2}
	a := &fakeAI{answers: []string{"```java\nfirst();\n```", "```java\nsecond();\n```"}}
	e := Engine{Platform: p, AI: a, MaxRetry: 2}
	ok, err := e.Solve(context.Background(), "one", 1)
	if err != nil || !ok || p.writes != 2 {
		t.Fatal(ok, err, p.writes)
	}
	for _, prompt := range a.prompts {
		if !strings.Contains(prompt, "ORIGINAL") || strings.Contains(prompt, "first();") || strings.Contains(prompt, "platform mutated") {
			t.Fatal(prompt)
		}
	}
	if !strings.Contains(p.submitted[1][0].Code, "second();") || !strings.Contains(p.step.Info.Codes[0].Code, "ORIGINAL") {
		t.Fatal(p)
	}
}

func TestFinalAIResponseDoesNotWaitForStreaming(t *testing.T) {
	provider := finalFakeAI{fakeAI: &fakeAI{err: errors.New("ReadLatest should not be called")}}
	engine := Engine{AI: provider}
	_, err := engine.complete(context.Background(), "class A {", 1)
	if err == nil || err.Error() != "AI 回复尚未完整，未提交" {
		t.Fatalf("incomplete final response error = %v", err)
	}
}

func TestNoWritesInEveryDryMode(t *testing.T) {
	for _, step := range []core.Step{codeStep(), choiceStep()} {
		for _, mode := range []struct{ dry, demo bool }{{true, false}, {false, true}, {true, true}} {
			p := &fakePlatform{step: step}
			answer := "```java\nx();\n```"
			if step.Type == "select" {
				answer = "1. A"
			}
			e := Engine{Platform: p, AI: &fakeAI{answers: []string{answer}}, DryRun: mode.dry, Demo: mode.demo}
			ok, err := e.Solve(context.Background(), "one", 1)
			if err != nil || !ok || p.writes != 0 {
				t.Fatal(step.Type, mode, ok, err, p.writes)
			}
		}
	}
}
func TestSkipRetryAndCancel(t *testing.T) {
	for _, step := range []core.Step{{Type: "unknown"}, {Type: "select"}, {Type: "code", Info: core.QuestionInfo{Codes: []core.Code{{Path: "README.md"}}}}} {
		a := &fakeAI{}
		e := Engine{Platform: &fakePlatform{step: step}, AI: a}
		ok, err := e.Solve(context.Background(), "id", 0)
		if ok || err != nil || len(a.prompts) != 0 {
			t.Fatal(step, ok, err)
		}
	}
	p := &fakePlatform{step: choiceStep(), passAt: 1}
	a := &fakeAI{answers: []string{"", "1. Z", "1. B"}}
	e := Engine{Platform: p, AI: a, MaxRetry: 2}
	ok, err := e.Solve(context.Background(), "id", 1)
	if err != nil || !ok || p.writes != 1 {
		t.Fatal(ok, err, p.writes)
	}
	e.AI = &fakeAI{err: errors.New("unavailable")}
	_, err = e.Solve(context.Background(), "id", 1)
	if err == nil {
		t.Fatal("AI failure ignored")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err = e.Solve(ctx, "id", 1); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}
func TestMultiFileAndEmptyRetry(t *testing.T) {
	for _, answer := range []string{"===FILE: A.java===\npublic class Wrong {}\n===FILE: B.java===\npublic class Wrong {}", "```java\ncall();\n```"} {
		step := codeStep()
		step.Info.Codes = append(step.Info.Codes, core.Code{Path: "B.java", Code: "class B {}"})
		p := &fakePlatform{step: step, passAt: 1}
		a := &fakeAI{answers: []string{answer}, latest: "===FILE: A.java===\npublic class Wrong {}\n===FILE: B.java===\npublic class Wrong {}"}
		engine := Engine{Platform: p, AI: a}
		ok, e := engine.Solve(context.Background(), "one", 1)
		if e != nil || !ok || len(p.submitted) != 1 || len(p.submitted[0]) != 2 {
			t.Fatal(ok, e, p.submitted)
		}
		if strings.Contains(p.submitted[0][0].Code, "class Wrong") {
			t.Fatal(p.submitted)
		}
	}
	p := &fakePlatform{step: codeStep(), passAt: 1}
	a := &fakeAI{answers: []string{"", "```java\ncall();\n```"}}
	engine := Engine{Platform: p, AI: a, MaxRetry: 1}
	if ok, e := engine.Solve(context.Background(), "one", 1); e != nil || !ok || p.writes != 1 {
		t.Fatal(ok, e, p.writes)
	}
}
