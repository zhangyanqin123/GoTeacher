package repository

import (
	"context"
	"database/sql"
	"fmt"

	"gyz-service/internal/model"
)

// 前端监控告警数据访问（mon_alert，见 PLAN-frontend-monitor.md）。

// InsertMonAlert 新增告警（去重：同 rule+env 已有 pending 时由 service 走 UpdateMonAlertMetric，
// uk_dedup_pending 兜底并发/重启场景下的重复 INSERT——撞唯一键时返回错误由调用方记日志即可）。
// dedupKey 由 service 组装（rule_code|env）单独传入：列不进 SELECT/行模型，纯内部去重语义
func (r *Repository) InsertMonAlert(ctx context.Context, a *model.MonAlert, dedupKey string) error {
	const q = `INSERT INTO mon_alert
	           (rule_code, level, env, ver, window_start, window_end, metric_value, threshold,
	            detail, status, dedup_key, created_at, updated_at)
	           VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, 'pending', ?, NOW(), NOW())`
	_, err := r.db.ExecContext(ctx, q, a.RuleCode, a.Level, a.Env, a.Ver,
		a.WindowStart, a.WindowEnd, a.MetricValue, a.Threshold, a.Detail, dedupKey)
	return err
}

// FindPendingMonAlertByRule 查同规则+环境的 pending 告警（滚动去重入口）。无则返回 (nil, nil)
func (r *Repository) FindPendingMonAlertByRule(ctx context.Context, ruleCode, env string) (*model.MonAlert, error) {
	const q = `SELECT id, rule_code, level, env, ver, window_start, window_end, metric_value,
	                  threshold, detail, status, ack_user, ack_note, acked_at, created_at, updated_at
	           FROM mon_alert WHERE rule_code = ? AND env = ? AND status = 'pending' LIMIT 1`
	var a model.MonAlert
	err := r.db.QueryRowContext(ctx, q, ruleCode, env).Scan(
		&a.ID, &a.RuleCode, &a.Level, &a.Env, &a.Ver, &a.WindowStart, &a.WindowEnd,
		&a.MetricValue, &a.Threshold, &a.Detail, &a.Status, &a.AckUser, &a.AckNote,
		&a.AckedAt, &a.CreatedAt, &a.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("find pending mon_alert %s/%s: %w", ruleCode, env, err)
	}
	return &a, nil
}

// UpdateMonAlertMetric 持续触发的滚动更新：指标/窗口/摘要刷新（不动 status，保持 pending）
func (r *Repository) UpdateMonAlertMetric(ctx context.Context, id int64, ver string, windowStart, windowEnd string,
	metric, threshold int, detail string) error {
	const q = `UPDATE mon_alert SET ver = ?, window_start = ?, window_end = ?, metric_value = ?,
	           threshold = ?, detail = ?, updated_at = NOW() WHERE id = ?`
	_, err := r.db.ExecContext(ctx, q, ver, windowStart, windowEnd, metric, threshold, detail, id)
	return err
}

// UpdateMonAlertAck 告警确认（status→acked 落库确认人与结论）。
// 返回 found=false 表示该 id 不存在或已非 pending（并发重复确认），哨兵错误由 service 包装
func (r *Repository) UpdateMonAlertAck(ctx context.Context, id int64, ackUser, ackNote string) (bool, error) {
	const q = `UPDATE mon_alert SET status = 'acked', ack_user = ?, ack_note = ?, acked_at = NOW(),
	           updated_at = NOW() WHERE id = ? AND status = 'pending'`
	res, err := r.db.ExecContext(ctx, q, ackUser, ackNote, id)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// ListMonAlerts 告警多条件分页查询（默认最新在前）
func (r *Repository) ListMonAlerts(ctx context.Context, f model.MonAlertListFilter) ([]model.MonAlert, int, error) {
	where, args := monAlertWhere(f)

	var count int
	if err := r.db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM mon_alert WHERE "+where, args...,
	).Scan(&count); err != nil {
		return nil, 0, fmt.Errorf("count mon_alert: %w", err)
	}

	const q = `SELECT id, rule_code, level, env, ver, window_start, window_end, metric_value,
	                  threshold, detail, status, ack_user, ack_note, acked_at, created_at, updated_at
	           FROM mon_alert WHERE %s ORDER BY id DESC LIMIT ? OFFSET ?`
	rows, err := r.db.QueryContext(ctx,
		fmt.Sprintf(q, where), append(args, f.Limit, f.Offset)...,
	)
	if err != nil {
		return nil, 0, fmt.Errorf("query mon_alert list: %w", err)
	}
	defer rows.Close()

	list := make([]model.MonAlert, 0, f.Limit)
	for rows.Next() {
		var a model.MonAlert
		if err := rows.Scan(
			&a.ID, &a.RuleCode, &a.Level, &a.Env, &a.Ver, &a.WindowStart, &a.WindowEnd,
			&a.MetricValue, &a.Threshold, &a.Detail, &a.Status, &a.AckUser, &a.AckNote,
			&a.AckedAt, &a.CreatedAt, &a.UpdatedAt,
		); err != nil {
			return nil, 0, fmt.Errorf("scan mon_alert row: %w", err)
		}
		list = append(list, a)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterate mon_alert rows: %w", err)
	}
	return list, count, nil
}

func monAlertWhere(f model.MonAlertListFilter) (string, []any) {
	where := "1 = 1"
	var args []any
	if f.Status != "" {
		where += " AND status = ?"
		args = append(args, f.Status)
	}
	if f.RuleCode != "" {
		where += " AND rule_code = ?"
		args = append(args, f.RuleCode)
	}
	if f.Level != "" {
		where += " AND level = ?"
		args = append(args, f.Level)
	}
	if f.Env != "" {
		where += " AND env = ?"
		args = append(args, f.Env)
	}
	if f.Begin != "" {
		where += " AND created_at >= ?"
		args = append(args, f.Begin)
	}
	if f.End != "" {
		where += " AND created_at <= ?"
		args = append(args, f.End)
	}
	return where, args
}

// DeleteMonEventsBefore 保留期清理：分批删除（LIMIT 5000）返回本轮删除行数，调用方循环至 0
func (r *Repository) DeleteMonEventsBefore(ctx context.Context, before string) (int64, error) {
	res, err := r.db.ExecContext(ctx,
		"DELETE FROM mon_event WHERE created_at < ? LIMIT 5000", before)
	if err != nil {
		return 0, fmt.Errorf("delete mon_event before %s: %w", before, err)
	}
	return res.RowsAffected()
}
