package logger

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Maimai06/tempo-ai-answer/internal/core"
)

func TestSecretsRedacted(t *testing.T) {
	dir := t.TempDir()
	l, e := Open(dir, "private-pass", "private-user")
	if e != nil {
		t.Fatal(e)
	}
	l.Consume(core.Event{Kind: "log", Text: "private-user private-pass token=abc123 cookie:secret"})
	if e = l.Close(); e != nil {
		t.Fatal(e)
	}
	b, e := os.ReadFile(filepath.Join(dir, "tempo.log"))
	if e != nil {
		t.Fatal(e)
	}
	for _, s := range []string{"private-pass", "private-user", "abc123", "secret"} {
		if strings.Contains(string(b), s) {
			t.Fatal("secret leaked")
		}
	}
}
