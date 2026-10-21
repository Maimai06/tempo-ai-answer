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
			if diagnostic := sanitizeGitFailure(exitErr.Stderr, out); diagnostic != "" {
				return "", fmt.Errorf("git %s 失败: %w: %s", args[0], err, diagnostic)
			}
		}
		return "", fmt.Errorf("git %s 失败: %w", args[0], err)
	}
	return strings.TrimSpace(string(out)), nil
}

var credentialURL = regexp.MustCompile(`(?i)(https?://)[^\s/@]+@`)

func sanitizeGitFailure(stderr, stdout []byte) string {
	return sanitizeGitDiagnostic(string(stderr) + "\n" + string(stdout))
}

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

// ConflictingRefs 返回目标中不在源快照内、且阻止源 ref 创建的文件/目录冲突项。
func ConflictingRefs(source, target map[string]string) []string {
	conflicts := []string{}
	for targetRef := range target {
		if source[targetRef] != "" {
			continue
		}
		for sourceRef := range source {
			if strings.HasPrefix(sourceRef, targetRef+"/") || strings.HasPrefix(targetRef, sourceRef+"/") {
				conflicts = append(conflicts, targetRef)
				break
			}
		}
	}
	sort.Strings(conflicts)
	return conflicts
}

func deletionPlan(refs []string, target map[string]string) []string {
	args := []string{"push", "--atomic", "--porcelain"}
	for _, ref := range refs {
		args = append(args, "--force-with-lease="+ref+":"+target[ref])
	}
	args = append(args, "target")
	for _, ref := range refs {
		args = append(args, ":"+ref)
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
	conflicts := ConflictingRefs(source, target)
	deletions := deletionPlan(conflicts, target)
	// 若没有变化，仍验证快照，不发空 refspec 推送。
	same := len(source) == len(target)
	for k, v := range source {
		same = same && target[k] == v
	}
	if same {
		return nil
	}
	if err = verifySourceSnapshot(ctx, dir, source); err != nil {
		return err
	}
	if o.DryRun {
		if len(deletions) > 0 {
			fmt.Println("dry-run: 先清理路径冲突 ref:", strings.Join(deletions, " "))
		}
		fmt.Println("dry-run: 原子同步 ref:", strings.Join(Plan(source, expectedTarget(target, conflicts)), " "))
		return nil
	}
	if len(conflicts) > 0 {
		if _, err = git(ctx, dir, deletions...); err != nil {
			return err
		}
		cleaned, e := git(ctx, dir, "ls-remote", "--refs", "target", "refs/heads/*", "refs/tags/*")
		if e != nil {
			return e
		}
		actualTarget := refs(cleaned)
		if !maps.Equal(actualTarget, expectedTarget(target, conflicts)) {
			return fmt.Errorf("GitHub 引用在清理路徑冲突后发生并发变化，已停止同步")
		}
		target = actualTarget
		if err = verifySourceSnapshot(ctx, dir, source); err != nil {
			return err
		}
	}
	if _, err = git(ctx, dir, Plan(source, target)...); err != nil {
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

func expectedTarget(target map[string]string, deleted []string) map[string]string {
	expected := maps.Clone(target)
	for _, ref := range deleted {
		delete(expected, ref)
	}
	return expected
}

func verifySourceSnapshot(ctx context.Context, dir string, source map[string]string) error {
	fresh, err := git(ctx, dir, "ls-remote", "--refs", "source", "refs/heads/*", "refs/tags/*")
	if err != nil {
		return err
	}
	if !maps.Equal(refs(fresh), source) {
		return fmt.Errorf("CNB 引用在同步准备期间变化，请重试新流水线")
	}
	return nil
}
