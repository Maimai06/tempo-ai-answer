// Package solver 编排答题与重试；只发布事件，不依赖 dashboard。
package solver

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Maimai06/tempo-ai-answer/internal/codemerge"
	"github.com/Maimai06/tempo-ai-answer/internal/core"
	"github.com/Maimai06/tempo-ai-answer/internal/prompts"
)

// Engine 依赖接口而不是网页实现，Demo 比 DryRun 有更高优先级。
type Engine struct {
	Platform     core.Platform
	AI           core.AIProvider
	Sink         core.Sink
	MaxRetry     int
	DryRun, Demo bool
	PerChat      int
	seq          int
}

func (e *Engine) log(text string) { e.Sink.Emit(core.Event{Kind: "log", Text: text}) }
func (e *Engine) result(passed *bool, detail string) {
	e.Sink.Emit(core.Event{Kind: "result", Text: detail, Passed: passed})
}
func (e *Engine) complete(ctx context.Context, answer string, n int) (string, error) {
	for i := 0; i < 15; i++ {
		if codemerge.Complete(answer, n) {
			return answer, nil
		}
		more, err := e.AI.ReadLatest(ctx)
		if err != nil {
			return "", err
		}
		if len(more) > len(answer) {
			answer = more
			continue
		}
		if err = core.Wait(ctx, 2*time.Second); err != nil {
			return "", err
		}
	}
	return "", errors.New("AI 回复尚未完整，未提交")
}

// Solve 每轮从同一不可变快照重新生成 prompt 和提交副本。
func (e *Engine) Solve(ctx context.Context, id string, seq int) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	step, err := e.Platform.GetStep(ctx, id)
	if err != nil {
		return false, err
	}
	e.Sink.Emit(core.Event{Kind: "step", Step: &step, Index: seq})
	snapshot := core.NewSnapshot(step)
	if step.Type != "code" && step.Type != "select" {
		e.log("跳过：不支持的题型 " + step.Type)
		return false, nil
	}
	if step.Type == "code" && !codemerge.Codable(step.Info.Codes) {
		e.log("跳过：没有可编辑代码或属于终端操作题")
		return false, nil
	}
	if step.Type == "select" && len(step.Info.Choices) == 0 {
		e.log("跳过：没有选择题内容")
		return false, nil
	}
	round, err := e.AI.BeginQuestion(ctx)
	if err != nil {
		return false, err
	}
	e.Sink.Emit(core.Event{Kind: "round", Text: fmt.Sprintf("%d/%d", round, max(1, e.PerChat))})
	if step.Type == "code" {
		e.seq++
	}
	feedback := ""
	var last error
	for attempt := 1; attempt <= e.MaxRetry+1; attempt++ {
		if err = ctx.Err(); err != nil {
			return false, err
		}
		original := snapshot.Step()
		prompt := prompts.Code(snapshot, feedback, e.seq)
		n := len(original.Info.Codes)
		if original.Type == "select" {
			prompt = prompts.Select(original.Info.Choices, feedback)
			n = 0
		}
		e.Sink.Emit(core.Event{Kind: "asking", Attempt: attempt, PromptLen: utf8.RuneCountInString(prompt)})
		answer, err := e.AI.Ask(ctx, prompt, n)
		if err != nil {
			last = err
			feedback = "上次 AI 调用失败，请重新按格式作答。"
			continue
		}
		if strings.TrimSpace(answer) == "" {
			feedback = "上次未返回答案，请按要求作答。"
			continue
		}
		if original.Type == "code" {
			answer, err = e.complete(ctx, answer, n)
			if err != nil {
				last = err
				feedback = err.Error()
				continue
			}
		}
		e.Sink.Emit(core.Event{Kind: "answer", Text: answer})
		var result core.Result
		if original.Type == "select" {
			letters := codemerge.ChoiceAnswers(answer, len(original.Info.Choices))
			missing := false
			lines := []string{}
			for i, lt := range letters {
				q := &original.Info.Choices[i]
				q.Answer = codemerge.Letters(lt, q.Type)
				q.Passed = false
				if q.Answer == "" {
					missing = true
				}
				for _, ch := range lt {
					if int(ch-'A') >= len(q.Content) {
						missing = true
					}
				}
				lines = append(lines, fmt.Sprintf("第%d题 → %s", i+1, lt))
			}
			e.Sink.Emit(core.Event{Kind: "generated", Text: strings.Join(lines, "\n")})
			if missing {
				feedback = "上次有题目未给出有效选项，请每题都作答。"
				continue
			}
			if e.DryRun || e.Demo {
				e.result(nil, "试运行：未提交评测")
				return true, nil
			}
			result, err = e.Platform.EvaluateSelect(ctx, id, original.Info.Choices)
		} else {
			code := codemerge.Extract(answer)
			if code == "" {
				feedback = "上次没有按格式输出代码，请输出完整代码。"
				continue
			}
			codes := original.Info.Codes
			if len(codes) == 1 {
				codes[0].Code = codemerge.Merge(codes[0].Code, code)
			} else {
				files := codemerge.SplitMulti(answer)
				hit := 0
				for i, c := range codes {
					if body, ok := codemerge.MatchPath(files, c.Path); ok {
						codes[i].Code = codemerge.Merge(c.Code, body)
						hit++
					}
				}
				if hit == 0 {
					index := 0
					for i, c := range codes {
						if codemerge.Begin.MatchString(c.Code) {
							index = i
							break
						}
					}
					codes[index].Code = codemerge.Merge(codes[index].Code, codemerge.StripMarks(code))
					e.Sink.Emit(core.Event{Kind: "warn", Text: "AI 未按多文件格式输出，只更新 " + codes[index].Path})
				}
			}
			for i, c := range codes {
				codes[i].Code = codemerge.FixClass(codemerge.StripMarks(c.Code), c.Path)
			}
			e.Sink.Emit(core.Event{Kind: "generated", Text: codes[0].Code})
			if e.DryRun || e.Demo {
				e.result(nil, "试运行：未提交评测")
				return true, nil
			}
			result, err = e.Platform.Evaluate(ctx, id, codes)
		}
		if err != nil {
			return false, err
		}
		passed, detail := codemerge.Summarize(result)
		if original.Type == "select" && !passed {
			wrong := []string{}
			for i, q := range original.Info.Choices {
				ok := false
				for _, trial := range result.Trials {
					if string(trial.ID) == string(q.ID) && trial.Passed {
						ok = true
					}
				}
				if !ok {
					wrong = append(wrong, fmt.Sprint(i+1))
				}
			}
			detail = "上次第 " + strings.Join(wrong, ", ") + " 题答错了，请重新仔细判断。"
		}
		e.result(&passed, detail)
		if passed {
			return true, nil
		}
		feedback = detail
		if feedback == "" {
			feedback = "输出与预期不一致，请检查输出格式。"
		}
		last = nil
	}
	if last != nil {
		return false, last
	}
	return false, nil
}
