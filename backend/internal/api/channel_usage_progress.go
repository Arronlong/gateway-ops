package api

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/guliansheng/gateway-ops/internal/progress"
)

func (observer *sseObserver) keepAlive() {
	observer.mu.Lock()
	defer observer.mu.Unlock()
	if observer.closed {
		return
	}
	if _, err := io.WriteString(observer.w, ": keepalive\n\n"); err != nil {
		observer.closed = true
		return
	}
	if observer.flush != nil {
		observer.flush()
	}
}

func setupUsageProgress(ctx context.Context, c *gin.Context, d *Deps, site string) (context.Context, func()) {
	observer := setupSSE(c)
	stop, done := make(chan struct{}), make(chan struct{})
	go func() {
		defer close(done)
		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-stop:
				return
			case <-ctx.Done():
				return
			case <-ticker.C:
				info := d.Monitor.UsageRequestWait(site)
				if info.Status == 0 && time.Until(info.Until) > time.Second {
					observer.Emit(progress.Event{Stage: progress.StageUsage, Message: fmt.Sprintf("\u6b63\u5728\u7b49\u5f85\u4e0a\u6e38\u8bf7\u6c42\u989d\u5ea6\uff0c%s \u540e\u7ee7\u7eed", info.Until.UTC().Format(time.RFC3339)), Data: map[string]any{"kind": "usage_wait", "retry_at": info.Until}, Time: time.Now()})
				} else {
					observer.keepAlive()
				}
			}
		}
	}()
	return progress.WithObserver(ctx, observer), func() { close(stop); <-done }
}
