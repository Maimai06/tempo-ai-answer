// build 在任意主机生成四目标轻量包，构建清单明确浏览器未随包。
package main

import (
	"archive/zip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

func must(e error) {
	if e != nil {
		fmt.Fprintln(os.Stderr, e)
		os.Exit(1)
	}
}
func digest(path string) string {
	b, e := os.ReadFile(path)
	must(e)
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}
func command(args ...string) string {
	b, e := exec.Command(args[0], args[1:]...).Output()
	must(e)
	return strings.TrimSpace(string(b))
}
func main() {
	out := "artifacts/release"
	must(os.MkdirAll(out, 0755))
	commit := command("git", "rev-parse", "HEAD")
	version := os.Getenv("CNB_BRANCH")
	if version == "" || os.Getenv("CNB_IS_TAG") != "true" {
		version = "dev-" + commit[:8]
	}
	version = strings.ReplaceAll(version, "/", "-")
	sums := ""
	licenses := notices()
	dirty := command("git", "status", "--porcelain") != ""
	for _, target := range [][2]string{{"windows", "amd64"}, {"darwin", "amd64"}, {"darwin", "arm64"}, {"linux", "amd64"}} {
		name := "tempo-ai-answer"
		if target[0] == "windows" {
			name += ".exe"
		}
		dir, e := os.MkdirTemp("", "tempo-build-")
		must(e)
		bin := filepath.Join(dir, name)
		cmd := exec.Command("go", "build", "-trimpath", "-buildvcs=false", "-ldflags", "-s -w -X main.version="+version+" -X main.commit="+commit, "-o", bin, "./cmd/tempo-ai-answer")
		cmd.Env = append(os.Environ(), "CGO_ENABLED=0", "GOOS="+target[0], "GOARCH="+target[1])
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		must(cmd.Run())
		manifest := map[string]any{"version": version, "commit": commit, "go": runtime.Version(), "goos": target[0], "goarch": target[1], "cgo": false, "variant": "executable-only", "dirty": dirty, "browser": "system browser required", "go_mod_sha256": digest("go.mod"), "go_sum_sha256": digest("go.sum"), "binary_sha256": digest(bin)}
		mb, e := json.MarshalIndent(manifest, "", "  ")
		must(e)
		zipname := "tempo-ai-answer-" + version + "-" + target[0] + "-" + target[1] + "-executable-only.zip"
		file, e := os.Create(filepath.Join(out, zipname))
		must(e)
		zw := zip.NewWriter(file)
		add := func(n string, b []byte, mode os.FileMode) {
			h := &zip.FileHeader{Name: n, Method: zip.Deflate}
			h.SetMode(mode)
			w, e := zw.CreateHeader(h)
			must(e)
			_, e = w.Write(b)
			must(e)
		}
		b, e := os.ReadFile(bin)
		must(e)
		add(name, b, 0755)
		add("manifest.json", mb, 0644)
		add("THIRD_PARTY_NOTICES.txt", licenses, 0644)
		for _, p := range []string{"LICENSE", "README.md", "config.example.json"} {
			b, e := os.ReadFile(p)
			must(e)
			add(p, b, 0644)
		}
		if target[0] == "darwin" {
			add("启动.command", []byte("#!/bin/sh\ncd -- \"$(dirname -- \"$0\")\" || exit 1\n./tempo-ai-answer\nstatus=$?\nprintf '\\n按回车关闭窗口…'\nread -r ignored\nexit \"$status\"\n"), 0755)
		}
		must(zw.Close())
		must(file.Close())
		sums += digest(filepath.Join(out, zipname)) + "  " + zipname + "\n"
		must(os.RemoveAll(dir))
		fmt.Println(zipname)
	}
	must(os.WriteFile(filepath.Join(out, "SHA256SUMS"), []byte(sums), 0644))
}
