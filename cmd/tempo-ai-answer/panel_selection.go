package main

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/Maimai06/tempo-ai-answer/internal/ai"
	"github.com/Maimai06/tempo-ai-answer/internal/core"
	"github.com/Maimai06/tempo-ai-answer/internal/dashboard"
)

type panelCatalog struct {
	mu        sync.Mutex
	client    *runtimeState
	classroom map[string]core.Classroom
	courses   map[string]map[string]core.Course
	steps     map[string]map[string][]core.StepRef
}

type panelDecision struct {
	selection dashboard.RunSelection
	plan      []coursePlan
	classroom core.Classroom
	canceled  bool
}

var errPanelContinue = errors.New("继续选择新的答题范围")

func newPanelCatalog(r *runtimeState) *panelCatalog {
	return &panelCatalog{
		client:    r,
		classroom: map[string]core.Classroom{},
		courses:   map[string]map[string]core.Course{},
		steps:     map[string]map[string][]core.StepRef{},
	}
}

func (c *panelCatalog) api(decisions chan<- panelDecision) dashboard.SelectionAPI {
	decide := func(ctx context.Context, value panelDecision) error {
		select {
		case decisions <- value:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return dashboard.SelectionAPI{
		Classrooms: c.listClassrooms,
		Courses:    c.listCourses,
		Steps:      c.listSteps,
		Start: func(ctx context.Context, selection dashboard.RunSelection) error {
			plan, classroom, err := c.plan(selection, c.client)
			if err != nil {
				return err
			}
			return decide(ctx, panelDecision{selection: selection, plan: plan, classroom: classroom})
		},
		Cancel: func(ctx context.Context) error {
			return decide(ctx, panelDecision{canceled: true})
		},
	}
}

func (c *panelCatalog) listClassrooms(ctx context.Context) ([]core.Classroom, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	classrooms, err := c.client.plat.ListClassrooms(ctx)
	if err != nil {
		return nil, err
	}
	c.classroom = make(map[string]core.Classroom, len(classrooms))
	c.courses = make(map[string]map[string]core.Course)
	c.steps = make(map[string]map[string][]core.StepRef)
	for _, classroom := range classrooms {
		id := strings.TrimSpace(string(classroom.ID))
		if id == "" || classroom.Name == "" {
			return nil, errors.New("课堂清单存在缺少名称或 ID 的项目")
		}
		if _, exists := c.classroom[id]; exists {
			return nil, errors.New("课堂清单存在重复 ID")
		}
		c.classroom[id] = classroom
	}
	return classrooms, nil
}

func (c *panelCatalog) listCourses(ctx context.Context, classroomID string) ([]dashboard.CourseOption, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	classroomID = strings.TrimSpace(classroomID)
	if _, ok := c.classroom[classroomID]; !ok {
		return nil, errors.New("请先从课堂清单中选择课堂")
	}
	previous := c.client.plat.ClassroomID
	c.client.plat.ClassroomID = classroomID
	courses, err := c.client.plat.ListCourses(ctx, "practice")
	c.client.plat.ClassroomID = previous
	if err != nil {
		return nil, err
	}
	byID := make(map[string]core.Course, len(courses))
	options := make([]dashboard.CourseOption, 0, len(courses))
	for _, course := range courses {
		id := course.Identifier()
		if id == "" || strings.TrimSpace(course.Label()) == "" {
			return nil, errors.New("实验清单存在缺少名称或 ID 的项目")
		}
		if _, exists := byID[id]; exists {
			return nil, errors.New("实验清单存在重复 ID")
		}
		byID[id] = course
		options = append(options, dashboard.CourseOption{ID: id, Number: course.Number(), Chapter: course.Chapter, Name: course.Label()})
	}
	c.courses[classroomID] = byID
	delete(c.steps, classroomID)
	return options, nil
}

func (c *panelCatalog) listSteps(ctx context.Context, classroomID, courseID string) ([]dashboard.StepOption, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	classroomID, courseID = strings.TrimSpace(classroomID), strings.TrimSpace(courseID)
	course, ok := c.courses[classroomID][courseID]
	if !ok {
		return nil, errors.New("请先从当前课堂的实验清单中选择实验")
	}
	previous := c.client.plat.ClassroomID
	c.client.plat.ClassroomID = classroomID
	steps, err := c.client.plat.ListSteps(ctx, course.Identifier())
	c.client.plat.ClassroomID = previous
	if err != nil {
		return nil, err
	}
	if len(steps) == 0 {
		return nil, errors.New("平台暂未返回关卡；请在 Tempo 打开该实验并启动实践环境后重试")
	}
	steps = append([]core.StepRef(nil), steps...)
	sort.SliceStable(steps, func(i, j int) bool { return steps[i].Sequence < steps[j].Sequence })
	seen := make(map[core.ID]bool, len(steps))
	options := make([]dashboard.StepOption, 0, len(steps))
	for _, step := range steps {
		if step.ID == "" || seen[step.ID] || step.Sequence < 1 {
			return nil, errors.New("关卡清单存在缺少名称、序号或重复 ID 的项目")
		}
		seen[step.ID] = true
		options = append(options, dashboard.StepOption{ID: string(step.ID), Sequence: step.Sequence, Name: step.Name, Finished: step.Finished})
	}
	if c.steps[classroomID] == nil {
		c.steps[classroomID] = map[string][]core.StepRef{}
	}
	c.steps[classroomID][courseID] = steps
	return options, nil
}

func (c *panelCatalog) plan(selection dashboard.RunSelection, r *runtimeState) ([]coursePlan, core.Classroom, error) {
	if err := selection.Validate(); err != nil {
		return nil, core.Classroom{}, err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	classroom, ok := c.classroom[selection.ClassroomID]
	if !ok {
		return nil, core.Classroom{}, errors.New("所选课堂不在本次登录读取的课堂清单中，请刷新后重试")
	}
	availableCourses := c.courses[selection.ClassroomID]
	plan := make([]coursePlan, 0, len(selection.Courses))
	for _, chosen := range selection.Courses {
		course, exists := availableCourses[chosen.CourseID]
		if !exists {
			return nil, core.Classroom{}, errors.New("所选实验不在该课堂的清单中，请重新选择")
		}
		availableSteps, exists := c.steps[selection.ClassroomID][chosen.CourseID]
		if !exists {
			return nil, core.Classroom{}, fmt.Errorf("实验「%s」的关卡清单已失效，请重新加载", course.Label())
		}
		stepMap := make(map[string]core.StepRef, len(availableSteps))
		for _, step := range availableSteps {
			stepMap[string(step.ID)] = step
		}
		entry := coursePlan{course: course, steps: make([]core.StepRef, 0, len(chosen.StepIDs))}
		for _, id := range chosen.StepIDs {
			step, exists := stepMap[id]
			if !exists {
				return nil, core.Classroom{}, fmt.Errorf("实验「%s」中找不到所选关卡，请重新加载", course.Label())
			}
			if step.Finished && selection.Mode != dashboard.ModeTrial {
				return nil, core.Classroom{}, fmt.Errorf("「%s」的第 %d 关已完成；当前模式会跳过已完成关卡", course.Label(), step.Sequence)
			}
			entry.steps = append(entry.steps, step)
		}
		plan = append(plan, entry)
	}
	if selection.Mode == dashboard.ModeTrial && (len(plan) != 1 || len(plan[0].steps) != 1) {
		return nil, core.Classroom{}, errors.New("单关试运行必须且只能选择一个实验中的一个关卡")
	}
	r.plat.ClassroomID = selection.ClassroomID
	return plan, classroom, nil
}

func (r *runtimeState) panelRun(ctx context.Context, a *app) error {
	if r.cfg.AI.Provider == "openai_compatible" {
		if _, err := r.newAPIProvider(); err != nil {
			return err
		}
		fmt.Fprintln(a.output(), "已选择兼容 AI API；本次无需打开或登录 DeepSeek。")
	} else {
		web := ai.New(r.session.NewPage(), time.Duration(r.cfg.AI.AnswerTimeout)*time.Second, r.cfg.Run.PerChat, r.sink)
		if err := web.Open(ctx); err != nil {
			return err
		}
		loggedIn, err := web.LoggedIn(ctx)
		if err != nil {
			return err
		}
		if !loggedIn {
			fmt.Fprintln(a.output(), "请在打开的 DeepSeek 标签页完成登录，检测到登录后会自动继续。")
			waitCtx, cancel := context.WithTimeout(ctx, 10*time.Minute)
			defer cancel()
			for !loggedIn {
				if err := core.Wait(waitCtx, time.Second); err != nil {
					return errors.New("等待 DeepSeek 登录超时或已取消")
				}
				loggedIn, err = web.LoggedIn(waitCtx)
				if err != nil {
					return err
				}
			}
		}
		r.aiWeb = web
	}

	catalog := newPanelCatalog(r)
	decisions := make(chan panelDecision, 1)
	closeRequested := make(chan struct{}, 1)
	selectionAPI := catalog.api(decisions)
	selectionAPI.Close = func(ctx context.Context) error {
		select {
		case closeRequested <- struct{}{}:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		default:
			return errors.New("退出请求已提交")
		}
	}
	continueRequested := make(chan struct{}, 1)
	selectionAPI.Continue = func(ctx context.Context) error {
		select {
		case continueRequested <- struct{}{}:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		default:
			return errors.New("继续选择请求已提交")
		}
	}
	if err := r.panel.EnableSelection(selectionAPI); err != nil {
		return err
	}
	if err := r.panel.Start(8877); err != nil {
		return err
	}
	defer func() {
		closeCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		if err := r.panel.Close(closeCtx); err != nil {
			r.log.Log.Warn("面板关闭失败")
		}
	}()
	r.panelExit = closeRequested
	r.panelContinue = continueRequested
	selectURL := r.panel.URL
	fmt.Fprintln(a.output(), "答题与监控面板：", selectURL)
	if err := r.session.NewPage().OpenLocal(ctx, selectURL); err != nil {
		return fmt.Errorf("无法打开范围选择面板 %s: %w", selectURL, err)
	}
	a.webPanel, a.webConfirmed = true, true
	if r.aiWeb != nil && r.cfg.Run.CleanupChat {
		defer r.cleanupPanelChats()
	}
	for {
		var decision panelDecision
		select {
		case <-ctx.Done():
			return ctx.Err()
		case decision = <-decisions:
		}
		if decision.canceled {
			fmt.Fprintln(a.output(), "已取消，未启动实践环境、AI 作答或评测。")
			return nil
		}
		r.classroom = strings.TrimSpace(decision.classroom.Name + " " + decision.classroom.Class)
		switch decision.selection.Mode {
		case dashboard.ModeTrial:
			a.dry, a.demo, a.includePassed, a.limit = true, false, true, 1
		case dashboard.ModeDemo:
			a.dry, a.demo, a.includePassed, a.limit = false, true, false, 0
		default:
			a.dry, a.demo, a.includePassed, a.limit = false, false, false, 0
		}
		err := r.runPlan(ctx, a, decision.plan)
		if errors.Is(err, errPanelContinue) {
			continue
		}
		return err
	}
}

func (r *runtimeState) cleanupPanelChats() {
	cleanupCtx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	n, err := r.aiWeb.Cleanup(cleanupCtx)
	if err != nil {
		r.sink.Emit(core.Event{Kind: "warn", Text: "对话清理失败：" + err.Error()})
		return
	}
	r.sink.Emit(core.Event{Kind: "log", Text: fmt.Sprintf("已清理 %d 个新对话", n)})
}
