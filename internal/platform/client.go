// Package platform 只在浏览器页面上下文发平台 fetch，保留 Cookie 与网关头。
package platform

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/Maimai06/tempo-ai-answer/internal/core"
)

const gateway = "/gateway/tempo-talents/api"

// Page 是 CDP 页面的最小接口，允许离线 fixture 验证协议。
type Page interface {
	Eval(context.Context, string, any) error
	Navigate(context.Context, string) error
}

// Client 持有页面会话、账号与运行期课堂，不使用全局配置。
type Client struct {
	Page                                     Page
	BaseURL, Username, Password, ClassroomID string
	Log                                      func(string)
}
type response struct {
	Status int       `json:"status"`
	JSON   *envelope `json:"json"`
	URL    string    `json:"url"`
	HTML   bool      `json:"html"`
}
type envelope struct {
	Success bool            `json:"success"`
	Data    json.RawMessage `json:"data"`
	Message string          `json:"message"`
}

func (c *Client) fetch(ctx context.Context, path, method string, data url.Values) (response, error) {
	u, e := url.Parse(c.BaseURL)
	if e != nil {
		return response{}, e
	}
	prefix := strings.TrimRight(u.Path, "/")
	if i := strings.LastIndex(prefix, "/tempo-talents"); i >= 0 {
		prefix = prefix[:i]
	}
	u.Path = prefix + path
	u.RawPath = ""
	if method == "GET" {
		u.RawQuery = data.Encode()
	}
	args := map[string]any{"url": u.String(), "method": method, "body": data.Encode()}
	b, e := json.Marshal(args)
	if e != nil {
		return response{}, e
	}
	js := `(async(a)=>{const opt={method:a.method,credentials:'include',headers:{'X-Requested-With':'XMLHttpRequest'}};if(a.method!=='GET'){opt.headers['Content-Type']='application/x-www-form-urlencoded; charset=UTF-8';opt.body=a.body}const r=await fetch(a.url,opt);const text=await r.text();let json=null;try{json=JSON.parse(text)}catch{}return {status:r.status,json,url:r.url,html:/^\s*<(?:!doctype|html)/i.test(text)}})(` + string(b) + `)`
	var r response
	e = c.Page.Eval(ctx, js, &r)
	return r, e
}
func (c *Client) current(ctx context.Context) (core.User, bool) {
	r, e := c.fetch(ctx, gateway+"/tt-common/user/current", "GET", nil)
	if e != nil || r.Status != 200 || r.JSON == nil || !r.JSON.Success || string(r.JSON.Data) == "null" {
		return core.User{}, false
	}
	var u core.User
	if e = json.Unmarshal(r.JSON.Data, &u); e != nil || u.Name == "" {
		return core.User{}, false
	}
	return u, true
}

// Login 复用持久 profile，验证码交由可见浏览器中的用户完成。
func (c *Client) Login(ctx context.Context) (core.User, error) {
	ctx, cancel := context.WithTimeout(ctx, 150*time.Second)
	defer cancel()
	if e := c.Page.Navigate(ctx, c.BaseURL); e != nil {
		return core.User{}, fmt.Errorf("打开平台失败: %w", e)
	}
	if e := core.Wait(ctx, 2500*time.Millisecond); e != nil {
		return core.User{}, e
	}
	if u, ok := c.current(ctx); ok {
		return u, nil
	}
	credentials, _ := json.Marshal([]string{c.Username, c.Password})
	js := `(a=>{const inputs=[...document.querySelectorAll('input.el-input__inner')];if(inputs.length<2)return false;for(let i=0;i<2;i++){const set=Object.getOwnPropertyDescriptor(HTMLInputElement.prototype,'value').set;set.call(inputs[i],a[i]);inputs[i].dispatchEvent(new Event('input',{bubbles:true}));inputs[i].dispatchEvent(new Event('change',{bubbles:true}))}document.querySelector('button.login-button')?.click();return true})(` + string(credentials) + `)`
	var filled bool
	if e := c.Page.Eval(ctx, js, &filled); e != nil || !filled {
		return core.User{}, errors.New("找不到登录表单，请检查平台页面或先手工登录")
	}
	if c.Log != nil {
		c.Log("正在登录；若有验证码，请在浏览器中完成")
	}
	for {
		if e := core.Wait(ctx, time.Second); e != nil {
			return core.User{}, errors.New("平台登录超时或已取消")
		}
		if u, ok := c.current(ctx); ok {
			return u, nil
		}
	}
}

// API 检查网关响应，最多重登一次；不在错误中回显响应与凭据。
func (c *Client) API(ctx context.Context, path, method string, data url.Values, out any) error {
	relogged := false
	for attempt := 0; attempt < 3; attempt++ {
		r, e := c.fetch(ctx, gateway+path, method, data)
		if e != nil {
			if method != "GET" || attempt == 2 {
				return fmt.Errorf("平台请求失败 %s: %w", path, e)
			}
			if e = core.Wait(ctx, time.Second); e != nil {
				return e
			}
			continue
		}
		login := r.Status == 401 || r.Status == 302 || r.HTML && (strings.Contains(strings.ToLower(r.URL), "login") || r.URL != "" && !strings.Contains(r.URL, gateway))
		if login && !relogged {
			relogged = true
			if _, e = c.Login(ctx); e != nil {
				return e
			}
			continue
		}
		if r.Status != 200 || r.JSON == nil || !r.JSON.Success {
			return fmt.Errorf("平台接口 %s 未成功（HTTP %d），请检查会话或实践环境", path, r.Status)
		}
		if out == nil {
			return nil
		}
		if e = json.Unmarshal(r.JSON.Data, out); e != nil {
			return fmt.Errorf("平台接口 %s 数据格式错误: %w", path, e)
		}
		return nil
	}
	return fmt.Errorf("平台接口 %s 重试次数耗尽", path)
}
func vals(pairs ...string) url.Values {
	v := url.Values{}
	for i := 0; i+1 < len(pairs); i += 2 {
		v.Set(pairs[i], pairs[i+1])
	}
	return v
}

// ListClassrooms 读取用户已加入的进行中课堂。
func (c *Client) ListClassrooms(ctx context.Context) ([]core.Classroom, error) {
	var data struct {
		Info struct {
			Rows []core.Classroom `json:"rows"`
		} `json:"classroomListInfo"`
	}
	e := c.API(ctx, "/tt-classroom/list", "POST", vals("classroomStatus", "started", "page", "1", "rows", "50", "schoolYear", ""), &data)
	return data.Info.Rows, e
}

// EnsureClassroom 以 Client 中的课堂 ID 为运行期唯一来源。
func (c *Client) EnsureClassroom(ctx context.Context) error {
	if c.ClassroomID != "" && !strings.HasPrefix(c.ClassroomID, "YOUR_") {
		return nil
	}
	cs, e := c.ListClassrooms(ctx)
	if e != nil {
		return e
	}
	if len(cs) == 0 {
		return errors.New("没有找到已加入的课堂")
	}
	c.ClassroomID = string(cs[0].ID)
	return nil
}

// ListCourses 拍平章节与实验，补齐所属章节及稳定编号。
func (c *Client) ListCourses(ctx context.Context, kind string) ([]core.Course, error) {
	var chapters []core.Course
	e := c.API(ctx, "/tt-classroom/classroomCourseListDetail", "POST", vals("classroomId", c.ClassroomID, "coursesName", "", "coursesState", "allStudent", "coursesType", kind), &chapters)
	if e != nil {
		return nil, e
	}
	out := []core.Course{}
	for i, ch := range chapters {
		no := ch.Sort
		if no == 0 {
			no = i + 1
		}
		for _, v := range ch.Children {
			v.Chapter = ch.Label()
			v.ChapterNo = no
			v.CourseNo = v.Sort
			v.CourseID = core.ID(v.Identifier())
			out = append(out, v)
		}
	}
	if len(out) == 0 {
		for _, v := range chapters {
			if v.Type != "chapter" {
				out = append(out, v)
			}
		}
	}
	for i, course := range out {
		if course.Identifier() == "" {
			return nil, fmt.Errorf("课程列表第 %d 个实验缺少实验 ID，请检查平台课程数据", i+1)
		}
	}
	return out, nil
}

// ListSteps 读取某实验的关卡和完成状态。
func (c *Client) ListSteps(ctx context.Context, id string) ([]core.StepRef, error) {
	var data struct {
		Rows []core.StepRef `json:"rows"`
	}
	e := c.API(ctx, "/tt-course-challenge/challenge/list", "GET", vals("courseId", id, "classroomId", c.ClassroomID, "page", "1", "rows", "1000"), &data)
	return data.Rows, e
}

// Prepare 必要时初始化实践环境，并在限定时间内等待就绪。
func (c *Client) Prepare(ctx context.Context, id string) ([]core.StepRef, error) {
	if s, e := c.ListSteps(ctx, id); e == nil {
		return s, nil
	}
	if e := c.API(ctx, "/tt-course-challenge/environment/initialize", "POST", vals("courseId", id, "classroomId", c.ClassroomID), nil); e != nil {
		return nil, e
	}
	ctx, cancel := context.WithTimeout(ctx, 150*time.Second)
	defer cancel()
	for {
		if e := core.Wait(ctx, 3*time.Second); e != nil {
			return nil, fmt.Errorf("实践环境启动超时或取消: %w", e)
		}
		if s, e := c.ListSteps(ctx, id); e == nil && len(s) > 0 {
			return s, nil
		}
	}
}

// GetStep 读取原始题目详情。
func (c *Client) GetStep(ctx context.Context, id string) (core.Step, error) {
	var s core.Step
	e := c.API(ctx, "/tt-course-challenge/challenge", "GET", vals("courseStepId", id, "classroomId", c.ClassroomID), &s)
	if s.ID == "" {
		s.ID = core.ID(id)
	}
	return s, e
}
func (c *Client) save(ctx context.Context, id, key string, payload any) error {
	b, e := json.Marshal(payload)
	if e != nil {
		return e
	}
	return c.API(ctx, "/tt-course-challenge/challenge/save", "POST", vals("courseStepId", id, "classroomId", c.ClassroomID, "newElapsedTime", "5", key, string(b)), nil)
}

// Evaluate 保存代码、等待同步、触发并轮询评测。
func (c *Client) Evaluate(ctx context.Context, id string, codes []core.Code) (core.Result, error) {
	if e := c.save(ctx, id, "codeJson", codes); e != nil {
		return core.Result{}, e
	}
	if e := core.Wait(ctx, 1500*time.Millisecond); e != nil {
		return core.Result{}, e
	}
	var ticket string
	if e := c.API(ctx, "/tt-course-challenge/challenge/code/verification/start", "POST", vals("courseStepId", id, "classroomId", c.ClassroomID), &ticket); e != nil {
		return core.Result{}, e
	}
	if ticket == "" {
		return core.Result{}, errors.New("评测未返回 ticket")
	}
	ctx, cancel := context.WithTimeout(ctx, 180*time.Second)
	defer cancel()
	for {
		var r core.Result
		e := c.API(ctx, "/tt-course-challenge/challenge/code/verification", "GET", vals("courseStepId", id, "classroomId", c.ClassroomID, "ticket", ticket), &r)
		if e != nil {
			return r, e
		}
		switch strings.ToLower(r.Phase) {
		case "succeeded", "failed", "error":
			return r, nil
		}
		if len(r.Trials) > 0 {
			return r, nil
		}
		if e = core.Wait(ctx, 2*time.Second); e != nil {
			return r, fmt.Errorf("评测未完成: %w", e)
		}
	}
}

// EvaluateSelect 保存选择题答案并读取同步评测结果。
func (c *Client) EvaluateSelect(ctx context.Context, id string, choices []core.Choice) (core.Result, error) {
	if e := c.save(ctx, id, "selectOrJudgeJson", choices); e != nil {
		return core.Result{}, e
	}
	if e := core.Wait(ctx, time.Second); e != nil {
		return core.Result{}, e
	}
	var r core.Result
	e := c.API(ctx, "/tt-course-challenge/challenge/select-and-judge/verification", "POST", vals("classroomId", c.ClassroomID, "courseStepId", id), &r)
	return r, e
}
