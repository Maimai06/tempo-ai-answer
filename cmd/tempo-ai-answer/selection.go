package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/Maimai06/tempo-ai-answer/internal/core"
	"github.com/Maimai06/tempo-ai-answer/internal/scope"
)

var errSelectionCanceled = errors.New("已取消选择")

type classroomCatalog interface {
	ListClassrooms(context.Context) ([]core.Classroom, error)
}

func (a *app) output() io.Writer {
	if a.writer != nil {
		return a.writer
	}
	return os.Stdout
}

// readLine 统一菜单与确认输入，取消后不再读取下一轮选择。
func (a *app) readLine(ctx context.Context, prompt string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	fmt.Fprint(a.output(), prompt)
	type result struct {
		text string
		err  error
	}
	input := make(chan result, 1)
	go func() {
		text, err := a.reader.ReadString('\n')
		input <- result{text, err}
	}()
	select {
	case <-ctx.Done():
		return "", ctx.Err()
	case line := <-input:
		if line.err != nil {
			return "", fmt.Errorf("输入已结束，操作已取消: %w", line.err)
		}
		return strings.TrimSpace(line.text), nil
	}
}

// chooseNumbers 对课堂、实验和关卡复用范围校验；0 返回，留空不代表全部。
func (a *app) chooseNumbers(ctx context.Context, prompt string, count int, multiple bool) ([]int, error) {
	for {
		text, err := a.readLine(ctx, prompt)
		if err != nil {
			return nil, err
		}
		if text == "0" {
			return nil, errSelectionCanceled
		}
		numbers, err := scope.ParseRange(text)
		valid := err == nil && len(numbers) > 0 && (multiple || len(numbers) == 1)
		for _, n := range numbers {
			if count > 0 && n > count {
				valid = false
			}
		}
		if valid {
			return numbers, nil
		}
		fmt.Fprintln(a.output(), "请输入列表中的有效序号或范围；留空不会选择全部，输入 0 返回。")
	}
}

func (a *app) selectClassroom(ctx context.Context, catalog classroomCatalog, configured string) (core.Classroom, error) {
	classrooms, err := catalog.ListClassrooms(ctx)
	if err != nil {
		return core.Classroom{}, err
	}
	if len(classrooms) == 0 {
		return core.Classroom{}, errors.New("没有找到已加入的课堂")
	}
	wanted := strings.TrimSpace(a.classroomID)
	if wanted == "" && !a.inMenu && !strings.HasPrefix(configured, "YOUR_") {
		wanted = strings.TrimSpace(configured)
	}
	for _, classroom := range classrooms {
		if classroom.ID == "" {
			return core.Classroom{}, errors.New("课堂列表缺少课堂 ID，请检查平台数据")
		}
		if wanted != "" && string(classroom.ID) == wanted {
			return classroom, nil
		}
	}
	if wanted != "" {
		return core.Classroom{}, errors.New("指定课堂不在已加入的进行中课堂列表中，请使用 classrooms 查看")
	}
	if len(classrooms) == 1 {
		return classrooms[0], nil
	}
	if a.yes || a.headless {
		return core.Classroom{}, errors.New("有多个课堂，请使用 --classroom 指定课堂 ID，或在可见交互模式中选择")
	}
	fmt.Fprintln(a.output(), "\n请选择课堂：")
	for i, classroom := range classrooms {
		fmt.Fprintf(a.output(), "  %d. %s %s\n", i+1, classroom.Name, classroom.Class)
	}
	numbers, err := a.chooseNumbers(ctx, "课堂序号（0 返回） > ", len(classrooms), false)
	if err != nil {
		return core.Classroom{}, err
	}
	return classrooms[numbers[0]-1], nil
}

func (a *app) validateRunSelection() error {
	if (a.yes || a.headless) && strings.TrimSpace(a.scope) == "" && strings.TrimSpace(a.course) == "" {
		return errors.New("请使用 --scope 或 --course 明确选择实验范围，不能默认运行全部实验")
	}
	return nil
}

// selectPlan 先筛选实验，再选择明确的实验序号与关卡范围。
func (a *app) selectPlan(ctx context.Context, courses []core.Course, selection scope.Scope, steps []int) ([]core.Course, []int, error) {
	if err := a.validateRunSelection(); err != nil {
		return nil, nil, err
	}
	var plan []core.Course
	for _, course := range courses {
		if selection.Match(course) && strings.Contains(course.Label(), strings.TrimSpace(a.course)) {
			plan = append(plan, course)
		}
	}
	if len(plan) == 0 {
		return nil, nil, errors.New("没有匹配到实验，请检查范围")
	}
	if strings.TrimSpace(a.scope) != "" || strings.TrimSpace(a.course) != "" {
		return plan, steps, nil
	}
	fmt.Fprintln(a.output(), "\n请选择实验范围（序号对应下面的列表）：")
	for i, course := range plan {
		fmt.Fprintf(a.output(), "  %d. [%s] %s\n", i+1, course.Number(), course.Label())
	}
	numbers, err := a.chooseNumbers(ctx, "实验序号或范围（如 1、1-3,5；0 返回） > ", len(plan), true)
	if err != nil {
		return nil, nil, err
	}
	chosen := make([]core.Course, 0, len(numbers))
	for _, n := range numbers {
		chosen = append(chosen, plan[n-1])
	}
	if len(steps) == 0 {
		steps, err = a.chooseNumbers(ctx, "关卡编号或范围（如 1、1-2,5；适用于每个已选实验；0 返回） > ", 0, true)
		if err != nil {
			return nil, nil, err
		}
	}
	return chosen, steps, nil
}
