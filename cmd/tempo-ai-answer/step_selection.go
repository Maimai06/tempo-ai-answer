package main

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/Maimai06/tempo-ai-answer/internal/core"
)

type stepCatalog interface {
	ListSteps(context.Context, string) ([]core.StepRef, error)
}

// coursePlan 为每个实验保存独立的关卡选择；交互选择按 ID 固定目标。
type coursePlan struct {
	course  core.Course
	steps   []core.StepRef
	numbers []int
}

func (p coursePlan) keep(step core.StepRef, skipPassed bool) bool {
	if skipPassed && step.Finished {
		return false
	}
	if p.steps != nil {
		for _, selected := range p.steps {
			if selected.ID == step.ID {
				return true
			}
		}
		return false
	}
	if len(p.numbers) == 0 {
		return true
	}
	for _, n := range p.numbers {
		if n == step.Sequence {
			return true
		}
	}
	return false
}

type stepListError struct {
	course string
	err    error
}

func (e *stepListError) Error() string {
	return fmt.Sprintf("读取实验「%s」的关卡失败: %v", e.course, e.err)
}
func (e *stepListError) Unwrap() error { return e.err }

// selectStepPlan 只读关卡清单，取消时不启动实践环境、AI 或评测。
func (a *app) selectStepPlan(ctx context.Context, catalog stepCatalog, courses []core.Course, requested []int, skipPassed bool) ([]coursePlan, error) {
	interactive := len(requested) == 0 && strings.TrimSpace(a.scope) == "" && strings.TrimSpace(a.course) == ""
	plan := make([]coursePlan, 0, len(courses))
	for i, course := range courses {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		entry := coursePlan{course: course, numbers: requested}
		if interactive {
			fmt.Fprintf(a.output(), "\n第 3 步 · 选择关卡（实验 %d/%d）\n[%s] %s\n正在读取关卡列表…\n", i+1, len(courses), course.Number(), course.Label())
			steps, err := catalog.ListSteps(ctx, course.Identifier())
			if err != nil {
				return nil, &stepListError{course.Label(), err}
			}
			if len(steps) == 0 {
				return nil, &stepListError{course.Label(), errors.New("平台返回的关卡列表为空")}
			}
			steps = append([]core.StepRef(nil), steps...)
			sort.SliceStable(steps, func(i, j int) bool { return steps[i].Sequence < steps[j].Sequence })
			seen := map[core.ID]bool{}
			for j, step := range steps {
				if step.ID == "" || seen[step.ID] {
					return nil, &stepListError{course.Label(), errors.New("关卡 ID 缺失或重复")}
				}
				seen[step.ID] = true
				fmt.Fprintf(a.output(), "  %d. %s  [%s]\n", j+1, stepName(step), stepStatus(step, skipPassed))
			}
			if a.includePassed {
				fmt.Fprintln(a.output(), "请选择一个关卡；已完成的也可试运行，不会提交答案。")
			} else {
				fmt.Fprintln(a.output(), "选择这个实验的关卡序号，支持 1-3,5；每个实验分别选择。")
			}
			for {
				numbers, err := a.chooseNumbers(ctx, "选择关卡（输入左侧序号；0 返回） > ", len(steps), !a.includePassed)
				if err != nil {
					return nil, err
				}
				entry.steps = make([]core.StepRef, 0, len(numbers))
				for _, n := range numbers {
					if !skipPassed || !steps[n-1].Finished {
						entry.steps = append(entry.steps, steps[n-1])
					}
				}
				if len(entry.steps) > 0 {
					if len(entry.steps) < len(numbers) {
						fmt.Fprintln(a.output(), "已排除范围内已完成的关卡。")
					}
					break
				}
				fmt.Fprintln(a.output(), "所选关卡均已完成，请选择未完成关卡；测试已完成关卡可返回主菜单选 7。")
			}
		}
		plan = append(plan, entry)
	}
	return plan, nil
}

func stepName(step core.StepRef) string {
	if strings.TrimSpace(step.Name) != "" {
		return step.Name
	}
	return fmt.Sprintf("第 %d 关（未命名）", step.Sequence)
}

func stepStatus(step core.StepRef, skipPassed bool) string {
	if !step.Finished {
		return "未完成"
	}
	if skipPassed {
		return "已完成，将跳过"
	}
	return "已完成"
}
