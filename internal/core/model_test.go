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
