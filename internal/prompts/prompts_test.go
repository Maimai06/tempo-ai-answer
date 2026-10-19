package prompts

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
		Kind, Text, Feedback string
		Expected             json.RawMessage
		Step                 core.Step
		Items                []core.Choice
		Seq                  int
	}
	if e = json.Unmarshal(b, &fs); e != nil {
		t.Fatal(e)
	}
	for _, f := range fs {
		var got string
		switch f.Kind {
		case "prompt":
			got = Code(core.NewSnapshot(f.Step), f.Feedback, f.Seq)
		case "select":
			got = Select(f.Items, f.Feedback)
		case "html":
			got = HTMLText(f.Text)
		default:
			continue
		}
		var expected string
		if e := json.Unmarshal(f.Expected, &expected); e != nil {
			t.Fatal(e)
		}
		if got != expected {
			t.Fatalf("%s parity\ngot: %q\nwant: %q", f.Kind, got, expected)
		}
	}
}
func TestPromptEdges(t *testing.T) {
	s := core.Step{Description: strings.Repeat("中", 6001), Trials: []core.Trial{{Expected: "x", Input: "a"}, {}}}
	p := Code(core.NewSnapshot(s), "", 0)
	if strings.Contains(p, strings.Repeat("中", 6001)) || !strings.Contains(p, "最后一行没有换行") {
		t.Fatal(p)
	}
	for _, typ := range []string{"single_choice", "multiple_choice", "judge", "unknown"} {
		if TypeName(typ) == "" {
			t.Fatal(typ)
		}
	}
	if Select(nil, "") == "" {
		t.Fatal("empty select")
	}
}
