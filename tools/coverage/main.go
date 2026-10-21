// coverage 对 PRD 指定的每个纯逻辑包单独检查语句覆盖率。
package main

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
)

func main() {
	if len(os.Args) != 2 {
		panic("需要 coverprofile 路径")
	}
	f, e := os.Open(os.Args[1])
	if e != nil {
		panic(e)
	}
	defer f.Close()
	total := map[string]int{}
	hit := map[string]int{}
	s := bufio.NewScanner(f)
	for s.Scan() {
		fields := strings.Fields(s.Text())
		if len(fields) != 3 {
			continue
		}
		path := strings.Split(fields[0], ":")[0]
		parts := strings.Split(path, "/")
		pkg := parts[len(parts)-2]
		n, e := strconv.Atoi(fields[1])
		if e != nil {
			panic(e)
		}
		total[pkg] += n
		if fields[2] != "0" {
			hit[pkg] += n
		}
	}
	if e = s.Err(); e != nil {
		panic(e)
	}
	ok := true
	for _, p := range []string{"config", "scope", "prompts", "codemerge"} {
		pct := 0.0
		if total[p] > 0 {
			pct = 100 * float64(hit[p]) / float64(total[p])
		}
		fmt.Printf("%s: %.1f%%\n", p, pct)
		ok = ok && pct >= 90
	}
	if !ok {
		os.Exit(1)
	}
}
