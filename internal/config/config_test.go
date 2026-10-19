package config

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func good() Config {
	c := Defaults()
	c.Platform.BaseURL = "https://example.invalid/tempo-talents/"
	c.Platform.Username = "fixture"
	c.Platform.Password = "secret-fixture"
	return c
}
func TestDefaultsAndValidation(t *testing.T) {
	c := good()
	b, e := json.Marshal(c)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = Decode(b); e != nil {
		t.Fatal(e)
	}
	raw := []byte(`{"platform":{"base_url":"https://example.invalid/","username":"fixture","password":"fixture"},"run":{"skip_passed":false,"cleanup_chat":false,"max_retry":0,"delay_between_steps":0}}`)
	v, e := Decode(raw)
	if e != nil || v.Run.SkipPassed || v.Run.CleanupChat || v.Run.MaxRetry != 0 || v.Run.Delay != 0 || v.Run.PerChat != 3 {
		t.Fatal(v, e)
	}
	for _, raw := range []string{`null`, `{`, `[]`, `{"platform":null}`, `{} {}`, `{"run":{"max_retry":"two"}}`} {
		if _, e := Decode([]byte(raw)); e == nil {
			t.Fatal(raw)
		}
	}
	cases := []func(*Config){func(c *Config) { c.Platform.BaseURL = "YOUR_HOST" }, func(c *Config) { c.Platform.BaseURL = "file:///tmp" }, func(c *Config) { c.Platform.BaseURL = "https://user:pass@example.invalid" }, func(c *Config) { c.Platform.Username = "" }, func(c *Config) { c.Platform.Password = "YOUR_PASSWORD" }, func(c *Config) { c.AI.AnswerTimeout = 0 }, func(c *Config) { c.AI.AnswerTimeout = 3601 }, func(c *Config) { c.Run.MaxRetry = -1 }, func(c *Config) { c.Run.MaxRetry = 21 }, func(c *Config) { c.Run.Delay = -1 }, func(c *Config) { c.Run.Delay = 3601 }, func(c *Config) { c.Run.PerChat = 0 }, func(c *Config) { c.Run.PerChat = 101 }}
	for _, change := range cases {
		v := good()
		change(&v)
		if v.Validate() == nil {
			t.Fatal(v)
		}
	}
}
func TestLoad(t *testing.T) {
	p := filepath.Join(t.TempDir(), "a", "config.json")
	if _, e := Load(p); !errors.Is(e, ErrCreated) {
		t.Fatal(e)
	}
	if _, e := Load(p); e == nil {
		t.Fatal("placeholder accepted")
	}
	c := good()
	b, _ := json.Marshal(c)
	if e := os.WriteFile(p, b, 0600); e != nil {
		t.Fatal(e)
	}
	if _, e := Load(p); e != nil {
		t.Fatal(e)
	}
	var out bytes.Buffer
	if e := CopyTemplate(&out); e != nil || !bytes.Equal(out.Bytes(), Template) {
		t.Fatal(e)
	}
	if _, e := Load(filepath.Dir(p)); e == nil {
		t.Fatal("directory accepted")
	}
	if d, e := DataDir(); e != nil || filepath.Base(d) != "tempo-ai-answer" {
		t.Fatal(d, e)
	}
}
func TestInvalidLocalEnvironment(t *testing.T) {
	if !hasNull([]any{1, nil}) || hasNull([]any{1, "x"}) {
		t.Fatal("nested null")
	}
	p := filepath.Join(t.TempDir(), "file")
	if e := os.WriteFile(p, []byte("x"), 0600); e != nil {
		t.Fatal(e)
	}
	if _, e := Load(filepath.Join(p, "child", "config.json")); e == nil {
		t.Fatal("non-directory parent accepted")
	}
	t.Setenv("HOME", "")
	t.Setenv("APPDATA", "")
	t.Setenv("XDG_CONFIG_HOME", "")
	if _, e := DataDir(); e == nil {
		t.Fatal("missing user config dir accepted")
	}
}
