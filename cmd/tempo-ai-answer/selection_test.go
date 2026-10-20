package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"

	"github.com/Maimai06/tempo-ai-answer/internal/core"
	"github.com/Maimai06/tempo-ai-answer/internal/platform"
	"github.com/Maimai06/tempo-ai-answer/internal/scope"
)

type fakeCatalog struct {
	classrooms []core.Classroom
	err        error
}

func (c fakeCatalog) ListClassrooms(context.Context) ([]core.Classroom, error) {
	return c.classrooms, c.err
}

func selectionApp(input string) (*app, *bytes.Buffer) {
	output := &bytes.Buffer{}
	return &app{reader: bufio.NewReader(strings.NewReader(input)), writer: output}, output
}

func TestClassroomSelection(t *testing.T) {
	catalog := fakeCatalog{classrooms: []core.Classroom{{ID: "java", Name: "Java"}, {ID: "python", Name: "Python"}}}
	for _, tc := range []struct {
		name, input, flag, configured, want string
		menu, yes, headless                 bool
		wantErr                             bool
	}{
		{name: "pick second classroom", input: "2\n", want: "python"},
		{name: "retry blank and out of range", input: "\n9\n1-2\n2\n", want: "python"},
		{name: "flag overrides config", flag: "python", configured: "java", want: "python", yes: true},
		{name: "config for CLI", configured: "python", want: "python", yes: true},
		{name: "menu offers another classroom", input: "2\n", configured: "java", menu: true, want: "python"},
		{name: "unknown classroom", flag: "unknown", wantErr: true},
		{name: "noninteractive requires classroom", yes: true, wantErr: true},
		{name: "headless requires classroom", headless: true, wantErr: true},
		{name: "EOF cancels", wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a, _ := selectionApp(tc.input)
			a.classroomID, a.inMenu, a.yes, a.headless = tc.flag, tc.menu, tc.yes, tc.headless
			got, err := a.selectClassroom(context.Background(), catalog, tc.configured)
			if (err != nil) != tc.wantErr || string(got.ID) != tc.want {
				t.Fatalf("got %q, %v; want %q, error=%v", got.ID, err, tc.want, tc.wantErr)
			}
		})
	}
	a, _ := selectionApp("0\n")
	if _, err := a.selectClassroom(context.Background(), catalog, ""); !errors.Is(err, errSelectionCanceled) {
		t.Fatal(err)
	}
	for _, catalog := range []fakeCatalog{{}, {err: errors.New("fixture")}, {classrooms: []core.Classroom{{Name: "missing ID"}}}} {
		if _, err := a.selectClassroom(context.Background(), catalog, ""); err == nil {
			t.Fatal("invalid classroom catalog accepted")
		}
	}
	only := fakeCatalog{classrooms: []core.Classroom{{ID: "only"}}}
	if got, err := a.selectClassroom(context.Background(), only, ""); err != nil || got.ID != "only" {
		t.Fatal(got, err)
	}
}

func courseFixtures() []core.Course {
	return []core.Course{
		{CourseID: "a", Name: "Java 基础", ChapterNo: 1, CourseNo: 1},
		{CourseID: "b", Name: "Java 循环", ChapterNo: 1, CourseNo: 2},
		{CourseID: "c", Name: "Java 数组", ChapterNo: 1, CourseNo: 3},
		{CourseID: "d", Name: "Python", ChapterNo: 2, CourseNo: 1},
		{CourseID: "e", Name: "Java 类", ChapterNo: 2, CourseNo: 2},
	}
}

func TestExperimentAndStepRanges(t *testing.T) {
	a, output := selectionApp("\n9\n3-1\n1-3,5,2\n\n1-2,5\n")
	plan, steps, err := a.selectPlan(context.Background(), courseFixtures(), scope.Scope{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, course := range plan {
		ids = append(ids, course.Identifier())
	}
	if !reflect.DeepEqual(ids, []string{"a", "b", "c", "e"}) || !reflect.DeepEqual(steps, []int{1, 2, 5}) {
		t.Fatalf("unexpected selected IDs %v, steps %v", ids, steps)
	}
	if !strings.Contains(output.String(), "[2-1] Python") || !strings.Contains(output.String(), "留空不会选择全部") {
		t.Fatal(output.String())
	}
}

func TestExplicitExperimentFilters(t *testing.T) {
	for _, tc := range []struct {
		name, scope, keyword string
		want                 []string
	}{
		{"one experiment", "1-2", "", []string{"b"}},
		{"chapter range", "第1章 1-2", "", []string{"a", "b"}},
		{"keyword", "", "循环", []string{"b"}},
		{"combined filters", "第2章", "Java", []string{"e"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a, output := selectionApp("")
			a.scope, a.course, a.yes = tc.scope, tc.keyword, true
			selection, err := scope.Parse(a.scope)
			if err != nil {
				t.Fatal(err)
			}
			plan, steps, err := a.selectPlan(context.Background(), courseFixtures(), selection, []int{2, 3})
			if err != nil {
				t.Fatal(err)
			}
			var ids []string
			for _, course := range plan {
				ids = append(ids, course.Identifier())
			}
			if !reflect.DeepEqual(ids, tc.want) || !reflect.DeepEqual(steps, []int{2, 3}) || output.Len() != 0 {
				t.Fatal(ids, steps, output.String())
			}
		})
	}
}

func TestSelectionCancellationAndEmptyScope(t *testing.T) {
	for _, input := range []string{"0\n", "1\n0\n", "", "\n"} {
		a, _ := selectionApp(input)
		if _, _, err := a.selectPlan(context.Background(), courseFixtures(), scope.Scope{}, nil); err == nil {
			t.Fatalf("cancelled or empty input %q selected a plan", input)
		}
	}
	for _, mode := range []app{{yes: true}, {headless: true}, {yes: true, steps: "1"}} {
		if _, _, err := mode.selectPlan(context.Background(), courseFixtures(), scope.Scope{}, nil); err == nil {
			t.Fatal("unattended run defaulted to all experiments")
		}
	}
	a, _ := selectionApp("")
	a.course = "not found"
	if _, _, err := a.selectPlan(context.Background(), courseFixtures(), scope.Scope{}, nil); err == nil {
		t.Fatal("missing course accepted")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	a, _ = selectionApp("1\n")
	if _, _, err := a.selectPlan(ctx, courseFixtures(), scope.Scope{}, nil); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}

type cancelPage struct{ requests int }

func (*cancelPage) Navigate(context.Context, string) error { return nil }
func (p *cancelPage) Eval(_ context.Context, js string, out any) error {
	p.requests++
	if !strings.Contains(js, "/classroomCourseListDetail") {
		return errors.New("selection touched an unexpected endpoint")
	}
	b, err := json.Marshal(map[string]any{"status": 200, "json": map[string]any{"success": true, "data": courseFixtures()}})
	if err != nil {
		return err
	}
	return json.Unmarshal(b, out)
}

func TestCancelBeforeOpeningAIOrEnvironment(t *testing.T) {
	page := &cancelPage{}
	r := runtimeState{plat: &platform.Client{Page: page, BaseURL: "https://example.invalid/", ClassroomID: "fixture"}}
	a, _ := selectionApp("1\n0\n")
	if err := r.run(context.Background(), a, scope.Scope{}, nil); !errors.Is(err, errSelectionCanceled) || page.requests != 1 {
		t.Fatal(err, page.requests)
	}
}

func TestMenuExitAndReadCancellation(t *testing.T) {
	a, _ := selectionApp("0\n")
	root := command()
	root.SetContext(context.Background())
	if err := a.menu(root); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	input, writer := io.Pipe()
	defer input.Close()
	defer writer.Close()
	a.reader = bufio.NewReader(input)
	done := make(chan error, 1)
	go func() { _, err := a.readLine(ctx, ""); done <- err }()
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}
