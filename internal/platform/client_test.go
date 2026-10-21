package platform

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/Maimai06/tempo-ai-answer/internal/core"
)

type fakePage struct {
	responses []response
	calls     []string
	err       error
}

func (p *fakePage) Navigate(context.Context, string) error { return nil }
func (p *fakePage) Eval(_ context.Context, js string, out any) error {
	p.calls = append(p.calls, js)
	if p.err != nil {
		return p.err
	}
	if len(p.responses) == 0 {
		return errors.New("unexpected request")
	}
	b, e := json.Marshal(p.responses[0])
	p.responses = p.responses[1:]
	if e != nil {
		return e
	}
	return json.Unmarshal(b, out)
}
func resp(data string) response {
	return response{Status: 200, JSON: &envelope{Success: true, Data: json.RawMessage(data)}}
}
func TestGatewayProtocol(t *testing.T) {
	p := &fakePage{responses: []response{resp(`{"rows":[{"stepId":123,"name":"fixture","sequence":1}]}`)}}
	c := Client{Page: p, BaseURL: "https://example.invalid/tempo-talents/", ClassroomID: "班 1"}
	s, e := c.ListSteps(context.Background(), "course &1")
	if e != nil || len(s) != 1 || s[0].ID != "123" {
		t.Fatal(s, e)
	}
	js := p.calls[0]
	for _, want := range []string{"credentials:'include'", "X-Requested-With", "/gateway/tempo-talents/api/tt-course-challenge/challenge/list", "courseId=course+%261", "classroomId=%E7%8F%AD+1"} {
		if !strings.Contains(js, want) {
			t.Fatal(want, js)
		}
	}
}
func TestFlattenAndPayloads(t *testing.T) {
	p := &fakePage{responses: []response{resp(`[{"sort":2,"coursesName":"第二章","lastList":[{"coursesId":"c1","coursesName":"Java","sort":3}]}]`), resp(`null`), resp(`null`)}}
	c := Client{Page: p, BaseURL: "https://example.invalid/tempo-talents/", ClassroomID: "class"}
	cs, e := c.ListCourses(context.Background(), "practice")
	if e != nil || len(cs) != 1 || cs[0].Identifier() != "c1" || cs[0].Number() != "2-3" {
		t.Fatal(cs, e)
	}
	if e = c.save(context.Background(), "step", "codeJson", []core.Code{{Path: "A.java", Code: "中文\n", Name: "A"}}); e != nil {
		t.Fatal(e)
	}
	if !strings.Contains(p.calls[1], "codeJson=") || !strings.Contains(p.calls[1], "newElapsedTime=5") {
		t.Fatal(p.calls[1])
	}
	if e = c.save(context.Background(), "step", "selectOrJudgeJson", []core.Choice{{ID: []byte(`123`), Answer: "0"}}); e != nil {
		t.Fatal(e)
	}
	if !strings.Contains(p.calls[2], "selectOrJudgeJson=") {
		t.Fatal(p.calls[2])
	}
}
func TestFailuresAndClassroom(t *testing.T) {
	for _, r := range []response{{Status: 200, HTML: true, URL: "https://example.invalid/gateway/tempo-talents/api/fixture"}, {Status: 403}, {Status: 200, JSON: &envelope{Success: false}}, {Status: 200, JSON: &envelope{Success: true, Data: json.RawMessage(`[]`)}}} {
		p := &fakePage{responses: []response{r}}
		c := Client{Page: p, BaseURL: "https://example.invalid/"}
		var u core.User
		if e := c.API(context.Background(), "/fixture", "POST", nil, &u); e == nil {
			t.Fatal(r)
		}
	}
	p := &fakePage{responses: []response{resp(`{"classroomListInfo":{"rows":[{"id":"live-id"}]}}`)}}
	c := Client{Page: p, BaseURL: "https://example.invalid/"}
	if e := c.EnsureClassroom(context.Background()); e != nil || c.ClassroomID != "live-id" {
		t.Fatal(c.ClassroomID, e)
	}
	if e := c.EnsureClassroom(context.Background()); e != nil {
		t.Fatal(e)
	}
}

func TestAPIErrorKeepsPlatformMessageForPrivateLogs(t *testing.T) {
	p := &fakePage{responses: []response{{Status: 200, JSON: &envelope{Success: false, Message: "实践环境尚未启动"}}}}
	c := Client{Page: p, BaseURL: "https://example.invalid/"}
	err := c.API(context.Background(), "/fixture", "GET", nil, nil)
	var detail interface{ LogDetail() string }
	if !errors.As(err, &detail) {
		t.Fatalf("expected detailed API error, got %v", err)
	}
	if strings.Contains(err.Error(), "实践环境尚未启动") || !strings.Contains(detail.LogDetail(), "实践环境尚未启动") {
		t.Fatalf("platform message should be private-log-only: public=%q detail=%q", err, detail.LogDetail())
	}
}

func TestCoursesWithNullableIDs(t *testing.T) {
	for _, raw := range []string{
		`[{"id":"chapter","courseId":null,"coursesId":null,"sort":1,"lastList":[{"id":null,"courseId":null,"coursesId":123,"sort":2}]}]`,
		`[{"id":null,"courseId":456,"coursesId":null,"sort":1,"lastList":[{"id":null,"courseId":null,"coursesId":123,"sort":2}]}]`,
		`[{"id":"chapter","sort":1,"lastList":[{"id":123,"courseId":null,"coursesId":null,"sort":2}]}]`,
	} {
		p := &fakePage{responses: []response{resp(raw)}}
		c := Client{Page: p, BaseURL: "https://example.invalid/", ClassroomID: "class"}
		courses, err := c.ListCourses(context.Background(), "practice")
		if err != nil || len(courses) != 1 || courses[0].Identifier() != "123" || courses[0].Number() != "1-2" {
			t.Fatalf("courses: %+v, err: %v", courses, err)
		}
	}
}

func TestCoursesRejectMissingExperimentID(t *testing.T) {
	for _, raw := range []string{
		`[{"id":"chapter","lastList":[{"id":null,"courseId":null,"coursesId":null}]}]`,
		`[{"id":null,"courseId":null,"coursesId":null,"coursesType":"practice"}]`,
		`[{"lastList":[{"coursesName":"missing ID"}]}]`,
	} {
		p := &fakePage{responses: []response{resp(raw)}}
		c := Client{Page: p, BaseURL: "https://example.invalid/", ClassroomID: "class"}
		if _, err := c.ListCourses(context.Background(), "practice"); err == nil || !strings.Contains(err.Error(), "缺少实验 ID") {
			t.Fatalf("expected missing experiment ID error, got %v", err)
		}
	}
}

func TestMultipleClassroomsRequireSelection(t *testing.T) {
	p := &fakePage{responses: []response{resp(`{"classroomListInfo":{"rows":[{"id":"first"},{"id":"second"}]}}`)}}
	c := Client{Page: p, BaseURL: "https://example.invalid/"}
	if err := c.EnsureClassroom(context.Background()); err == nil || c.ClassroomID != "" {
		t.Fatal("silently selected the first classroom", c.ClassroomID, err)
	}
}
