// Package config 负责默认值、升级兼容、中文校验与独立用户目录。
package config

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

// Template 内嵌不含真实凭据的占位配置。
//
//go:embed template.json
var Template []byte

// Config 对齐 v1 配置；先写默认值再解码，显式 false/0 保持原值。
type Config struct {
	Platform struct {
		BaseURL     string `json:"base_url"`
		Username    string `json:"username"`
		Password    string `json:"password"`
		ClassroomID string `json:"classroom_id"`
	} `json:"platform"`
	AI struct {
		AnswerTimeout int `json:"answer_timeout"`
	} `json:"ai"`
	Run struct {
		MaxRetry    int  `json:"max_retry"`
		Delay       int  `json:"delay_between_steps"`
		SkipPassed  bool `json:"skip_passed"`
		CleanupChat bool `json:"cleanup_chat"`
		PerChat     int  `json:"questions_per_chat"`
	} `json:"run"`
}

// Defaults 返回兼容 v1 的缺省值。
func Defaults() Config {
	var c Config
	c.AI.AnswerTimeout = 180
	c.Run.MaxRetry = 2
	c.Run.Delay = 3
	c.Run.SkipPassed = true
	c.Run.CleanupChat = true
	c.Run.PerChat = 3
	return c
}
func hasNull(v any) bool {
	if v == nil {
		return true
	}
	switch x := v.(type) {
	case map[string]any:
		for _, v := range x {
			if hasNull(v) {
				return true
			}
		}
	case []any:
		for _, v := range x {
			if hasNull(v) {
				return true
			}
		}
	}
	return false
}

// Decode 补齐旧配置缺省项，拒绝 null、字段类型错误与多份 JSON。
func Decode(b []byte) (Config, error) {
	c := Defaults()
	var raw map[string]any
	if err := json.Unmarshal(b, &raw); err != nil {
		return c, fmt.Errorf("配置 JSON 无效: %w", err)
	}
	if hasNull(raw) {
		return c, errors.New("配置字段不能为 null")
	}
	d := json.NewDecoder(bytes.NewReader(b))
	if err := d.Decode(&c); err != nil {
		return c, fmt.Errorf("配置字段类型错误: %w", err)
	}
	return c, c.Validate()
}

// Validate 启动前校验，不在错误中回显凭据。
func (c Config) Validate() error {
	u, e := url.Parse(c.Platform.BaseURL)
	if e != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") || strings.Contains(strings.ToUpper(c.Platform.BaseURL), "YOUR_") || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return errors.New("platform.base_url 请填写真实 HTTP(S) 平台地址")
	}
	if c.Platform.Username == "" || strings.HasPrefix(c.Platform.Username, "YOUR_") {
		return errors.New("请填写 platform.username")
	}
	if c.Platform.Password == "" || strings.HasPrefix(c.Platform.Password, "YOUR_") {
		return errors.New("请填写 platform.password")
	}
	if c.AI.AnswerTimeout < 1 || c.AI.AnswerTimeout > 3600 {
		return errors.New("answer_timeout 范围为 1..3600 秒")
	}
	if c.Run.MaxRetry < 0 || c.Run.MaxRetry > 20 || c.Run.Delay < 0 || c.Run.Delay > 3600 || c.Run.PerChat < 1 || c.Run.PerChat > 100 {
		return errors.New("run 参数范围错误：max_retry 0..20，delay_between_steps 0..3600，questions_per_chat 1..100")
	}
	return nil
}

// ErrCreated 表示已创建占位配置，用户填写后可再次运行。
var ErrCreated = errors.New("已创建配置，请填写平台地址与账号后再次运行")

// Load 读取配置；文件不存在时排他创建占位模板。
func Load(path string) (Config, error) {
	b, e := os.ReadFile(path)
	if errors.Is(e, os.ErrNotExist) {
		if e = os.MkdirAll(filepath.Dir(path), 0700); e != nil {
			return Config{}, e
		}
		f, e := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if e != nil {
			return Config{}, e
		}
		_, e = f.Write(Template)
		ce := f.Close()
		if e != nil {
			return Config{}, e
		}
		if ce != nil {
			return Config{}, ce
		}
		return Config{}, ErrCreated
	}
	if e != nil {
		return Config{}, e
	}
	return Decode(b)
}

// DataDir 使用系统用户配置目录，迁移和升级不覆盖 profile 或日志。
func DataDir() (string, error) {
	d, e := os.UserConfigDir()
	if e != nil {
		return "", e
	}
	return filepath.Join(d, "tempo-ai-answer"), nil
}

// CopyTemplate 仅输出不含真实凭据的嵌入模板。
func CopyTemplate(w io.Writer) error { _, e := w.Write(Template); return e }
