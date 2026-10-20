package main

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/Maimai06/tempo-ai-answer/internal/core"
	"github.com/Maimai06/tempo-ai-answer/internal/scope"
)

type fakeStepCatalog struct {
	steps map[string][]core.StepRef
	err   error
	calls []string
}

func (c *fakeStepCatalog) ListSteps(_ context.Context, id string) ([]core.StepRef, error) {
	c.calls = append(c.calls, id)
	return c.steps[id], c.err
}

func TestIndependentStepRangesUseDisplayedRowsAndIDs(t *testing.T) {
	catalog := &fakeStepCatalog{steps: map[string][]core.StepRef{
		"a": {
			{ID: "a-30", Name: "基础第三关", Sequence: 30},
			{ID: "a-10", Name: "基础第一关", Sequence: 10, Finished: true},
			{ID: "a-20", Name: "基础第二关", Sequence: 20},
		},
		"b": {{ID: "b-1", Name: "循环第一关", Sequence: 1}, {ID: "b-2", Name: "循环第二关", Sequence: 2}},
	}}
	a, output := selectionApp("\n4\n1-3\n2\n")
	plan, err := a.selectStepPlan(context.Background(), catalog, courseFixtures()[:2], nil, true)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(catalog.calls, []string{"a", "b"}) || len(plan) != 2 || len(plan[0].steps) != 2 || plan[0].steps[0].ID != "a-20" || plan[0].steps[1].ID != "a-30" || plan[1].steps[0].ID != "b-2" {
		t.Fatal(plan, catalog.calls)
	}
	if !plan[0].keep(core.StepRef{ID: "a-20", Sequence: 20}, true) || plan[0].keep(core.StepRef{ID: "changed", Sequence: 20}, true) || plan[1].keep(core.StepRef{ID: "b-1", Sequence: 1}, true) {
		t.Fatal("row choices did not remain attached to selected step IDs")
	}
	for _, want := range []string{"1. 基础第一关", "已完成，将跳过", "2. 基础第二关", "循环第二关", "序号超出列表", "已排除"} {
		if !strings.Contains(output.String(), want) {
			t.Errorf("missing %q in %s", want, output.String())
		}
	}
	output.Reset()
	a.showRunPlan("测试课堂", plan)
	if !strings.Contains(output.String(), "基础第二关") || !strings.Contains(output.String(), "循环第二关") || strings.Contains(output.String(), "循环第一关") || strings.Contains(output.String(), "适用于每个") {
		t.Fatal(output.String())
	}
}

func TestSingleTrialSelectsOneExperimentAndOneCompletedStep(t *testing.T) {
	a, output := selectionApp("1-2\n2\n1-2\n1\n")
	a.menuCommand("7")
	courses, _, err := a.selectPlan(context.Background(), courseFixtures(), scope.Scope{}, nil)
	if err != nil || len(courses) != 1 || courses[0].Identifier() != "b" {
		t.Fatal(courses, err)
	}
	catalog := &fakeStepCatalog{steps: map[string][]core.StepRef{"b": {
		{ID: "done", Name: "已经完成的一关", Sequence: 1, Finished: true},
		{ID: "another", Name: "另一关", Sequence: 2},
	}}}
	plan, err := a.selectStepPlan(context.Background(), catalog, courses, nil, false)
	if err != nil || len(plan) != 1 || len(plan[0].steps) != 1 || plan[0].steps[0].ID != "done" {
		t.Fatal(plan, err)
	}
	if strings.Count(output.String(), "一次只能选择一项") != 2 {
		t.Fatal(output.String())
	}
}

func TestStepSelectionCannotDefaultToAllOrInventSteps(t *testing.T) {
	for _, tc := range []struct {
		name, input string
		steps       []core.StepRef
		err         error
	}{
		{"blank then cancel", "\n0\n", []core.StepRef{{ID: "a", Sequence: 1}}, nil},
		{"completed then cancel", "1\n0\n", []core.StepRef{{ID: "a", Sequence: 1, Finished: true}}, nil},
		{"unavailable list", "", nil, errors.New("environment unavailable")},
		{"empty list", "", nil, nil},
		{"missing ID", "", []core.StepRef{{Name: "no ID"}}, nil},
		{"duplicate ID", "", []core.StepRef{{ID: "same"}, {ID: "same"}}, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a, _ := selectionApp(tc.input)
			catalog := &fakeStepCatalog{steps: map[string][]core.StepRef{"a": tc.steps}, err: tc.err}
			plan, err := a.selectStepPlan(context.Background(), catalog, courseFixtures()[:1], nil, true)
			if err == nil || len(plan) != 0 || len(catalog.calls) != 1 {
				t.Fatal(plan, err, catalog.calls)
			}
		})
	}
}

func TestExplicitCLISequencesPreserveTheirMeaning(t *testing.T) {
	a, output := selectionApp("")
	a.scope = "第1章"
	catalog := &fakeStepCatalog{err: errors.New("should not be read before confirmation")}
	plan, err := a.selectStepPlan(context.Background(), catalog, courseFixtures()[:2], []int{20}, true)
	if err != nil || len(catalog.calls) != 0 || output.Len() != 0 {
		t.Fatal(err, catalog.calls, output.String())
	}
	for _, entry := range plan {
		if !entry.keep(core.StepRef{Sequence: 20}, true) || entry.keep(core.StepRef{Sequence: 1}, true) || entry.keep(core.StepRef{Sequence: 20, Finished: true}, true) {
			t.Fatal("CLI --steps lost sequence or completion filtering")
		}
	}
	plan, err = a.selectStepPlan(context.Background(), catalog, courseFixtures()[:1], nil, true)
	if err != nil || !plan[0].keep(core.StepRef{Sequence: 1}, true) || !plan[0].keep(core.StepRef{Sequence: 20}, true) {
		t.Fatal("explicit CLI experiment no longer includes unskipped steps", err)
	}
}
