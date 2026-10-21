package dashboard

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/Maimai06/tempo-ai-answer/internal/core"
)

type SelectionMode string

const (
	ModeFormal SelectionMode = "formal"
	ModeTrial  SelectionMode = "trial"
	ModeDemo   SelectionMode = "demo"
)

// CourseOption 暴露给选择面板的实验展示字段，不包含原始平台响应。
type CourseOption struct {
	ID      string `json:"id"`
	Number  string `json:"number"`
	Chapter string `json:"chapter"`
	Name    string `json:"name"`
}

// StepOption 只保留范围选择需要展示与回传的字段。
type StepOption struct {
	ID       string `json:"id"`
	Sequence int    `json:"sequence"`
	Name     string `json:"name"`
	Finished bool   `json:"finished"`
}

type CourseSelection struct {
	CourseID string   `json:"course_id"`
	StepIDs  []string `json:"step_ids"`
}

type RunSelection struct {
	Mode        SelectionMode     `json:"mode"`
	ClassroomID string            `json:"classroom_id"`
	Courses     []CourseSelection `json:"courses"`
}

// Validate 检查面板提交的最小计划结构，平台 ID 归属由运行器再次校验。
func (s RunSelection) Validate() error {
	if strings.TrimSpace(s.ClassroomID) == "" {
		return errors.New("请选择课堂")
	}
	if s.Mode != ModeFormal && s.Mode != ModeTrial && s.Mode != ModeDemo {
		return errors.New("运行模式无效")
	}
	if len(s.Courses) == 0 {
		return errors.New("至少选择一个实验和关卡")
	}
	courses := map[string]bool{}
	stepCount := 0
	for _, course := range s.Courses {
		id := strings.TrimSpace(course.CourseID)
		if id == "" || courses[id] {
			return errors.New("实验选择为空或重复")
		}
		courses[id] = true
		if len(course.StepIDs) == 0 {
			return fmt.Errorf("实验 %s 尚未选择关卡", id)
		}
		steps := map[string]bool{}
		for _, stepID := range course.StepIDs {
			stepID = strings.TrimSpace(stepID)
			if stepID == "" || steps[stepID] {
				return fmt.Errorf("实验 %s 的关卡选择为空或重复", id)
			}
			steps[stepID] = true
			stepCount++
		}
	}
	if s.Mode == ModeTrial && stepCount != 1 {
		return errors.New("单关试运行必须且只能选择一个关卡")
	}
	return nil
}

// SelectionAPI 提供登录后按需读取清单，并接收用户确认后的范围。
type SelectionAPI struct {
	Classrooms func(context.Context) ([]core.Classroom, error)
	Courses    func(context.Context, string) ([]CourseOption, error)
	Steps      func(context.Context, string, string) ([]StepOption, error)
	Start      func(context.Context, RunSelection) error
	Cancel     func(context.Context) error
	Continue   func(context.Context) error // 本段完成后回到范围选择状态。
	Close      func(context.Context) error
}
