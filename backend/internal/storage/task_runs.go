package storage

import (
	"context"
	"gorm.io/gorm"
	"time"
)

type TaskRun struct {
	ID         uint       `gorm:"primaryKey" json:"id"`
	Task       string     `gorm:"size:48;not null;index:idx_task_runs_history,priority:1" json:"task"`
	Status     string     `gorm:"size:16;not null;index" json:"status"`
	StartedAt  time.Time  `gorm:"not null;index:idx_task_runs_history,priority:2" json:"started_at"`
	FinishedAt *time.Time `json:"finished_at"`
	DurationMS int64      `json:"duration_ms"`
	Summary    string     `gorm:"type:text" json:"summary"`
	Logs       string     `gorm:"type:text" json:"logs,omitempty"`
}

type TaskRunPage struct {
	Runs      []TaskRun `json:"runs" gorm:"-"`
	Total     int64     `json:"total"`
	Succeeded int64     `json:"succeeded"`
	Failed    int64     `json:"failed"`
	Running   int64     `json:"running"`
	Page      int       `json:"page"`
	PageSize  int       `json:"page_size"`
}

type TaskRuns struct{ db *gorm.DB }

func NewTaskRuns(db *gorm.DB) *TaskRuns { return &TaskRuns{db: db} }
func (s *Settings) TaskRuns() *TaskRuns { return NewTaskRuns(s.db) }

func (r *TaskRuns) Start(task string) (*TaskRun, error) {
	run := &TaskRun{Task: task, Status: "running", StartedAt: time.Now().UTC()}
	return run, r.db.Create(run).Error
}

func (r *TaskRuns) Finish(run *TaskRun, status, summary, logs string) error {
	return r.FinishContext(context.Background(), run, status, summary, logs)
}

func (r *TaskRuns) FinishContext(ctx context.Context, run *TaskRun, status, summary, logs string) error {
	if run.FinishedAt == nil {
		now := time.Now().UTC()
		run.FinishedAt = &now
		run.DurationMS = now.Sub(run.StartedAt).Milliseconds()
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	return r.db.WithContext(ctx).Model(&TaskRun{}).Where("id = ? AND status = ?", run.ID, "running").Updates(map[string]any{
		"status": status, "summary": summary, "logs": logs, "finished_at": *run.FinishedAt, "duration_ms": run.DurationMS,
	}).Error
}

func (r *TaskRuns) InterruptRunning(before time.Time) error {
	return r.db.Model(&TaskRun{}).Where("status = ? AND started_at < ?", "running", before).Updates(map[string]any{
		"status": "failed", "finished_at": before, "summary": "\u670d\u52a1\u91cd\u542f\uff0c\u4e0a\u6b21\u6267\u884c\u672a\u6b63\u5e38\u7ed3\u675f",
		"logs":        gorm.Expr("COALESCE(logs, '') || ?", "\n\u670d\u52a1\u91cd\u542f\uff0c\u6267\u884c\u5df2\u4e2d\u65ad\uff1b\u4e2d\u65ad\u671f\u95f4\u7684\u65e5\u5fd7\u53ef\u80fd\u4e0d\u5b8c\u6574\u3002"),
		"duration_ms": gorm.Expr("GREATEST(0, EXTRACT(EPOCH FROM (? - started_at)) * 1000)::bigint", before),
	}).Error
}

func (r *TaskRuns) List(task string, page, pageSize int) (TaskRunPage, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}
	out := TaskRunPage{Runs: []TaskRun{}, Page: page, PageSize: pageSize}
	q := r.db.Model(&TaskRun{}).Where("task = ?", task)
	if err := q.Select("count(*) AS total, COALESCE(SUM(CASE WHEN status = 'success' THEN 1 ELSE 0 END), 0) AS succeeded, COALESCE(SUM(CASE WHEN status = 'failed' THEN 1 ELSE 0 END), 0) AS failed, COALESCE(SUM(CASE WHEN status = 'running' THEN 1 ELSE 0 END), 0) AS running").Scan(&out).Error; err != nil {
		return out, err
	}
	out.Page, out.PageSize = page, pageSize
	err := r.db.Where("task = ?", task).Omit("logs").Order("started_at DESC,id DESC").Offset((page - 1) * pageSize).Limit(pageSize).Find(&out.Runs).Error
	return out, err
}

func (r *TaskRuns) Find(task string, id uint) (*TaskRun, error) {
	var run TaskRun
	err := r.db.Where("task = ? AND id = ?", task, id).First(&run).Error
	return &run, err
}
