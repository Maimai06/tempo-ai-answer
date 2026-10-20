package logger

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
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

func TestReadableConsoleKeepsRedactedDiagnosticFile(t *testing.T) {
	dir := t.TempDir()
	console := &bytes.Buffer{}
	l, err := OpenWithConsole(dir, console, "private-pass")
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Go(func() { l.Log.WithGroup("login").With("password", "private-pass").Info("正在登录 private-pass") })
	}
	wg.Wait()
	l.RecordError(errors.New("strconv.ParseInt private-pass token=hidden-token"))
	if err = l.Close(); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(dir, "tempo.log"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(console.String(), "[提示] 正在登录") || strings.Contains(console.String(), "time=") || strings.Contains(console.String(), "strconv.ParseInt") {
		t.Fatal(console.String())
	}
	if !strings.Contains(string(b), "time=") || !strings.Contains(string(b), "strconv.ParseInt") {
		t.Fatal("missing file diagnostics")
	}
	for _, text := range []string{console.String(), string(b)} {
		if strings.Contains(text, "private-pass") || strings.Contains(text, "hidden-token") {
			t.Fatal("secret leaked")
		}
	}
}
