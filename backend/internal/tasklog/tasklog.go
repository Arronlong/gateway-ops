package tasklog

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

type contextKey struct{}
type Recorder struct {
	mu        sync.Mutex
	lines     strings.Builder
	failed    bool
	truncated bool
}

var bearer = regexp.MustCompile(`(?i)\b(?:bearer|basic)\s+[^\s"'<>]+`)

// 上游错误可能包含转义 JSON 或完整认证头，敏感字段后的行内内容整段隐去。
var credentials = regexp.MustCompile(`(?im)((?:api[_-]?key|access[_-]?token|refresh[_-]?token|token|password|authorization|proxy-authorization|secret|cookie|set-cookie)[\\"']*\s*[=:]\s*)[^\r\n]*`)
var apiKeys = regexp.MustCompile(`sk-[a-zA-Z0-9_-]{8,}`)
var urlCredentials = regexp.MustCompile(`(https?://)[^/\s:@]+:[^/\s@]+@`)

func Redact(text string) string {
	text = bearer.ReplaceAllString(text, "[REDACTED]")
	text = credentials.ReplaceAllString(text, "${1}[REDACTED]")
	text = apiKeys.ReplaceAllString(text, "[REDACTED]")
	return urlCredentials.ReplaceAllString(text, "${1}[REDACTED]@")
}

func WithRecorder(ctx context.Context) (context.Context, *Recorder) {
	recorder := &Recorder{}
	return context.WithValue(ctx, contextKey{}, recorder), recorder
}

func Add(ctx context.Context, level, format string, args ...any) {
	recorder, _ := ctx.Value(contextKey{}).(*Recorder)
	if recorder == nil {
		return
	}
	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	if level == "ERROR" {
		recorder.failed = true
	}
	if recorder.truncated {
		return
	}
	line := fmt.Sprintf("%s [%s] %s\n", time.Now().UTC().Format(time.RFC3339Nano), level, Redact(fmt.Sprintf(format, args...)))
	remaining := 64*1024 - recorder.lines.Len()
	if len(line) > remaining {
		line = line[:remaining]
		for !utf8.ValidString(line) {
			line = line[:len(line)-1]
		}
		recorder.truncated = true
		line += "\n[\u65e5\u5fd7\u8fbe\u5230 64 KiB\uff0c\u540e\u7eed\u5185\u5bb9\u5df2\u7701\u7565]\n"
	}
	recorder.lines.WriteString(line)
}

func (r *Recorder) Snapshot() (string, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.lines.String(), r.failed
}
