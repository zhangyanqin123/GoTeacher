package repository

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"gyz-service/internal/model"
)

// ListDiagnoses 按筛选条件分页查询诊股记录。
// 动态 WHERE：nil 指针/空字符串条件不拼接；模糊条件用 LIKE CONCAT('%',?,'%')；
// 排序 id 倒序（最新在上，对齐 resign 先例；mock 的 1..6 顺序是数组副作用，页面不依赖）。
func (r *Repository) ListDiagnoses(ctx context.Context, f model.DiagnoseListFilter) ([]model.Diagnose, int, error) {
	where, args := diagnoseWhere(f)

	// 1. 总数
	var count int
	if err := r.db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM diagnose d WHERE "+where, args...,
	).Scan(&count); err != nil {
		return nil, 0, fmt.Errorf("count diagnose: %w", err)
	}

	// 2. 当前页（LIMIT/OFFSET 只能拼常量，参数放最后）
	const query = `SELECT d.id, d.user_nick_name, d.user_name, d.stock_code, d.stock_name,
	                      d.buy_price, d.buy_num, d.teacher_name, d.submit_time,
	                      d.report_content, d.report_submit_time, d.status, d.remark
	               FROM diagnose d
	               WHERE %s
	               ORDER BY d.id DESC
	               LIMIT ? OFFSET ?`

	list := make([]model.Diagnose, 0, f.PageSize)
	rows, err := r.db.QueryContext(ctx,
		fmt.Sprintf(query, where), append(args, f.PageSize, (f.PageIndex-1)*f.PageSize)...,
	)
	if err != nil {
		return nil, 0, fmt.Errorf("query diagnose list: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var it model.Diagnose
		if err := rows.Scan(
			&it.ID, &it.UserNickName, &it.UserName, &it.StockCode, &it.StockName,
			&it.BuyPrice, &it.BuyNum, &it.TeacherName, &it.SubmitTime,
			&it.ReportContent, &it.ReportSubmitTime, &it.Status, &it.Remark,
		); err != nil {
			return nil, 0, fmt.Errorf("scan diagnose row: %w", err)
		}
		list = append(list, it)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterate diagnose rows: %w", err)
	}
	return list, count, nil
}

// diagnoseWhere 拼接筛选条件，返回 WHERE 片段（不含 WHERE 关键字）与参数。
func diagnoseWhere(f model.DiagnoseListFilter) (string, []any) {
	var conds []string
	var args []any

	if f.ID != nil {
		conds = append(conds, "d.id = ?")
		args = append(args, *f.ID)
	}
	for _, like := range []struct {
		cond string
		val  string
	}{
		{"d.user_nick_name LIKE CONCAT('%', ?, '%')", f.UserNickName},
		{"d.user_name LIKE CONCAT('%', ?, '%')", f.UserName},
		{"d.stock_code LIKE CONCAT('%', ?, '%')", f.StockCode},
		{"d.stock_name LIKE CONCAT('%', ?, '%')", f.StockName},
		{"d.teacher_name LIKE CONCAT('%', ?, '%')", f.TeacherName},
	} {
		if like.val != "" {
			conds = append(conds, like.cond)
			args = append(args, like.val)
		}
	}
	if f.BuyPrice != nil {
		conds = append(conds, "d.buy_price = ?")
		args = append(args, *f.BuyPrice)
	}
	if f.BuyNum != nil {
		conds = append(conds, "d.buy_num = ?")
		args = append(args, *f.BuyNum)
	}
	if f.Status != nil {
		conds = append(conds, "d.status = ?")
		args = append(args, *f.Status)
	}
	// 日期闭区间：EndTime 加一天走开区间，避免对列套函数失索引；
	// report_submit_time 为 NULL（未提交）的行天然排除，与 mock 空串排除一致
	if f.SubmitBeginTime != "" && f.SubmitEndTime != "" {
		conds = append(conds, "d.submit_time >= ? AND d.submit_time < DATE_ADD(?, INTERVAL 1 DAY)")
		args = append(args, f.SubmitBeginTime, f.SubmitEndTime)
	}
	if f.ReportBeginTime != "" && f.ReportEndTime != "" {
		conds = append(conds, "d.report_submit_time >= ? AND d.report_submit_time < DATE_ADD(?, INTERVAL 1 DAY)")
		args = append(args, f.ReportBeginTime, f.ReportEndTime)
	}

	if len(conds) == 0 {
		return "1 = 1", args
	}
	return strings.Join(conds, " AND "), args
}

// GetDiagnose 按ID取单条诊股记录，不存在返回 (nil, nil)，404 语义归 service。
func (r *Repository) GetDiagnose(ctx context.Context, id int64) (*model.Diagnose, error) {
	const query = `SELECT d.id, d.user_nick_name, d.user_name, d.stock_code, d.stock_name,
	                      d.buy_price, d.buy_num, d.teacher_name, d.submit_time,
	                      d.report_content, d.report_submit_time, d.status, d.remark
	               FROM diagnose d WHERE d.id = ?`

	var it model.Diagnose
	err := r.db.QueryRowContext(ctx, query, id).Scan(
		&it.ID, &it.UserNickName, &it.UserName, &it.StockCode, &it.StockName,
		&it.BuyPrice, &it.BuyNum, &it.TeacherName, &it.SubmitTime,
		&it.ReportContent, &it.ReportSubmitTime, &it.Status, &it.Remark,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("query diagnose %d: %w", id, err)
	}
	return &it, nil
}

// ListDiagnoseAuditLogs 诊股记录的审核流程日志，按 id 正序（插入序=时间序，mock 同构）。
// 无日志返回空切片（属正常路径，不视为错误）。
func (r *Repository) ListDiagnoseAuditLogs(ctx context.Context, diagnoseID int64) ([]model.DiagnoseAuditLog, error) {
	const query = `SELECT l.log_time, l.log_type, l.operator, l.result, l.remark
	               FROM diagnose_audit_log l
	               WHERE l.diagnose_id = ?
	               ORDER BY l.id`

	rows, err := r.db.QueryContext(ctx, query, diagnoseID)
	if err != nil {
		return nil, fmt.Errorf("query diagnose audit logs %d: %w", diagnoseID, err)
	}
	defer rows.Close()

	list := make([]model.DiagnoseAuditLog, 0)
	for rows.Next() {
		var it model.DiagnoseAuditLog
		if err := rows.Scan(&it.Time, &it.Type, &it.Operator, &it.Result, &it.Remark); err != nil {
			return nil, fmt.Errorf("scan diagnose audit log: %w", err)
		}
		list = append(list, it)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate diagnose audit logs: %w", err)
	}
	return list, nil
}

// SubmitDiagnoseReport 提交诊股报告：条件 UPDATE（status IN (1,3,5) 作状态机守卫，
// RowsAffected==0 说明状态已流转，回滚返回 false）+ 同事务写流程日志。
func (r *Repository) SubmitDiagnoseReport(ctx context.Context, id int64, content string, log model.DiagnoseAuditLogInsert) (bool, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return false, fmt.Errorf("begin submit diagnose tx: %w", err)
	}
	defer tx.Rollback()

	res, err := tx.ExecContext(ctx,
		"UPDATE diagnose SET status = 2, report_content = ?, report_submit_time = NOW() WHERE id = ? AND status IN (1, 3, 5)",
		content, id,
	)
	if err != nil {
		return false, fmt.Errorf("update diagnose %d for submit: %w", id, err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("rows affected of submit diagnose %d: %w", id, err)
	}
	if affected == 0 {
		return false, nil // 状态不允许（2/4/6），未产生任何变更
	}

	if err := insertDiagnoseAuditLog(ctx, tx, log); err != nil {
		return false, err
	}
	if err := tx.Commit(); err != nil {
		return false, fmt.Errorf("commit submit diagnose tx: %w", err)
	}
	return true, nil
}

// AuditDiagnose 审核诊股报告：条件 UPDATE（WHERE status = fromStatus 作状态机守卫，
// RowsAffected==0 说明顺序错位/已审/未提审，返回 false）+ 同事务写流程日志。
func (r *Repository) AuditDiagnose(ctx context.Context, id int64, fromStatus, toStatus int, log model.DiagnoseAuditLogInsert) (bool, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return false, fmt.Errorf("begin audit diagnose tx: %w", err)
	}
	defer tx.Rollback()

	res, err := tx.ExecContext(ctx,
		"UPDATE diagnose SET status = ? WHERE id = ? AND status = ?",
		toStatus, id, fromStatus,
	)
	if err != nil {
		return false, fmt.Errorf("update diagnose %d for audit: %w", id, err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("rows affected of audit diagnose %d: %w", id, err)
	}
	if affected == 0 {
		return false, nil // 状态机未命中，未产生任何变更
	}

	if err := insertDiagnoseAuditLog(ctx, tx, log); err != nil {
		return false, err
	}
	if err := tx.Commit(); err != nil {
		return false, fmt.Errorf("commit audit diagnose tx: %w", err)
	}
	return true, nil
}

// insertDiagnoseAuditLog 流程日志落库（log_time 库端 NOW()），仅在有 tx 的写路径内调用
func insertDiagnoseAuditLog(ctx context.Context, tx *sql.Tx, log model.DiagnoseAuditLogInsert) error {
	const query = `INSERT INTO diagnose_audit_log (diagnose_id, log_type, operator, result, remark, log_time)
	               VALUES (?, ?, ?, ?, ?, NOW())`
	if _, err := tx.ExecContext(ctx, query,
		log.DiagnoseID, log.LogType, log.Operator, log.Result, log.Remark,
	); err != nil {
		return fmt.Errorf("insert diagnose_audit_log: %w", err)
	}
	return nil
}
