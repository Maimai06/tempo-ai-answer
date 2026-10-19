package codemerge

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/Maimai06/tempo-ai-answer/internal/core"
)

func TestV1Parity(t *testing.T) {
	b, e := os.ReadFile("../../testdata/v1-parity.json")
	if e != nil {
		t.Fatal(e)
	}
	var fs []struct {
		Kind, Text, ExpectedOutput, ActualOutput string
		Expected                                 json.RawMessage
	}
	if e = json.Unmarshal(b, &fs); e != nil {
		t.Fatal(e)
	}
	for _, f := range fs {
		switch f.Kind {
		case "choice":
			var want []string
			_ = json.Unmarshal(f.Expected, &want)
			got := ChoiceAnswers(f.Text, 3)
			if strings.Join(got, "|") != strings.Join(want, "|") {
				t.Fatal(f.Text, got, want)
			}
		case "extract", "diff":
			var want string
			_ = json.Unmarshal(f.Expected, &want)
			got := Extract(f.Text)
			if f.Kind == "diff" {
				got = Diff(f.ExpectedOutput, f.ActualOutput)
			}
			if got != want {
				t.Fatal(f.Kind, got, want)
			}
		}
	}
}
func TestMergeAndPaths(t *testing.T) {
	frame := "class A {\n/********* Begin *********/\nold\n/********* End *********/\n}"
	if Merge(frame, "") != frame || Merge(frame, "class B {}") != "class B {}" || Merge("none", "x") != "x" {
		t.Fatal("merge cases")
	}
	s := Merge(frame, "\n    if (true) {\n        x();\n    }\n")
	if !strings.Contains(s, "\n    if (true) {\n        x();\n    }\n") {
		t.Fatal(s)
	}
	s = Merge(frame, "/********* Begin *********/\nx();\n/********* End *********/")
	if strings.Contains(s, "old") {
		t.Fatal(s)
	}
	files := SplitMulti("===FILE: src/A.java===\n```java\nclass A {}\n```\n===FILE: B.java===\nclass B {}")
	if len(files) != 2 {
		t.Fatal(files)
	}
	for _, p := range []string{"src/A.java", "A.java", "B.java", "x/B.java"} {
		if _, ok := MatchPath(files, p); !ok {
			t.Fatal(p)
		}
	}
	if _, ok := MatchPath(map[string]string{"x/A.java": "x", "y/A.java": "y"}, "A.java"); ok {
		t.Fatal("ambiguous")
	}
	if _, ok := MatchPath(files, "C.java"); ok {
		t.Fatal("missing")
	}
	if _, ok := MatchPath(map[string]string{"./src/A.java": "x"}, "src/A.java"); !ok {
		t.Fatal("normalized")
	}
	if StripMarks("===FILE: A.java===\nx") != "x" {
		t.Fatal("marks")
	}
	if FixClass("public final class Wrong {}", "A.java") != "public final class A {}" || FixClass("class A {}", "A.java") != "class A {}" || FixClass("public class X {}", "X.ts") != "public class X {}" {
		t.Fatal("class")
	}
	if !Codable([]core.Code{{Path: "A.JAVA"}}) || Codable([]core.Code{{Path: "README.md"}}) {
		t.Fatal("codable")
	}
}
func TestCompleteAndAnswers(t *testing.T) {
	for _, s := range []string{"class A {}", "```java\nx\n```", "Begin End } blah"} {
		if !Complete(s, 1) {
			t.Fatal(s)
		}
	}
	for _, s := range []string{"Begin {", "text", ""} {
		if Complete(s, 1) {
			t.Fatal(s)
		}
	}
	if Complete("===FILE: A===\nx", 2) || Complete("===FILE: A===\nx\n===FILE: B===\nx", 2) || !Complete("===FILE: A===\n{}\n===FILE: B===\n{}", 2) {
		t.Fatal("multi")
	}
	if Letters("ac", "multiple_choice") != "0,2" || Letters("BA", "judge") != "1" || Letters("?", "single_choice") != "" {
		t.Fatal("letters")
	}
	if Repr("'\"\x00\\\r\t") != "'\\'\"\\x00\\\\\\r\\t'" {
		t.Fatal(Repr("'\"\x00\\\r\t"))
	}
	if ChoiceAnswers("9. A\n1. AA\n2. f", 2)[1] != "F" {
		t.Fatal("choice")
	}
}
func TestSummary(t *testing.T) {
	no := false
	r := core.Result{Trials: []core.Trial{{Expected: "x", Actual: "y"}, {Visible: &no}, {Passed: true, Expected: "ok", Actual: "ok"}}}
	passed, detail := Summarize(r)
	if passed || !strings.Contains(detail, "第 0 个字符") {
		t.Fatal(detail)
	}
	for _, r := range []core.Result{{Message: "fail"}, {Phase: "error"}, {Passed: true}} {
		_, _ = Summarize(r)
	}
}
func TestExtractThenMergeDedents(t *testing.T) {
	frame := "class A {\n/********* Begin *********/\n/********* End *********/\n}"
	answer := "```java\n    if (true) {\n        call();\n    }\n```"
	s := Merge(frame, Extract(answer))
	if !strings.Contains(s, "\n    if (true) {\n        call();\n    }\n") {
		t.Fatal(s)
	}
	if _, ok := MatchPath(map[string]string{"./src/A.java": "first", "../src/A.java": "second"}, "src/A.java"); ok {
		t.Fatal("normalized ambiguity accepted")
	}
}
