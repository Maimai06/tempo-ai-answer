package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestHelpAndConfigCreation(t *testing.T) {
	dir := t.TempDir()
	root := command()
	root.SetArgs([]string{"--version"})
	if e := root.Execute(); e != nil {
		t.Fatal(e)
	}
	root = command()
	root.SetArgs([]string{"--data-dir", dir, "run", "--scope", "第x章"})
	if e := root.Execute(); e == nil {
		t.Fatal("invalid scope accepted")
	}
	if _, e := os.Stat(filepath.Join(dir, "config.json")); !os.IsNotExist(e) {
		t.Fatal("invalid scope wrote config")
	}
	root = command()
	root.SetArgs([]string{"--data-dir", dir, "run"})
	if e := root.Execute(); e != nil {
		t.Fatal(e)
	}
	b, e := os.ReadFile(filepath.Join(dir, "config.json"))
	if e != nil || !strings.Contains(string(b), "YOUR_PLATFORM_HOST") {
		t.Fatal(string(b), e)
	}
}
