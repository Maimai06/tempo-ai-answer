// Package scope 解析章节、实验和关卡范围，不依赖平台服务。
package scope

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/Maimai06/tempo-ai-answer/internal/core"
)

var chapterRE = regexp.MustCompile(`^(?:第)?\s*([0-9一二三四五六七八九十]+)\s*章\s*(.*)$`)
var pairRE = regexp.MustCompile(`^(\d+)\s*[-－]\s*(\d+)$`)
var intervalRE = regexp.MustCompile(`^(\d+)\s*[-－~～到至]\s*(\d+)$`)

// Scope 中的 0 表示未限制；解析后数字保证为正数。
type Scope struct {
	Chapter, From, To int
	Keyword           string
}

func number(s string) (int, error) {
	n, e := strconv.Atoi(s)
	if e == nil && n >= 1 && n <= 9999 {
		return n, nil
	}
	return 0, fmt.Errorf("无效范围数字：%s", s)
}
func chinese(s string) (int, error) {
	if n, e := number(s); e == nil {
		return n, nil
	}
	digits := map[rune]int{'一': 1, '二': 2, '三': 3, '四': 4, '五': 5, '六': 6, '七': 7, '八': 8, '九': 9}
	r := []rune(s)
	if len(r) == 1 {
		if r[0] == '十' {
			return 10, nil
		}
		if n := digits[r[0]]; n > 0 {
			return n, nil
		}
	}
	p := strings.Split(s, "十")
	if len(p) == 2 {
		t, u := 1, 0
		if p[0] != "" {
			rr := []rune(p[0])
			if len(rr) != 1 || digits[rr[0]] == 0 {
				return 0, fmt.Errorf("无效中文章号")
			}
			t = digits[rr[0]]
		}
		if p[1] != "" {
			rr := []rune(p[1])
			if len(rr) != 1 || digits[rr[0]] == 0 {
				return 0, fmt.Errorf("无效中文章号")
			}
			u = digits[rr[0]]
		}
		return t*10 + u, nil
	}
	return 0, fmt.Errorf("无效章号：%s", s)
}
func bounds(a, b string) (int, int, error) {
	x, e := number(a)
	if e != nil {
		return 0, 0, e
	}
	y, e := number(b)
	if e != nil || y < x {
		return 0, 0, fmt.Errorf("范围必须从小到大：%s-%s", a, b)
	}
	return x, y, nil
}

// ParseRange 支持 1-5,8，拒绝负数、倒序和过大范围。
func ParseRange(text string) ([]int, error) {
	out := []int{}
	seen := map[int]bool{}
	for _, p := range strings.Split(text, ",") {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		a, b := 0, 0
		var e error
		if m := intervalRE.FindStringSubmatch(p); m != nil {
			a, b, e = bounds(m[1], m[2])
		} else {
			a, e = number(p)
			b = a
		}
		if e != nil {
			return nil, e
		}
		for n := a; n <= b; n++ {
			if !seen[n] {
				seen[n] = true
				out = append(out, n)
			}
		}
	}
	return out, nil
}

// Parse 保持 v1 的实验编号含义，1-3 表示第一章第三个实验。
func Parse(text string) (Scope, error) {
	text = strings.TrimSpace(text)
	s := Scope{}
	if text == "" {
		return s, nil
	}
	rest := text
	if m := chapterRE.FindStringSubmatch(text); m != nil {
		n, e := chinese(m[1])
		if e != nil {
			return s, e
		}
		s.Chapter = n
		rest = strings.TrimSpace(m[2])
		if rest == "" {
			return s, nil
		}
	} else if m := pairRE.FindStringSubmatch(text); m != nil {
		a, e := number(m[1])
		if e != nil {
			return s, e
		}
		b, e := number(m[2])
		return Scope{Chapter: a, From: b, To: b}, e
	}
	if m := intervalRE.FindStringSubmatch(rest); m != nil {
		a, b, e := bounds(m[1], m[2])
		s.From = a
		s.To = b
		return s, e
	}
	if n, e := number(rest); e == nil {
		if s.Chapter > 0 {
			s.From = n
			s.To = n
		} else {
			s.Chapter = n
		}
		return s, nil
	}
	if s.Chapter > 0 || strings.HasPrefix(text, "第") || strings.ContainsAny(text, "0123456789") && (strings.ContainsAny(text, "-－") || regexp.MustCompile(`^\d+$`).MatchString(text)) {
		return Scope{}, fmt.Errorf("范围无效，请使用 第1章 1-5 或 1-3")
	}
	return Scope{Keyword: text}, nil
}
func (s Scope) Match(c core.Course) bool {
	if s.Keyword != "" {
		return strings.Contains(c.Label(), s.Keyword) || strings.Contains(c.Chapter, s.Keyword)
	}
	return (s.Chapter == 0 || s.Chapter == c.ChapterNo) && (s.From == 0 || c.CourseNo >= s.From && c.CourseNo <= s.To)
}
func (s Scope) String() string {
	if s.Keyword != "" {
		return "名称包含「" + s.Keyword + "」"
	}
	if s.Chapter > 0 {
		if s.From > 0 {
			if s.From == s.To {
				return fmt.Sprintf("第%d章 第%d个实验", s.Chapter, s.From)
			}
			return fmt.Sprintf("第%d章 第%d~%d个实验", s.Chapter, s.From, s.To)
		}
		return fmt.Sprintf("第%d章 全部实验", s.Chapter)
	}
	if s.From > 0 {
		return fmt.Sprintf("第%d~%d个实验（所有章）", s.From, s.To)
	}
	return "全部题目"
}
