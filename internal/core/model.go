// Package core 定义不依赖浏览器和 UI 的领域数据与事件。
package core

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"time"
)

// ID 接受平台字符串或数字标识，后续 URL 参数统一为字符串。
type ID string

// UnmarshalJSON 接受字符串或整数 ID，拒绝 null 与复合值。
func (i *ID) UnmarshalJSON(b []byte) error {
	var s string
	if len(b) > 0 && b[0] == '"' {
		if e := json.Unmarshal(b, &s); e != nil {
			return e
		}
	} else {
		var n json.Number
		if e := json.Unmarshal(b, &n); e != nil {
			return e
		}
		if _, e := strconv.ParseInt(n.String(), 10, 64); e != nil {
			return e
		}
		s = n.String()
	}
	*i = ID(s)
	return nil
}

// Code 是平台保存接口所需的代码文件。
type Code struct {
	Path string `json:"codePath"`
	Code string `json:"code"`
	Name string `json:"name"`
}

// Choice 保留 questionId 原始 JSON 类型，避免把数字 ID 错写为字符串。
type Choice struct {
	ID      json.RawMessage `json:"questionId"`
	Type    string          `json:"type"`
	Title   string          `json:"title"`
	Content []string        `json:"content"`
	Answer  string          `json:"answer"`
	Passed  bool            `json:"passed"`
}

// Trial 保留输出空格与换行，Visible 对应平台原有拼写。
type Trial struct {
	ID       json.RawMessage `json:"questionId,omitempty"`
	Expected string          `json:"expectedOutput"`
	Actual   string          `json:"realOutput"`
	Input    string          `json:"input"`
	Passed   bool            `json:"passed"`
	Visible  *bool           `json:"isTrailSetVisible,omitempty"`
}

// QuestionInfo 保存代码文件和选择题内容。
type QuestionInfo struct {
	Codes   []Code   `json:"questionCodeList"`
	Choices []Choice `json:"selectOrJudgeList"`
}

// Step 为单关完整原始题目。
type Step struct {
	ID          ID           `json:"courseStepId"`
	Title       string       `json:"title"`
	Course      string       `json:"courseName"`
	Description string       `json:"description"`
	Type        string       `json:"questionType"`
	TypeName    string       `json:"questionTypeName"`
	Info        QuestionInfo `json:"questionInfo"`
	Trials      []Trial      `json:"trialSetList"`
}

// Snapshot 隐藏原始数据，所有读出均深拷贝，杜绝重试污染。
type Snapshot struct{ step Step }

func clone(s Step) Step {
	s.Info.Codes = append([]Code(nil), s.Info.Codes...)
	s.Info.Choices = append([]Choice(nil), s.Info.Choices...)
	for i := range s.Info.Choices {
		q := &s.Info.Choices[i]
		q.ID = append(json.RawMessage(nil), q.ID...)
		q.Content = append([]string(nil), q.Content...)
	}
	s.Trials = append([]Trial(nil), s.Trials...)
	for i := range s.Trials {
		t := &s.Trials[i]
		t.ID = append(json.RawMessage(nil), t.ID...)
		if t.Visible != nil {
			v := *t.Visible
			t.Visible = &v
		}
	}
	return s
}

// NewSnapshot 深拷贝原始题目，隔离调用方引用。
func NewSnapshot(s Step) Snapshot { return Snapshot{clone(s)} }

// Step 返回原题的独立深拷贝。
func (s Snapshot) Step() Step { return clone(s.step) }

// Result 保存评测状态与测试输出。
type Result struct {
	Passed  bool    `json:"passFlag"`
	Trials  []Trial `json:"trialSetList"`
	Message string  `json:"message"`
	Phase   string  `json:"containerPhase"`
}

// Classroom 保存课堂列表中的标识与名称。
type Classroom struct {
	ID    ID     `json:"id"`
	Name  string `json:"classroomName"`
	Class string `json:"belongToClass"`
}

// Course 同时容纳章节与实验节点，拍平后补充编号。
type Course struct {
	ID           ID       `json:"id"`
	CoursesID    ID       `json:"coursesId"`
	CourseID     ID       `json:"courseId"`
	Name         string   `json:"coursesName"`
	FallbackName string   `json:"name"`
	Type         string   `json:"coursesType"`
	Sort         int      `json:"sort"`
	Children     []Course `json:"lastList"`
	Chapter      string   `json:"-"`
	ChapterNo    int      `json:"-"`
	CourseNo     int      `json:"-"`
}

// UnmarshalJSON 将章节和实验的可选 ID 空值视为未提供，其余 ID 类型仍严格校验。
func (c *Course) UnmarshalJSON(b []byte) error {
	type courseFields Course
	var value struct {
		courseFields
		ID        *ID `json:"id"`
		CoursesID *ID `json:"coursesId"`
		CourseID  *ID `json:"courseId"`
	}
	if err := json.Unmarshal(b, &value); err != nil {
		return err
	}
	next := Course(value.courseFields)
	if value.ID != nil {
		next.ID = *value.ID
	}
	if value.CoursesID != nil {
		next.CoursesID = *value.CoursesID
	}
	if value.CourseID != nil {
		next.CourseID = *value.CourseID
	}
	*c = next
	return nil
}

// Identifier 按平台兼容字段选取实验 ID。
func (c Course) Identifier() string {
	for _, v := range []ID{c.CourseID, c.CoursesID, c.ID} {
		if v != "" {
			return string(v)
		}
	}
	return ""
}

// Label 兼容课程名称字段。
func (c Course) Label() string {
	if c.Name != "" {
		return c.Name
	}
	return c.FallbackName
}

// Number 返回用于清单展示的章号与实验号。
func (c Course) Number() string { return fmt.Sprintf("%d-%d", c.ChapterNo, c.CourseNo) }

// StepRef 保存关卡清单和完成标记。
type StepRef struct {
	ID       ID     `json:"stepId"`
	Name     string `json:"name"`
	Sequence int    `json:"sequence"`
	Finished bool   `json:"finished"`
}

// User 仅保留需要展示的用户信息，不包含认证字段。
type User struct {
	Name string `json:"name"`
	Code string `json:"code"`
	Org  string `json:"orgName"`
}

// Platform 供答题引擎使用，可用 fixture 替换真实服务。
type Platform interface {
	GetStep(context.Context, string) (Step, error)
	Evaluate(context.Context, string, []Code) (Result, error)
	EvaluateSelect(context.Context, string, []Choice) (Result, error)
}

// AIProvider 隔离网页版实现与 Demo；所有长操作均响应取消。
type AIProvider interface {
	BeginQuestion(context.Context) (int, error)
	Ask(context.Context, string, int) (string, error)
	ReadLatest(context.Context) (string, error)
}

// Event 使用封闭的类型化字段，通过 Kind 区分状态变化。
type Event struct {
	Kind                                    string
	Text                                    string
	Step                                    *Step
	Attempt, PromptLen, Round, Index, Total int
	Passed                                  *bool
	At                                      time.Time
}

// Sink 同步接收领域事件，调用者负责订阅者的并发保护。
type Sink func(Event)

// Emit 补充事件时间；空订阅者可安全省略。
func (s Sink) Emit(e Event) {
	if s != nil {
		e.At = time.Now()
		s(e)
	}
}

// Wait 是可取消的轮询间隔，不以裸 sleep 阻塞退出。
func Wait(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}
