package scope

import (
	"reflect"
	"strings"
	"testing"

	"github.com/Maimai06/tempo-ai-answer/internal/core"
)

func TestParse(t *testing.T) {
	cases := []struct {
		text string
		want Scope
	}{{"", Scope{}}, {"第三章", Scope{Chapter: 3}}, {"第十章", Scope{Chapter: 10}}, {"第十一章", Scope{Chapter: 11}}, {"第二十章", Scope{Chapter: 20}}, {"第二十一章", Scope{Chapter: 21}}, {"第九十九章", Scope{Chapter: 99}}, {"第1章 1-5", Scope{Chapter: 1, From: 1, To: 5}}, {"1-3", Scope{Chapter: 1, From: 3, To: 3}}, {"第2章 2", Scope{Chapter: 2, From: 2, To: 2}}, {"8", Scope{Chapter: 8}}, {"1~3", Scope{From: 1, To: 3}}, {"Java", Scope{Keyword: "Java"}}}
	for _, c := range cases {
		s, e := Parse(c.text)
		if e != nil || s != c.want || s.String() == "" {
			t.Fatal(c.text, s, e)
		}
	}
	for _, s := range []string{"第x章", "1-x", "第2章 3-1", "第1章 abc", "0-1", "1-0", "0", "第一一章", "第一十十章"} {
		if _, e := Parse(s); e == nil {
			t.Fatal(s)
		}
	}
	for _, s := range []Scope{{From: 2, To: 2, Chapter: 1}, {From: 2, To: 3}, {Keyword: "Java"}, {}} {
		if s.String() == "" {
			t.Fatal(s)
		}
	}
}
func TestRangeAndMatch(t *testing.T) {
	v, e := ParseRange("1-3, 5,3,,")
	if e != nil || !reflect.DeepEqual(v, []int{1, 2, 3, 5}) {
		t.Fatal(v, e)
	}
	for _, s := range []string{"3-1", "1-x", "-1", "10000", "0-2"} {
		if _, e := ParseRange(s); e == nil {
			t.Fatal(s)
		}
	}
	c := core.Course{Name: "Java", Chapter: "基础", ChapterNo: 2, CourseNo: 3}
	for _, s := range []Scope{{}, {Keyword: "Java"}, {Keyword: "基础"}, {Chapter: 2, From: 2, To: 4}} {
		if !s.Match(c) {
			t.Fatal(s)
		}
	}
	for _, s := range []Scope{{Keyword: "不存在"}, {Chapter: 1}, {From: 4, To: 5}} {
		if s.Match(c) {
			t.Fatal(s)
		}
	}
	if !strings.Contains((Scope{From: 1, To: 2}).String(), "所有章") {
		t.Fatal("description")
	}
}
