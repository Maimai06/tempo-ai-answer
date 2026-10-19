// Package logger 提供控制台和文件日志，集中遮盖配置中的敏感值。
package logger

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/Maimai06/tempo-ai-answer/internal/core"
)

var sensitive = regexp.MustCompile(`(?i)(token|password|authorization|cookie)([=: ]+)[^\s,;]+`)

// Logger 同时写控制台与私有日志文件，保留集中脱敏规则。
type Logger struct {
	Log     *slog.Logger
	file    *os.File
	secrets []string
}

// Open 创建仅当前用户可读写的日志文件。
func Open(dir string, secrets ...string) (*Logger, error) {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	f, e := os.OpenFile(filepath.Join(dir, "tempo.log"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if e != nil {
		return nil, e
	}
	l := &Logger{file: f, secrets: secrets}
	l.Log = slog.New(slog.NewTextHandler(io.MultiWriter(os.Stderr, f), &slog.HandlerOptions{ReplaceAttr: func(_ []string, a slog.Attr) slog.Attr {
		if a.Value.Kind() == slog.KindString {
			a.Value = slog.StringValue(l.Redact(a.Value.String()))
		}
		return a
	}}))
	return l, nil
}

// Redact 遮盖已知敏感值和常见认证键值。
func (l *Logger) Redact(s string) string {
	for _, secret := range l.secrets {
		if secret != "" {
			s = strings.ReplaceAll(s, secret, "[已隐藏]")
		}
	}
	return sensitive.ReplaceAllString(s, "$1$2[已隐藏]")
}

// Consume 仅记录日志与异常事件，不将回答全文写入日志。
func (l *Logger) Consume(e core.Event) {
	switch e.Kind {
	case "log":
		l.Log.Info(e.Text)
	case "warn", "error":
		l.Log.Warn(e.Text)
	}
}

// Close 关闭日志文件并返回落盘错误。
func (l *Logger) Close() error { return l.file.Close() }
