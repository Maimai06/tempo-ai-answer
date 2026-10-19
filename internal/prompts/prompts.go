// Package prompts 按原始题目生成中文提示词，不读写提交副本。
package prompts

import (
	"fmt"
	"html"
	"regexp"
	"strings"

	"github.com/Maimai06/tempo-ai-answer/internal/core"
)

var images = regexp.MustCompile(`!\[[^\]]*\]\([^)]*\)(?:\{\{[^}]*\}\})?`)
var breaks = regexp.MustCompile(`(?i)<br\s*/?>|</(?:p|div|pre)\s*>`)
var tags = regexp.MustCompile(`<[^>]+>`)
var spaces = regexp.MustCompile(`[ \t]+\n`)
var newlines = regexp.MustCompile(`\n{3,}`)

// HTMLText 去掉题干 HTML，并保留代码与段落换行。
func HTMLText(s string) string {
	s = breaks.ReplaceAllString(s, "\n")
	s = tags.ReplaceAllString(s, "")
	s = html.UnescapeString(s)
	s = spaces.ReplaceAllString(s, "\n")
	return strings.TrimSpace(newlines.ReplaceAllString(s, "\n\n"))
}

// TypeName 返回选择题类型的中文名称。
func TypeName(t string) string {
	switch t {
	case "single_choice":
		return "单选题"
	case "multiple_choice":
		return "多选题"
	case "judge":
		return "判断题"
	}
	return "选择题"
}

// Select 生成逐题选择题提示词及重试反馈。
func Select(items []core.Choice, feedback string) string {
	p := []string{"下面是一组实训选择题，请作答。", ""}
	for i, q := range items {
		p = append(p, fmt.Sprintf("【第%d题】（%s）", i+1, TypeName(q.Type)), HTMLText(q.Title))
		for j, opt := range q.Content {
			p = append(p, fmt.Sprintf("%c. %s", 'A'+j, HTMLText(opt)))
		}
		p = append(p, "")
	}
	p = append(p, "【作答要求】", "1. 严格按下面的格式输出，每题一行，不要解释、不要复述题目：", "   1. B", "   2. AC", "2. 多选题写多个字母（如 AC），单选题只写一个字母，判断题 A=正确、B=错误。", "3. 题号必须和上面的题号一致，每题都要作答，不能漏。")
	if feedback != "" {
		p = append(p, "", "【上次作答有错，请注意】", feedback)
	}
	return strings.Join(p, "\n")
}

// Code 只使用原始快照生成编程题提示词。
func Code(snapshot core.Snapshot, feedback string, seq int) string {
	s := snapshot.Step()
	head := "下面是一道新的实训编程题，请只针对这一题作答，忽略之前对话里的其它题目。"
	if seq > 0 {
		head = fmt.Sprintf("（本次连续对话的第 %d 题）", seq) + head
	}
	desc := []rune(strings.TrimSpace(images.ReplaceAllString(s.Description, "")))
	if len(desc) > 6000 {
		desc = desc[:6000]
	}
	p := []string{head, "", "【课程】" + s.Course, "【关卡】" + s.Title, "", "【题目说明】", string(desc), ""}
	for _, c := range s.Info.Codes {
		p = append(p, "【待补全代码："+c.Path+"】", "```java", c.Code, "```", "")
	}
	visible := []core.Trial{}
	for _, t := range s.Trials {
		if t.Expected != "" {
			visible = append(visible, t)
		}
	}
	if len(visible) > 0 {
		p = append(p, "【测试数据】")
		for i, t := range visible {
			input := t.Input
			if input == "" {
				input = "（无）"
			}
			p = append(p, fmt.Sprintf("  用例%d: 输入=%s | 预期输出=%s", i+1, input, t.Expected))
		}
		if strings.HasSuffix(visible[0].Expected, "\n") {
			p = append(p, "  注意：预期输出的**最后一行也有换行**，请所有行都用 System.out.println，不要用 System.out.print。")
		} else {
			p = append(p, "  注意：预期输出的**最后一行没有换行**，最后一行请用 System.out.print。")
		}
		p = append(p, "")
	}
	if len(s.Info.Codes) > 1 {
		p = append(p, fmt.Sprintf("【输出要求】本关有 %d 个文件，请分别输出每个文件的完整内容。", len(s.Info.Codes)), "格式严格如下（每个文件一段，路径必须与下面完全一致，不要用 ``` 包裹）：")
		for _, c := range s.Info.Codes {
			p = append(p, "===FILE: "+c.Path+"===", "（这里放该文件的完整代码）", "")
		}
		p = append(p, shared...)
		p = append(p, "4. 每个文件的 public class 名必须与文件名一致。")
	} else {
		p = append(p, "【输出要求】", "1. 直接输出完整的、可编译运行的 Java 代码，不要解释、不要客套话。")
		p = append(p, shared...)
		p = append(p, "4. 只用一个 ```java 代码块输出最终代码。")
	}
	if feedback != "" {
		p = append(p, "", "【上次提交未通过，请修正】", feedback)
	}
	return strings.Join(p, "\n")
}
