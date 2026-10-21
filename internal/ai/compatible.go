package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/Maimai06/tempo-ai-answer/internal/core"
)

const maxAPIResponseSize = 8 << 20

// Compatible 调用兼容 OpenAI Chat Completions 的 API，不在请求间保留题目对话。
type Compatible struct {
	endpoint string
	model    string
	apiKey   string
	client   *http.Client
	sink     core.Sink
	perChat  int
	count    int
	latest   string
}

// NewCompatible 创建兼容接口 provider。API 密钥只保存在内存中。
func NewCompatible(baseURL, model, apiKey string, timeout time.Duration, perChat int, sink core.Sink) (*Compatible, error) {
	u, err := url.Parse(strings.TrimSpace(baseURL))
	if err != nil || u.Host == "" || (u.Scheme != "https" && u.Scheme != "http") || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return nil, errors.New("AI API 地址无效，请检查 ai.base_url")
	}
	if u.Scheme == "http" && u.Hostname() != "localhost" && u.Hostname() != "127.0.0.1" && u.Hostname() != "::1" {
		return nil, errors.New("AI API 使用 HTTP 时仅允许本机地址；远程服务请使用 HTTPS")
	}
	model = strings.TrimSpace(model)
	if model == "" {
		return nil, errors.New("AI API 模型不能为空，请检查 ai.model")
	}
	if strings.TrimSpace(apiKey) == "" {
		return nil, errors.New("AI API 密钥未配置")
	}
	if timeout <= 0 {
		return nil, errors.New("AI API 超时必须大于 0")
	}
	path := strings.TrimRight(u.Path, "/")
	if !strings.HasSuffix(path, "/chat/completions") {
		path += "/chat/completions"
	}
	u.Path = path
	u.RawPath = ""
	return &Compatible{
		endpoint: u.String(),
		model:    model,
		apiKey:   strings.TrimSpace(apiKey),
		client:   &http.Client{Timeout: timeout},
		sink:     sink,
		perChat:  max(1, perChat),
	}, nil
}

// BeginQuestion 更新面板显示的题目轮次；API 请求彼此独立，不保留对话上下文。
func (p *Compatible) BeginQuestion(ctx context.Context) (int, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	p.count = p.count%p.perChat + 1
	return p.count, nil
}

// ReadLatest 返回最近一次完整响应，供编程题完整性检查使用。
func (p *Compatible) ReadLatest(ctx context.Context) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	return p.latest, nil
}

// ResponsesAreFinal 表示兼容 API 以非流式方式返回完整响应。
func (*Compatible) ResponsesAreFinal() bool { return true }

// Ask 发送独立题目请求并返回第一条完成结果。
func (p *Compatible) Ask(ctx context.Context, prompt string, _ int) (string, error) {
	body, err := json.Marshal(struct {
		Model    string `json:"model"`
		Messages []struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"messages"`
		Stream bool `json:"stream"`
	}{Model: p.model, Messages: []struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	}{{Role: "user", Content: prompt}}, Stream: false})
	if err != nil {
		return "", fmt.Errorf("编码 AI API 请求失败: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.endpoint, bytes.NewReader(body))
	if err != nil {
		return "", errors.New("创建 AI API 请求失败")
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+p.apiKey)
	resp, err := p.client.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		return "", fmt.Errorf("AI API 请求失败: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("AI API 返回 HTTP %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxAPIResponseSize+1))
	if err != nil {
		return "", fmt.Errorf("读取 AI API 响应失败: %w", err)
	}
	if len(data) > maxAPIResponseSize {
		return "", errors.New("AI API 响应超过 8 MiB 限制")
	}
	var result struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(data, &result); err != nil {
		return "", fmt.Errorf("AI API 响应格式无效: %w", err)
	}
	if len(result.Choices) == 0 || strings.TrimSpace(result.Choices[0].Message.Content) == "" {
		return "", errors.New("AI API 未返回答案内容")
	}
	p.latest = strings.TrimSpace(result.Choices[0].Message.Content)
	p.sink.Emit(core.Event{Kind: "answer", Text: p.latest})
	return p.latest, nil
}
