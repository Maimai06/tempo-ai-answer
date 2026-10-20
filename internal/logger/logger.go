// Package logger 提供控制台和文件日志，集中遮盖配置中的敏感值。
package logger

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"

	"github.com/Maimai06/tempo-ai-answer/internal/core"
)

var sensitive = regexp.MustCompile(`(?i)(token|password|authorization|cookie)([=: ]+)[^\s,;]+`)

// Logger 同时写控制台与私有日志文件，保留集中脱敏规则。
type Logger struct {
	Log     *slog.Logger
	fileLog *slog.Logger
	file    *os.File
	secrets []string
}

// Open 创建仅当前用户可读写的日志文件。
func Open(dir string, secrets ...string) (*Logger, error) {
	return OpenWithConsole(dir, os.Stderr, secrets...)
}

// OpenWithConsole 将简洁中文提示写入指定终端，完整时间和级别保留在私有日志中。
func OpenWithConsole(dir string, console io.Writer, secrets ...string) (*Logger, error) {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	f, e := os.OpenFile(filepath.Join(dir, "tempo.log"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if e != nil {
		return nil, e
	}
	l := &Logger{file: f, secrets: secrets}
	fileHandler := slog.NewTextHandler(f, &slog.HandlerOptions{ReplaceAttr: func(_ []string, a slog.Attr) slog.Attr {
		if a.Value.Kind() == slog.KindString {
			a.Value = slog.StringValue(l.Redact(a.Value.String()))
		}
		return a
	}})
	l.fileLog = slog.New(fileHandler)
	l.Log = slog.New(&consoleHandler{file: fileHandler, console: console, redact: l.Redact, mu: &sync.Mutex{}})
	return l, nil
}

// consoleHandler 的终端只显示级别和消息，属性及分组保留在文件日志中。
type consoleHandler struct {
	file    slog.Handler
	console io.Writer
	redact  func(string) string
	mu      *sync.Mutex
}

func (h *consoleHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.file.Enabled(ctx, level)
}
func (h *consoleHandler) Handle(ctx context.Context, record slog.Record) error {
	if err := h.file.Handle(ctx, record); err != nil {
		return err
	}
	label := "提示"
	if record.Level >= slog.LevelError {
		label = "错误"
	} else if record.Level >= slog.LevelWarn {
		label = "注意"
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	_, err := fmt.Fprintf(h.console, "[%s] %s\n", label, h.redact(record.Message))
	return err
}
func (h *consoleHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	clone := *h
	clone.file = h.file.WithAttrs(attrs)
	return &clone
}
func (h *consoleHandler) WithGroup(name string) slog.Handler {
	clone := *h
	clone.file = h.file.WithGroup(name)
	return &clone
}

// RecordError 仅将底层错误写入文件，终端由调用方给出可操作的提示。
func (l *Logger) RecordError(err error) { l.fileLog.Error(err.Error()) }

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
