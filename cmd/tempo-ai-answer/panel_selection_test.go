package main

import (
	"testing"

	"github.com/Maimai06/tempo-ai-answer/internal/core"
	"github.com/Maimai06/tempo-ai-answer/internal/dashboard"
	"github.com/Maimai06/tempo-ai-answer/internal/platform"
)

func panelCatalogFixture() (*panelCatalog, *runtimeState) {
	r := &runtimeState{plat: &platform.Client{ClassroomID: "configured"}}
	catalog := &panelCatalog{
		client: r,
		classroom: map[string]core.Classroom{
			"class-1": {ID: "class-1", Name: "Java 课堂", Class: "2026 级"},
		},
		courses: map[string]map[string]core.Course{
			"class-1": {"course-1": {CourseID: "course-1", Name: "循环实验", Chapter: "第二章", ChapterNo: 2, CourseNo: 1}},
		},
		steps: map[string]map[string][]core.StepRef{
			"class-1": {"course-1": {
				{ID: "step-1", Name: "未完成关卡", Sequence: 1},
				{ID: "step-2", Name: "已完成关卡", Sequence: 2, Finished: true},
			}},
		},
	}
	return catalog, r
}

func TestPanelPlanChecksCatalogMembershipAndMode(t *testing.T) {
	catalog, r := panelCatalogFixture()
	valid := dashboard.RunSelection{Mode: dashboard.ModeFormal, ClassroomID: "class-1", Courses: []dashboard.CourseSelection{{CourseID: "course-1", StepIDs: []string{"step-1"}}}}
	plan, classroom, err := catalog.plan(valid, r)
	if err != nil || classroom.Name != "Java 课堂" || r.plat.ClassroomID != "class-1" || len(plan) != 1 || len(plan[0].steps) != 1 || plan[0].steps[0].ID != "step-1" {
		t.Fatalf("valid plan: plan=%+v classroom=%+v err=%v", plan, classroom, err)
	}

	for _, invalid := range []dashboard.RunSelection{
		{Mode: dashboard.ModeFormal, ClassroomID: "other", Courses: []dashboard.CourseSelection{{CourseID: "course-1", StepIDs: []string{"step-1"}}}},
		{Mode: dashboard.ModeFormal, ClassroomID: "class-1", Courses: []dashboard.CourseSelection{{CourseID: "other", StepIDs: []string{"step-1"}}}},
		{Mode: dashboard.ModeFormal, ClassroomID: "class-1", Courses: []dashboard.CourseSelection{{CourseID: "course-1", StepIDs: []string{"other"}}}},
		{Mode: dashboard.ModeFormal, ClassroomID: "class-1", Courses: []dashboard.CourseSelection{{CourseID: "course-1", StepIDs: []string{"step-2"}}}},
	} {
		if _, _, err := catalog.plan(invalid, r); err == nil {
			t.Fatalf("accepted selection outside usable catalog: %+v", invalid)
		}
	}

	trial := dashboard.RunSelection{Mode: dashboard.ModeTrial, ClassroomID: "class-1", Courses: []dashboard.CourseSelection{{CourseID: "course-1", StepIDs: []string{"step-2"}}}}
	plan, _, err = catalog.plan(trial, r)
	if err != nil || len(plan) != 1 || len(plan[0].steps) != 1 || !plan[0].steps[0].Finished {
		t.Fatalf("single-step trial should permit a completed lesson: %+v, %v", plan, err)
	}
}
