// Package mirror 将 CNB 的分支与标签原子同步至 GitHub。
package mirror

import (
	"context"
	"fmt"
	"maps"
	"os"
	"os/exec"
	"regexp"
	"sort"
	"strings"
)

// Options 限定镜像源、目标及已验证提交，拒绝过期流水线。
type Options struct {
	Source, Target, ExpectedRef, ExpectedCommit string
	DryRun                                      bool
}

func git(ctx context.Context, dir string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	// Git 输出中可能含凭据；先脱敏再附加诊断信息。
	out, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			if diagnostic := sanitizeGitDiagnostic(string(exitErr.Stderr)); diagnostic != "" {
				return "", fmt.Errorf("git %s 失败: %w: %s", args[0], err, diagnostic)
			}
		}
		return "", fmt.Errorf("git %s 失败: %w", args[0], err)
	}
	return strings.TrimSpace(string(out)), nil
}

var credentialURL = regexp.MustCompile(`(?i)(https?://)[^\s/@]+@`)

func sanitizeGitDiagnostic(diagnostic string) string {
	diagnostic = strings.Join(strings.Fields(diagnostic), " ")
	for _, name := range []string{"GITHUB_TOKEN", "CNB_TOKEN"} {
		if token := os.Getenv(name); token != "" {
			diagnostic = strings.ReplaceAll(diagnostic, token, "[redacted]")
		}
	}
	diagnostic = credentialURL.ReplaceAllString(diagnostic, `${1}[redacted]@`)
	if runes := []rune(diagnostic); len(runes) > 1000 {
		diagnostic = string(runes[:1000])
	}
	return diagnostic
}

func refs(text string) map[string]string {
	out := map[string]string{}
	for _, line := range strings.Split(text, "\n") {
		f := strings.Fields(line)
		if len(f) == 2 && (strings.HasPrefix(f[1], "refs/heads/") || strings.HasPrefix(f[1], "refs/tags/")) && !strings.HasSuffix(f[1], "^{}") {
			out[f[1]] = f[0]
		}
	}
	return out
}

// Plan 产生带显式 lease 的参数；空期望值也保护新建 ref。
func Plan(source, target map[string]string) []string {
	keys := map[string]bool{}
	for k := range source {
		keys[k] = true
	}
	for k := range target {
		keys[k] = true
	}
	names := []string{}
	for k := range keys {
		names = append(names, k)
	}
	sort.Strings(names)
	args := []string{"push", "--atomic", "--porcelain"}
	for _, k := range names {
		args = append(args, "--force-with-lease="+k+":"+target[k])
	}
	args = append(args, "target")
	for _, k := range names {
		if source[k] == "" {
			args = append(args, ":"+k)
		} else {
			args = append(args, k+":"+k)
		}
	}
	return args
}

// Run 拉取完整源引用，在隔离裸库中执行整仓同步，不修改调用者工作区。
func Run(ctx context.Context, o Options) error {
	if o.Source == "" || o.Target == "" || strings.HasPrefix(o.Source, "-") || strings.HasPrefix(o.Target, "-") {
		return fmt.Errorf("镜像源或目标无效")
	}
	dir, err := os.MkdirTemp("", "tempo-mirror-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	if _, err = git(ctx, dir, "init", "--bare", "."); err != nil {
		return err
	}
	for _, r := range []struct{ name, url string }{{"source", o.Source}, {"target", o.Target}} {
		if _, err = git(ctx, dir, "remote", "add", r.name, r.url); err != nil {
			return err
		}
	}
	if _, err = git(ctx, dir, "fetch", "--no-tags", "source", "+refs/heads/*:refs/heads/*", "+refs/tags/*:refs/tags/*"); err != nil {
		return err
	}
	s, err := git(ctx, dir, "show-ref")
	if err != nil {
		return err
	}
	source := refs(s)
	if len(source) == 0 {
		return fmt.Errorf("源仓库为空，拒绝删除目标")
	}
	if o.ExpectedRef != "" {
		verified, e := git(ctx, dir, "rev-parse", o.ExpectedRef+"^{commit}")
		if e != nil || verified != o.ExpectedCommit {
			return fmt.Errorf("源提交已变化或验证 ref 不存在，等待新的验证流水线")
		}
	}
	t, err := git(ctx, dir, "ls-remote", "--refs", "target", "refs/heads/*", "refs/tags/*")
	if err != nil {
		return err
	}
	target := refs(t)
	args := Plan(source, target)
	// 若没有变化，仍验证快照，不发空 refspec 推送。
	same := len(source) == len(target)
	for k, v := range source {
		same = same && target[k] == v
	}
	if same {
		return nil
	}
	fresh, err := git(ctx, dir, "ls-remote", "--refs", "source", "refs/heads/*", "refs/tags/*")
	if err != nil {
		return err
	}
	if !maps.Equal(refs(fresh), source) {
		return fmt.Errorf("CNB 引用在同步准备期间变化，请重试新流水线")
	}
	if o.DryRun {
		args = append(args[:1], append([]string{"--dry-run"}, args[1:]...)...)
	}
	_, err = git(ctx, dir, args...)
	if err != nil || o.DryRun {
		return err
	}
	actual, err := git(ctx, dir, "ls-remote", "--refs", "target", "refs/heads/*", "refs/tags/*")
	if err != nil {
		return err
	}
	if !maps.Equal(refs(actual), source) {
		return fmt.Errorf("镜像后远端引用与源快照不一致，可能有并发新增分支")
	}
	return nil
}
