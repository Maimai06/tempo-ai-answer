package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Maimai06/tempo-ai-answer/internal/mirror"
)

func main() {
	dry := flag.Bool("dry-run", false, "只校验推送计划")
	flag.Parse()
	// askpass 文件只包含变量名；Token 不进入参数、文件或 remote URL。
	if os.Getenv("GITHUB_TOKEN") != "" {
		dir, err := os.MkdirTemp("", "tempo-askpass-")
		if err != nil {
			fail(err)
		}
		defer os.RemoveAll(dir)
		path := filepath.Join(dir, "askpass.sh")
		body := "#!/bin/sh\ncase \"$1\" in *github.com*) ;; *) exit 1 ;; esac\ncase \"$1\" in *Username*) printf '%s' x-access-token ;; *) printf '%s' \"$GITHUB_TOKEN\" ;; esac\n"
		if err = os.WriteFile(path, []byte(body), 0700); err != nil {
			fail(err)
		}
		if err = os.Setenv("GIT_ASKPASS", path); err != nil {
			fail(err)
		}
	}
	if err := os.Setenv("GIT_TERMINAL_PROMPT", "0"); err != nil {
		fail(err)
	}
	ref := os.Getenv("TEMPO_VERIFIED_REF")
	commit := os.Getenv("CNB_COMMIT")
	if os.Getenv("CI") != "" && (ref == "" || commit == "") {
		fail(fmt.Errorf("CI 必须提供 TEMPO_VERIFIED_REF 和 CNB_COMMIT"))
	}
	err := mirror.Run(context.Background(), mirror.Options{Source: "https://cnb.cool/wuyin.keji/Tools/tempo-ai-answer", Target: "https://github.com/Maimai06/tempo-ai-answer", ExpectedRef: ref, ExpectedCommit: commit, DryRun: *dry})
	if err != nil {
		fail(err)
	}
	fmt.Println("镜像完成（dry-run:", *dry, ")")
}
func fail(err error) { fmt.Fprintln(os.Stderr, err); os.Exit(1) }
