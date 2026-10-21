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

func TestConfigPathPriority(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(t.TempDir(), "中文 数据")
	explicit := filepath.Join(t.TempDir(), "自定义 配置.json")
	for _, tc := range []struct {
		name, dataDir, configPath, want string
	}{
		{"portable", "", "", filepath.Join(filepath.Dir(exe), "config.json")},
		{"data directory", dir, "", filepath.Join(dir, "config.json")},
		{"explicit config", "", explicit, explicit},
		{"explicit config wins", dir, explicit, explicit},
		{"relative config", dir, "custom.json", "custom.json"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a := &app{dataDir: tc.dataDir, configPath: tc.configPath}
			if err := a.paths(); err != nil {
				t.Fatal(err)
			}
			want, err := filepath.Abs(tc.want)
			if err != nil {
				t.Fatal(err)
			}
			if a.configPath != want || !filepath.IsAbs(a.dataDir) {
				t.Fatalf("config=%q, want %q; data=%q", a.configPath, want, a.dataDir)
			}
			if err := a.paths(); err != nil || a.configPath != want {
				t.Fatalf("repeated resolution changed config: %q, %v", a.configPath, err)
			}
		})
	}
}

func TestConfigErrorShowsLoadedPath(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "配置 文件.json")
	if err := os.WriteFile(path, []byte(`{}`), 0600); err != nil {
		t.Fatal(err)
	}
	root := command()
	root.SetArgs([]string{"--data-dir", dir, "--config", path, "run", "--dry-run"})
	if err := root.Execute(); err == nil || !strings.Contains(err.Error(), path) || !strings.Contains(err.Error(), "platform.base_url") {
		t.Fatalf("expected actual config path in error, got %v", err)
	}
}

func TestUnattendedRunRequiresExperimentScope(t *testing.T) {
	dir := t.TempDir()
	for _, flag := range []string{"--yes", "--headless"} {
		root := command()
		root.SetArgs([]string{"--data-dir", dir, "run", flag, "--steps", "1", "--dry-run"})
		if err := root.Execute(); err == nil || !strings.Contains(err.Error(), "明确选择实验范围") {
			t.Fatal(err)
		}
		if _, err := os.Stat(filepath.Join(dir, "config.json")); !os.IsNotExist(err) {
			t.Fatal("unscoped unattended run touched user configuration")
		}
	}
}
