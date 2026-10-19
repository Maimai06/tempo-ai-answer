package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
)

// notices 随二进制保留 Go 与全部模块的许可证及 NOTICE，避免漏发。
func notices() []byte {
	var out strings.Builder
	license, err := os.ReadFile(filepath.Join(runtime.GOROOT(), "LICENSE"))
	if os.IsNotExist(err) {
		license, err = os.ReadFile(filepath.Join(filepath.Dir(runtime.GOROOT()), "LICENSE"))
	}
	must(err)
	fmt.Fprintf(&out, "Go %s\n%s\n\n", runtime.Version(), license)
	type module struct {
		Path, Version, Dir string
		Main               bool
	}
	modules := map[string]module{}
	for _, target := range [][2]string{{"windows", "amd64"}, {"darwin", "amd64"}, {"darwin", "arm64"}, {"linux", "amd64"}} {
		cmd := exec.Command("go", "list", "-deps", "-json", "./cmd/tempo-ai-answer")
		cmd.Env = append(os.Environ(), "CGO_ENABLED=0", "GOOS="+target[0], "GOARCH="+target[1])
		cmd.Stderr = os.Stderr
		data, err := cmd.Output()
		must(err)
		dec := json.NewDecoder(strings.NewReader(string(data)))
		for {
			var pkg struct{ Module *module }
			err := dec.Decode(&pkg)
			if err == io.EOF {
				break
			}
			must(err)
			if pkg.Module != nil && !pkg.Module.Main {
				modules[pkg.Module.Path] = *pkg.Module
			}
		}
	}
	keys := []string{}
	for path := range modules {
		keys = append(keys, path)
	}
	sort.Strings(keys)
	for _, path := range keys {
		mod := modules[path]
		found := false
		for _, name := range []string{"LICENSE", "LICENSE.txt", "LICENSE.md", "COPYING", "NOTICE"} {
			b, err := os.ReadFile(filepath.Join(mod.Dir, name))
			if os.IsNotExist(err) {
				continue
			}
			must(err)
			fmt.Fprintf(&out, "%s %s / %s\n%s\n\n", mod.Path, mod.Version, name, b)
			if name != "NOTICE" {
				found = true
			}
		}
		if !found {
			must(fmt.Errorf("缺少模块许可证: %s %s", mod.Path, mod.Version))
		}
	}
	return []byte(out.String())
}
