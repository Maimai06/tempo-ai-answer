package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Maimai06/tempo-ai-answer/internal/mirror"
)

// askpassScript 只响应固定 HTTPS 主机，临时令牌仅从环境读取。
const askpassScript = `#!/bin/sh
case "$1" in
  "Username for 'https://github.com': "|"Username for 'https://github.com/"*)
    [ -n "$GITHUB_TOKEN" ] || exit 1
    printf '%s' x-access-token ;;
  "Password for 'https://x-access-token@github.com': "|"Password for 'https://x-access-token@github.com/"*)
    [ -n "$GITHUB_TOKEN" ] || exit 1
    printf '%s' "$GITHUB_TOKEN" ;;
  "Username for 'https://cnb.cool': "|"Username for 'https://cnb.cool/"*)
    [ -n "$CNB_TOKEN" ] || exit 1
    printf '%s' cnb ;;
  "Password for 'https://cnb@cnb.cool': "|"Password for 'https://cnb@cnb.cool/"*)
    [ -n "$CNB_TOKEN" ] || exit 1
    printf '%s' "$CNB_TOKEN" ;;
  *) exit 1 ;;
esac
`

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	dry := flag.Bool("dry-run", false, "只校验推送计划")
	flag.Parse()
	if err := validateExecution(*dry); err != nil {
		return err
	}
	// askpass 文件只包含变量名；Token 不进入参数、文件或 remote URL。
	if os.Getenv("GITHUB_TOKEN") != "" || os.Getenv("CNB_TOKEN") != "" {
		dir, err := os.MkdirTemp("", "tempo-askpass-")
		if err != nil {
			return err
		}
		defer os.RemoveAll(dir)
		path := filepath.Join(dir, "askpass.sh")
		if err = os.WriteFile(path, []byte(askpassScript), 0700); err != nil {
			return err
		}
		if err = os.Setenv("GIT_ASKPASS", path); err != nil {
			return err
		}
		// Git 可能按本地语言生成提示；固定语言后再匹配 HTTPS 凭据提示。
		if err = os.Setenv("LC_ALL", "C"); err != nil {
			return err
		}
	}
	if err := os.Setenv("GIT_TERMINAL_PROMPT", "0"); err != nil {
		return err
	}
	ref := os.Getenv("TEMPO_VERIFIED_REF")
	commit := os.Getenv("CNB_COMMIT")
	err := mirror.Run(context.Background(), mirror.Options{Source: "https://cnb.cool/wuyin.keji/Tools/tempo-ai-answer", Target: "https://github.com/Maimai06/tempo-ai-answer", ExpectedRef: ref, ExpectedCommit: commit, DryRun: *dry})
	if err != nil {
		return err
	}
	fmt.Println("镜像完成（dry-run:", *dry, ")")
	return nil
}

// validateExecution 将实际写入限定为 CNB 流水线，本地仅允许核对计划。
func validateExecution(dry bool) error {
	if dry {
		return nil
	}
	if os.Getenv("CNB") != "true" || os.Getenv("CI") != "true" {
		return fmt.Errorf("实际同步仅允许 CNB 流水线执行；请推送代码到 CNB，本地仅可使用 --dry-run")
	}
	for _, key := range []string{"CNB_TOKEN", "GITHUB_TOKEN", "TEMPO_VERIFIED_REF", "CNB_COMMIT"} {
		if os.Getenv(key) == "" {
			return fmt.Errorf("CNB 镜像阶段缺少 %s", key)
		}
	}
	return nil
}
