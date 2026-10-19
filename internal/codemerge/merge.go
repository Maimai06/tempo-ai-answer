// Package codemerge 提供纯函数代码提取、合并和字符差异提示。
package codemerge

import (
	"fmt"
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"github.com/Maimai06/tempo-ai-answer/internal/core"
)

// Begin 识别平台的 Begin 注释边界。
var Begin = regexp.MustCompile(`/\*{2,}\s*Begin\s*\*{2,}/`)

// End 识别平台的 End 注释边界。
var End = regexp.MustCompile(`/\*{2,}\s*End\s*\*{2,}/`)
var fileMark = regexp.MustCompile(`(?m)^\s*===\s*FILE:\s*(\S+?)\s*===\s*$`)
var fenceLine = regexp.MustCompile("(?m)^\\s*```\\w*\\s*$")
var stripMark = regexp.MustCompile(`(?m)^\s*===\s*FILE:.*?===\s*$`)
var full = regexp.MustCompile(`(?m)^\s*(?:package\s|(?:public\s+|final\s+|abstract\s+)*class\s+\w+)`)
var public = regexp.MustCompile(`public\s+(?:final\s+|abstract\s+)?class\s+(\w+)`)
var blocks = regexp.MustCompile("(?s)```(?:\\w+)?\\s*\n(.*?)```")
var codeStart = regexp.MustCompile(`(?m)^\s*(?:package\s|import\s|public\s+class\s|class\s|/\*{2,}\s*Begin)`)
var noise = regexp.MustCompile(`^\s*(以上|这段|说明|解释|运行结果|输出如下|注意[:：])`)

// Codable 判断文件列表是否包含支持的代码扩展名。
func Codable(codes []core.Code) bool {
	for _, c := range codes {
		switch strings.ToLower(path.Ext(c.Path)) {
		case ".java", ".py", ".c", ".cpp", ".h", ".js", ".ts", ".sql", ".sh", ".go", ".php":
			return true
		}
	}
	return false
}

// Extract 提取最长代码块并去掉页面工具栏噪声，片段先去公共缩进。
func Extract(answer string) string {
	b := blocks.FindAllStringSubmatch(answer, -1)
	code := ""
	if len(b) > 0 {
		for _, m := range b {
			if len([]rune(m[1])) > len([]rune(code)) {
				code = strings.TrimRightFunc(m[1], unicode.IsSpace)
			}
		}
	} else {
		skip := map[string]bool{}
		for _, s := range []string{"java", "python", "c", "cpp", "js", "sql", "bash", "shell", "复制", "下载", "copy", "download", "展开", "收起", "代码"} {
			skip[s] = true
		}
		lines := []string{}
		for _, s := range strings.Split(answer, "\n") {
			if !skip[strings.ToLower(strings.TrimSpace(s))] && !noise.MatchString(s) {
				lines = append(lines, s)
			}
		}
		code = strings.Trim(strings.Join(lines, "\n"), "\r\n")
	}
	if m := codeStart.FindStringIndex(code); m != nil && m[0] > 0 {
		code = code[m[0]:]
	}
	if !full.MatchString(code) {
		code = dedent(code)
	}
	return strings.TrimSpace(code)
}

// SplitMulti 按 FILE 标记解析多文件回答。
func SplitMulti(answer string) map[string]string {
	out := map[string]string{}
	m := fileMark.FindAllStringSubmatchIndex(answer, -1)
	for i, p := range m {
		end := len(answer)
		if i+1 < len(m) {
			end = m[i+1][0]
		}
		body := strings.TrimSpace(fenceLine.ReplaceAllString(answer[p[1]:end], ""))
		if body != "" {
			out[strings.TrimSpace(answer[p[2]:p[3]])] = body
		}
	}
	return out
}

// MatchPath 优先精确匹配；有同名文件歧义时拒绝猜测。
func MatchPath(files map[string]string, p string) (string, bool) {
	if body, ok := files[p]; ok {
		return body, true
	}
	exact, fallback := []string{}, []string{}
	for k := range files {
		kk := strings.TrimLeft(strings.TrimSpace(k), "./")
		if kk == p {
			exact = append(exact, k)
		} else if kk == path.Base(p) || strings.HasSuffix(kk, "/"+path.Base(p)) || strings.HasSuffix(p, "/"+kk) {
			fallback = append(fallback, k)
		}
	}
	if len(exact) == 1 {
		return files[exact[0]], true
	}
	if len(exact) > 1 {
		return "", false
	}
	if len(fallback) == 1 {
		return files[fallback[0]], true
	}
	return "", false
}

// StripMarks 清除残留 FILE 标记，避免混入提交代码。
func StripMarks(s string) string { return strings.TrimSpace(stripMark.ReplaceAllString(s, "")) }

// FixClass 仅将 Java public class 名对齐至目标文件名。
func FixClass(code, p string) string {
	if strings.ToLower(path.Ext(p)) != ".java" {
		return code
	}
	m := public.FindStringSubmatchIndex(code)
	if m == nil {
		return code
	}
	name := strings.TrimSuffix(path.Base(p), path.Ext(p))
	return code[:m[2]] + name + code[m[3]:]
}
func dedent(s string) string {
	lines := strings.Split(strings.Trim(s, "\r\n"), "\n")
	min := int(^uint(0) >> 1)
	for _, l := range lines {
		if strings.TrimSpace(l) == "" {
			continue
		}
		n := len(l) - len(strings.TrimLeft(l, " \t"))
		if n < min {
			min = n
		}
	}
	if min > 0 && min < int(^uint(0)>>1) {
		for i, l := range lines {
			if len(l) >= min {
				lines[i] = l[min:]
			}
		}
	}
	return strings.Join(lines, "\n")
}

// Merge 将片段写入原始 Begin/End，完整 Java 文件直接替换。
func Merge(framework, code string) string {
	if strings.TrimSpace(code) == "" {
		return framework
	}
	if full.MatchString(code) {
		return strings.TrimSpace(code)
	}
	b, e := Begin.FindStringIndex(framework), End.FindStringIndex(framework)
	if b == nil || e == nil || e[0] < b[1] {
		return strings.TrimSpace(code)
	}
	body := code
	if m := Begin.FindStringIndex(body); m != nil {
		body = body[m[1]:]
	}
	if m := End.FindStringIndex(body); m != nil {
		body = body[:m[0]]
	}
	body = dedent(body)
	lines := strings.Split(body, "\n")
	for i, l := range lines {
		if strings.TrimSpace(l) != "" {
			lines[i] = "    " + l
		}
	}
	return framework[:b[1]] + "\n" + strings.Join(lines, "\n") + "\n" + framework[e[0]:]
}

// Complete 集中回答完整度规则，AI 驱动与 solver 共用。
func Complete(answer string, n int) bool {
	if n > 1 {
		files := SplitMulti(answer)
		// 多文件题收到完整的单文件回答时，允许 solver 执行原有首文件降级。
		if len(files) == 0 {
			return Complete(answer, 1)
		}
		if len(files) < n {
			return false
		}
		for _, s := range files {
			if !strings.HasSuffix(strings.TrimSpace(s), "}") {
				return false
			}
		}
		return true
	}
	tail := strings.TrimSpace(answer)
	if strings.Contains(answer, "Begin") && !strings.Contains(answer, "End") {
		return false
	}
	if strings.HasSuffix(tail, "}") || strings.HasSuffix(tail, "```") {
		return true
	}
	i := strings.LastIndex(answer, "End")
	return i >= 0 && strings.Contains(answer[i:], "}")
}

// Repr 对齐 Python 字符串 repr，反馈中精确保留不可见字符。
func Repr(s string) string {
	quote := "'"
	if strings.Contains(s, "'") && !strings.Contains(s, "\"") {
		quote = "\""
	}
	var b strings.Builder
	b.WriteString(quote)
	for _, r := range s {
		switch r {
		case '\\':
			b.WriteString(`\\`)
		case '\n':
			b.WriteString(`\n`)
		case '\r':
			b.WriteString(`\r`)
		case '\t':
			b.WriteString(`\t`)
		default:
			if string(r) == quote {
				b.WriteByte('\\')
				b.WriteRune(r)
			} else if !unicode.IsPrint(r) {
				if r <= 255 {
					fmt.Fprintf(&b, `\x%02x`, r)
				} else {
					fmt.Fprintf(&b, `\u%04x`, r)
				}
			} else {
				b.WriteRune(r)
			}
		}
	}
	b.WriteString(quote)
	return b.String()
}

// Diff 按 Unicode 字符定位输出首个差异，不裁剪空白。
func Diff(expected, actual string) string {
	e, a := []rune(expected), []rune(actual)
	if expected == actual {
		return ""
	}
	for i := 0; i < len(e) && i < len(a); i++ {
		if e[i] != a[i] {
			lo, hi := max(0, i-8), min(len(e), i+8)
			return fmt.Sprintf("第 %d 个字符开始不同：预期 %s，实际 %s（预期片段 %s）", i, Repr(string(e[i])), Repr(string(a[i])), Repr(string(e[lo:hi])))
		}
	}
	if len(e) > len(a) {
		return "实际输出少了结尾：" + Repr(string(e[len(a):]))
	}
	return "实际输出多了结尾：" + Repr(string(a[len(e):]))
}

var answerRE = regexp.MustCompile(`^\s*(?:第)?\s*(\d+)\s*[.、:：)）]?\s*([A-Fa-f][A-Fa-f,\s]*)$`)
var bareRE = regexp.MustCompile(`^[A-F]{1,4}$`)

// ChoiceAnswers 解析带题号或独立字母行的答案，漏题保留空串。
func ChoiceAnswers(text string, n int) []string {
	out := make([]string, max(n, 0))
	pool := []string{}
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if m := answerRE.FindStringSubmatch(line); m != nil {
			no, _ := strconv.Atoi(m[1])
			if no > 0 && no <= n {
				set := map[rune]bool{}
				for _, r := range strings.ToUpper(m[2]) {
					if r >= 'A' && r <= 'F' {
						set[r] = true
					}
				}
				letters := []string{}
				for r := range set {
					letters = append(letters, string(r))
				}
				sort.Strings(letters)
				out[no-1] = strings.Join(letters, "")
			}
		}
		if bareRE.MatchString(line) {
			pool = append(pool, line)
		}
	}
	j := 0
	for i := range out {
		if out[i] == "" && j < len(pool) {
			out[i] = pool[j]
			j++
		}
	}
	return out
}

// Letters 将字母转换为平台要求的字符串索引。
func Letters(letters, typ string) string {
	out := []string{}
	for _, r := range strings.ToUpper(letters) {
		if r >= 'A' && r <= 'F' {
			out = append(out, strconv.Itoa(int(r-'A')))
		}
	}
	if len(out) == 0 {
		return ""
	}
	if typ == "multiple_choice" {
		return strings.Join(out, ",")
	}
	return out[0]
}

// Summarize 整理可见测试用例及逐字符反馈。
func Summarize(r core.Result) (bool, string) {
	lines := []string{}
	for _, t := range r.Trials {
		if t.Visible != nil && !*t.Visible {
			continue
		}
		flag := "✗"
		if t.Passed {
			flag = "✓"
		}
		lines = append(lines, fmt.Sprintf("%s 预期: %s\n  实际: %s", flag, Repr(t.Expected), Repr(t.Actual)))
		if !t.Passed {
			if hint := Diff(t.Expected, t.Actual); hint != "" {
				lines = append(lines, "  → "+hint)
			}
		}
	}
	if !r.Passed && len(lines) == 0 {
		msg := r.Message
		if msg == "" {
			msg = r.Phase
		}
		lines = append(lines, msg)
	}
	return r.Passed, strings.Join(lines, "\n")
}
