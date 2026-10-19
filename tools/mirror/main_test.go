package main

import (
	"os"
	"os/exec"
	"testing"
)

func TestMirrorWritesRequireCNBPipeline(t *testing.T) {
	keys := []string{"CNB", "CI", "CNB_TOKEN", "GITHUB_TOKEN", "TEMPO_VERIFIED_REF", "CNB_COMMIT"}
	for _, tc := range []struct {
		name  string
		dry   bool
		clear string
		ok    bool
	}{
		{"cnb-pipeline", false, "", true},
		{"local-write", false, "CNB", false},
		{"missing-ci-marker", false, "CI", false},
		{"missing-source-token", false, "CNB_TOKEN", false},
		{"missing-target-token", false, "GITHUB_TOKEN", false},
		{"missing-verified-ref", false, "TEMPO_VERIFIED_REF", false},
		{"missing-verified-commit", false, "CNB_COMMIT", false},
		{"local-dry-run", true, "all", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, key := range keys {
				value := "fixture"
				if key == "CNB" || key == "CI" {
					value = "true"
				}
				if key == tc.clear || tc.clear == "all" {
					value = ""
				}
				t.Setenv(key, value)
			}
			if err := validateExecution(tc.dry); (err == nil) != tc.ok {
				t.Fatalf("运行路径限制错误: %v", err)
			}
		})
	}
}

func TestAskpassCredentialsStayOnTheirHost(t *testing.T) {
	sh, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("镜像阶段在 Linux 运行，本机未安装 sh")
	}
	for _, tc := range []struct {
		name, prompt, github, cnb, want string
		ok                              bool
	}{
		{"github-user", "Username for 'https://github.com': ", "github-fixture", "cnb-fixture", "x-access-token", true},
		{"github-password", "Password for 'https://x-access-token@github.com': ", "github-fixture", "cnb-fixture", "github-fixture", true},
		{"cnb-user", "Username for 'https://cnb.cool': ", "github-fixture", "cnb-fixture", "cnb", true},
		{"cnb-password", "Password for 'https://cnb@cnb.cool': ", "github-fixture", "cnb-fixture", "cnb-fixture", true},
		{"github-repo-path", "Password for 'https://x-access-token@github.com/Maimai06/tempo-ai-answer': ", "github-fixture", "cnb-fixture", "github-fixture", true},
		{"cnb-repo-path", "Password for 'https://cnb@cnb.cool/wuyin.keji/Tools/tempo-ai-answer': ", "github-fixture", "cnb-fixture", "cnb-fixture", true},
		{"no-github-token", "Password for 'https://x-access-token@github.com': ", "", "cnb-fixture", "", false},
		{"no-cnb-token", "Password for 'https://cnb@cnb.cool': ", "github-fixture", "", "", false},
		{"untrusted-host", "Password for 'https://x-access-token@example.invalid': ", "github-fixture", "cnb-fixture", "", false},
		{"github-suffix", "Password for 'https://x-access-token@github.com.example.invalid': ", "github-fixture", "cnb-fixture", "", false},
		{"cnb-suffix", "Password for 'https://cnb@cnb.cool.example.invalid': ", "github-fixture", "cnb-fixture", "", false},
		{"host-in-path", "Password for 'https://example.invalid/github.com': ", "github-fixture", "cnb-fixture", "", false},
		{"plain-http", "Password for 'http://x-access-token@github.com': ", "github-fixture", "cnb-fixture", "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("GITHUB_TOKEN", tc.github)
			t.Setenv("CNB_TOKEN", tc.cnb)
			cmd := exec.Command(sh, "-c", askpassScript, "tempo-askpass", tc.prompt)
			cmd.Env = os.Environ()
			out, err := cmd.CombinedOutput()
			if (err == nil) != tc.ok || string(out) != tc.want {
				t.Fatalf("凭据响应与主机边界不符: success=%v, output=%q", err == nil, out)
			}
		})
	}
}
