package core

import (
	"encoding/json"
	"testing"
)

func TestSnapshotDeepCopy(t *testing.T) {
	visible := true
	original := Step{Info: QuestionInfo{Codes: []Code{{Code: "original"}}, Choices: []Choice{{ID: []byte(`1`), Content: []string{"original"}}}}, Trials: []Trial{{Visible: &visible, ID: []byte(`1`)}}}
	snapshot := NewSnapshot(original)
	original.Info.Codes[0].Code = "bad"
	original.Info.Choices[0].Content[0] = "bad"
	original.Info.Choices[0].ID[0] = '2'
	visible = false
	copy := snapshot.Step()
	copy.Info.Codes[0].Code = "bad"
	copy.Info.Choices[0].Content[0] = "bad"
	copy.Info.Choices[0].ID[0] = '3'
	*copy.Trials[0].Visible = false
	fresh := snapshot.Step()
	if fresh.Info.Codes[0].Code != "original" || fresh.Info.Choices[0].Content[0] != "original" || string(fresh.Info.Choices[0].ID) != "1" || !*fresh.Trials[0].Visible {
		t.Fatal(fresh)
	}
}
func TestIDs(t *testing.T) {
	for _, text := range []string{`"abc"`, `123`} {
		var id ID
		if e := json.Unmarshal([]byte(text), &id); e != nil || id == "" {
			t.Fatal(e, id)
		}
	}
	for _, text := range []string{`null`, `{}`, `1.2`} {
		var id ID
		if e := json.Unmarshal([]byte(text), &id); e == nil {
			t.Fatal(text)
		}
	}
}

func TestCourseNullableIDs(t *testing.T) {
	var courses []Course
	raw := `[{"id":1,"coursesId":null,"courseId":null,"coursesName":"第一章","lastList":[{"id":null,"coursesId":9007199254740993,"courseId":null,"coursesName":"实验"}]}]`
	if err := json.Unmarshal([]byte(raw), &courses); err != nil {
		t.Fatal(err)
	}
	if len(courses) != 1 || courses[0].ID != "1" || len(courses[0].Children) != 1 || courses[0].Children[0].Identifier() != "9007199254740993" {
		t.Fatalf("unexpected courses: %+v", courses)
	}
	// 显式 null 应清空旧值，避免复用结构体时误用上一次的实验 ID。
	c := Course{ID: "old", CoursesID: "old", CourseID: "old"}
	if err := json.Unmarshal([]byte(`{"id":null,"coursesId":null,"courseId":null}`), &c); err != nil || c.Identifier() != "" {
		t.Fatalf("unexpected reused course: %+v, %v", c, err)
	}
	for _, field := range []string{"id", "coursesId", "courseId"} {
		for _, value := range []string{`{}`, `[]`, `true`, `1.2`} {
			var c Course
			if err := json.Unmarshal([]byte(`{"`+field+`":`+value+`}`), &c); err == nil {
				t.Fatalf("accepted invalid course ID: %s=%s", field, value)
			}
		}
	}
}
