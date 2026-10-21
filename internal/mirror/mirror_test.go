package mirror

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMirror(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	src := filepath.Join(root, "source")
	dst := filepath.Join(root, "target.git")
	other := filepath.Join(root, "other")
	run := func(dir string, args ...string) string {
		t.Helper()
		s, e := git(ctx, dir, args...)
		if e != nil {
			t.Fatal(e)
		}
		return s
	}
	for _, d := range []string{src, dst, other} {
		if e := os.MkdirAll(d, 0700); e != nil {
			t.Fatal(e)
		}
	}
	run(src, "init", "-b", "main")
	run(other, "init", "-b", "main")
	run(dst, "init", "--bare")
	commit := func(d, msg string) {
		run(d, "-c", "user.name=Fixture", "-c", "user.email=fixture@example.invalid", "commit", "--allow-empty", "-m", msg)
	}
	commit(src, "source")
	run(src, "branch", "legacy")
	run(src, "-c", "user.name=Fixture", "-c", "user.email=fixture@example.invalid", "tag", "-a", "python-v1", "-m", "archive")
	commit(other, "different root")
	run(other, "remote", "add", "target", dst)
	run(other, "push", "target", "main")
	run(other, "branch", "obsolete")
	run(other, "push", "target", "obsolete")
	o := Options{Source: src, Target: dst, ExpectedRef: "refs/heads/main", ExpectedCommit: run(src, "rev-parse", "HEAD")}
	o.DryRun = true
	if e := Run(ctx, o); e != nil {
		t.Fatal(e)
	}
	if run(dst, "rev-parse", "refs/heads/main") == o.ExpectedCommit {
		t.Fatal("dry-run changed target")
	}
	o.DryRun = false
	if e := Run(ctx, o); e != nil {
		t.Fatal(e)
	}
	if run(dst, "rev-parse", "refs/heads/main") != o.ExpectedCommit {
		t.Fatal("different root not mirrored")
	}
	s := run(dst, "show-ref")
	if strings.Contains(s, "obsolete") || !strings.Contains(s, "legacy") || !strings.Contains(s, "python-v1") {
		t.Fatal(s)
	}
	if e := Run(ctx, o); e != nil {
		t.Fatal(e)
	}
	o.ExpectedCommit = "stale"
	if e := Run(ctx, o); e == nil {
		t.Fatal("stale run accepted")
	}
	// 旧 lease 不能覆盖另一个并发写入者。
	old := run(dst, "rev-parse", "refs/heads/main")
	commit(other, "concurrent")
	run(other, "push", "--force-with-lease=refs/heads/main:"+old, "target", "main")
	if _, e := git(ctx, src, "push", "--atomic", "--force-with-lease=refs/heads/main:"+old, dst, "main:main"); e == nil {
		t.Fatal("stale lease accepted")
	}
}
func TestInvalid(t *testing.T) {
	if e := Run(context.Background(), Options{}); e == nil {
		t.Fatal("empty URLs")
	}
}

func TestSanitizeGitDiagnostic(t *testing.T) {
	t.Setenv("GITHUB_TOKEN", "github-secret")
	t.Setenv("CNB_TOKEN", "cnb-secret")
	input := "remote: auth failed https://user:password@example.invalid/path github-secret cnb-secret"
	got := sanitizeGitDiagnostic(input)
	for _, secret := range []string{"github-secret", "cnb-secret", "user:password"} {
		if strings.Contains(got, secret) {
			t.Fatalf("diagnostic leaked %q: %s", secret, got)
		}
	}
	for _, safe := range []string{"auth failed", "https://[redacted]@example.invalid/path", "[redacted]"} {
		if !strings.Contains(got, safe) {
			t.Fatalf("diagnostic lost %q: %s", safe, got)
		}
	}
}

func TestSanitizeGitDiagnosticLimitsLength(t *testing.T) {
	got := sanitizeGitDiagnostic(strings.Repeat("x", 1001))
	if len([]rune(got)) != 1000 {
		t.Fatalf("diagnostic length = %d, want 1000", len([]rune(got)))
	}
}

func TestSanitizeGitFailureIncludesBothStreams(t *testing.T) {
	got := sanitizeGitFailure([]byte("fatal: push failed"), []byte("! [remote rejected] main (protected branch)"))
	for _, want := range []string{"fatal: push failed", "remote rejected", "protected branch"} {
		if !strings.Contains(got, want) {
			t.Fatalf("diagnostic lost %q: %s", want, got)
		}
	}
}
