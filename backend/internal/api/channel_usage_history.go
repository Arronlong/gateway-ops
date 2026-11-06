package api

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/guliansheng/gateway-ops/internal/monitor"
	"github.com/guliansheng/gateway-ops/internal/progress"
)

func usageCheckWindow(since, until time.Time, rangeName string, now time.Time) (time.Time, time.Time) {
	cutoff := now.Add(-2 * time.Minute).UTC().Truncate(5 * time.Minute)
	if until.After(cutoff) {
		until = cutoff
	}
	if rangeName != "custom" && !since.IsZero() {
		rounded := since.Truncate(5 * time.Minute)
		if rounded.Before(since) {
			rounded = rounded.Add(5 * time.Minute)
		}
		since = rounded
	}
	return since, until
}

func channelUsageIntegrity(c *gin.Context, d *Deps) {
	since, until, rangeName, err := requestedTimeWindow(c, "today")
	if err != nil {
		fail(c, http.StatusBadRequest, err)
		return
	}
	id, err := uintParam(c, "id")
	if err != nil {
		fail(c, http.StatusBadRequest, err)
		return
	}
	ch, err := d.Channels.FindByID(id)
	if err != nil {
		fail(c, http.StatusNotFound, err)
		return
	}
	if d.Monitor == nil {
		fail(c, http.StatusServiceUnavailable, fmt.Errorf("channel monitoring is unavailable"))
		return
	}
	stream := c.Query("stream") == "1"
	ctx := c.Request.Context()
	if stream {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, 20*time.Minute)
		defer cancel()
		var stop func()
		ctx, stop = setupUsageProgress(ctx, c, d, ch.SiteURL)
		defer stop()
		progress.Start(ctx, progress.StageUsage, "\u6b63\u5728\u6838\u5bf9\u603b\u6570\u2026")
	}
	since, until = usageCheckWindow(since, until, rangeName, time.Now())
	if !since.Before(until) {
		result := monitor.UsageIntegrity{Status: "pending", Since: since, Until: since, CheckedAt: time.Now().UTC(), Accounts: []monitor.UsageAccountIntegrity{}}
		if stream {
			progress.OK(ctx, progress.StageDone, "\u533a\u95f4\u5c1a\u672a\u7a33\u5b9a", result)
		} else {
			c.JSON(http.StatusOK, gin.H{"data": result})
		}
		return
	}
	result, err := d.Monitor.CheckUsageTotals(ctx, ch, since, until)
	if err != nil {
		if stream {
			progress.Fail(ctx, progress.StageError, err.Error())
			return
		}
		fail(c, http.StatusInternalServerError, err)
		return
	}
	if stream {
		message := "\u6838\u5bf9\u5b8c\u6210"
		if result.Status == "matched" {
			message = "\u603b\u6570\u4e00\u81f4\uff0c\u5df2\u8df3\u8fc7\u5206\u6bb5\u6838\u5bf9"
		}
		if result.Status == "unavailable" {
			message = "\u6838\u5bf9\u5931\u8d25\uff0c\u8bf7\u67e5\u770b\u8d26\u53f7\u9519\u8bef\u8be6\u60c5"
		}
		if result.Phase == "interrupted" {
			message = "\u6838\u5bf9\u672a\u5b8c\u6210\uff0c\u5df2\u4fdd\u7559\u603b\u4f53\u5dee\u5f02\u548c\u5df2\u5b9a\u4f4d\u533a\u95f4"
		}
		progress.OK(ctx, progress.StageDone, message, result)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": result})
}

func refetchChannelUsageHistory(c *gin.Context, d *Deps) {
	since, until, _, err := requestedTimeWindow(c, "30d")
	if err != nil {
		fail(c, http.StatusBadRequest, err)
		return
	}
	now := time.Now().UTC()
	if until.After(now) {
		until = now
	}
	if !since.Before(until) {
		fail(c, http.StatusBadRequest, fmt.Errorf("history window must begin before now"))
		return
	}
	id, err := uintParam(c, "id")
	if err != nil {
		fail(c, http.StatusBadRequest, err)
		return
	}
	ch, err := d.Channels.FindByID(id)
	if err != nil {
		fail(c, http.StatusNotFound, err)
		return
	}
	if d.Monitor == nil {
		fail(c, http.StatusServiceUnavailable, fmt.Errorf("channel monitoring is unavailable"))
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 20*time.Minute)
	defer cancel()
	var stop func()
	ctx, stop = setupUsageProgress(ctx, c, d, ch.SiteURL)
	defer stop()
	progress.Start(ctx, progress.StageUsage, "\u6b63\u5728\u6838\u5bf9\u603b\u6570\uff0c\u4e00\u81f4\u533a\u95f4\u5c06\u76f4\u63a5\u8df3\u8fc7\u2026")
	report, err := d.Monitor.RefetchUsageHistoryWithReport(ctx, ch, since, until)
	if err != nil {
		progress.Fail(ctx, progress.StageError, err.Error())
		return
	}
	progress.OK(ctx, progress.StageDone, fmt.Sprintf("\u5386\u53f2\u6821\u9a8c\u5b8c\u6210\uff1a\u68c0\u67e5 %d \u4e2a\u533a\u95f4\uff0c\u8df3\u8fc7 %d \u4e2a\u4e00\u81f4\u533a\u95f4\uff0c\u4fee\u590d %d \u5929", report.CheckedWindows, report.SkippedWindows, report.RepairedDays), report.Result)
}
