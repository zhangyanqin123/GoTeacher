package repository

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"handicap-service/internal/model"
)

// ListTeachers 按筛选条件分页查询老师列表。
// 动态 WHERE：非零值条件才拼接；模糊条件用 LIKE CONCAT('%',?,'%')；
// bindSalesCount 用相关子查询过滤（不落冗余列，避免绑定事务双写漂移）。
func (r *Repository) ListTeachers(ctx context.Context, f model.TeacherListFilter) ([]model.Teacher, int, error) {
	where, args := teacherWhere(f)

	// 1. 总数
	var count int
	if err := r.db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM teacher t WHERE "+where, args...,
	).Scan(&count); err != nil {
		return nil, 0, fmt.Errorf("count teacher: %w", err)
	}

	// 2. 当前页（bindSalesCount 由子查询带出；LIMIT/OFFSET 只能拼常量，参数放最后）
	const query = `SELECT t.id, t.account, t.name, t.nickname, t.title, t.qualification,
	                      (SELECT COUNT(*) FROM teacher_sales ts WHERE ts.teacher_id = t.id) AS bind_sales_count,
	                      t.dept_id, t.dept_name, t.phone, t.work_no, t.status, t.rating,
	                      t.avatar, t.signature, t.created_at, t.updated_at, t.update_by
	               FROM teacher t
	               WHERE %s
	               ORDER BY t.id
	               LIMIT ? OFFSET ?`

	list := make([]model.Teacher, 0, f.PageSize)
	rows, err := r.db.QueryContext(ctx,
		fmt.Sprintf(query, where), append(args, f.PageSize, (f.PageIndex-1)*f.PageSize)...,
	)
	if err != nil {
		return nil, 0, fmt.Errorf("query teacher list: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var t model.Teacher
		if err := rows.Scan(
			&t.ID, &t.Account, &t.Name, &t.Nickname, &t.Title, &t.Qualification,
			&t.BindSalesCount,
			&t.DeptID, &t.DeptName, &t.Phone, &t.WorkNo, &t.Status, &t.Rating,
			&t.Avatar, &t.Signature, &t.CreatedAt, &t.UpdatedAt, &t.UpdateBy,
		); err != nil {
			return nil, 0, fmt.Errorf("scan teacher row: %w", err)
		}
		list = append(list, t)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterate teacher rows: %w", err)
	}
	return list, count, nil
}

// teacherWhere 拼接筛选条件，返回 WHERE 片段（不含 WHERE 关键字）与参数。
func teacherWhere(f model.TeacherListFilter) (string, []any) {
	var conds []string
	var args []any

	if f.DeptID != 0 {
		conds = append(conds, "t.dept_id = ?")
		args = append(args, f.DeptID)
	}
	if f.ID != 0 {
		conds = append(conds, "t.id = ?")
		args = append(args, f.ID)
	}
	for col, val := range map[string]string{
		"t.account":   f.Account,
		"t.nickname":  f.Nickname,
		"t.name":      f.Name,
		"t.title":     f.Title,
		"t.update_by": f.UpdateBy,
	} {
		if val != "" {
			conds = append(conds, col+" LIKE CONCAT('%', ?, '%')")
			args = append(args, val)
		}
	}
	if f.Qualification != "" {
		conds = append(conds, "t.qualification = ?")
		args = append(args, f.Qualification)
	}
	if f.BindSalesCount != nil {
		conds = append(conds, "(SELECT COUNT(*) FROM teacher_sales ts WHERE ts.teacher_id = t.id) = ?")
		args = append(args, *f.BindSalesCount)
	}
	if f.Status != "" {
		conds = append(conds, "t.status = ?")
		args = append(args, f.Status)
	}
	// 日期闭区间：EndTime 加一天走开区间，避免对列套函数失索引
	if f.UpdateBeginTime != "" && f.UpdateEndTime != "" {
		conds = append(conds, "t.updated_at >= ? AND t.updated_at < DATE_ADD(?, INTERVAL 1 DAY)")
		args = append(args, f.UpdateBeginTime, f.UpdateEndTime)
	}

	if len(conds) == 0 {
		return "1 = 1", args
	}
	return strings.Join(conds, " AND "), args
}

// ListTeacherOptions 全量老师下拉选项（含停用，按 id 排序）
func (r *Repository) ListTeacherOptions(ctx context.Context) ([]model.TeacherOption, error) {
	const query = `SELECT id, name, dept_name FROM teacher ORDER BY id`

	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("query teacher options: %w", err)
	}
	defer rows.Close()

	list := make([]model.TeacherOption, 0, 16)
	for rows.Next() {
		var o model.TeacherOption
		if err := rows.Scan(&o.ID, &o.Name, &o.DeptName); err != nil {
			return nil, fmt.Errorf("scan teacher option: %w", err)
		}
		list = append(list, o)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate teacher options: %w", err)
	}
	return list, nil
}

// ExistsTeacher 老师是否存在。
// 存在性检查不用 RowsAffected==0 判断：MySQL 值未变时 affected rows 也是 0。
func (r *Repository) ExistsTeacher(ctx context.Context, id int64) (bool, error) {
	const query = `SELECT 1 FROM teacher WHERE id = ?`
	var one int
	err := r.db.QueryRowContext(ctx, query, id).Scan(&one)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("exists teacher %d: %w", id, err)
	}
	return true, nil
}

// UpdateTeacher 编辑老师（title/rating/avatar/signature）
func (r *Repository) UpdateTeacher(ctx context.Context, req model.TeacherUpdateReq, updateBy string) error {
	const query = `UPDATE teacher
	               SET title = ?, rating = ?, avatar = ?, signature = ?, update_by = ?
	               WHERE id = ?`
	if _, err := r.db.ExecContext(ctx, query,
		req.Title, req.Rating, req.Avatar, req.Signature, updateBy, req.ID,
	); err != nil {
		return fmt.Errorf("update teacher %d: %w", req.ID, err)
	}
	return nil
}

// ListTeacherSalesByTeacher 老师绑定的业务员分页列表（详情弹窗用）
func (r *Repository) ListTeacherSalesByTeacher(ctx context.Context, teacherID int64, limit, offset int) ([]model.TeacherSalesRow, int, error) {
	const countQuery = `SELECT COUNT(*) FROM teacher_sales WHERE teacher_id = ?`
	var count int
	if err := r.db.QueryRowContext(ctx, countQuery, teacherID).Scan(&count); err != nil {
		return nil, 0, fmt.Errorf("count teacher_sales %d: %w", teacherID, err)
	}

	const query = `SELECT su.phone, su.nickname, su.dept_name, ts.bind_time
	               FROM teacher_sales ts
	               JOIN sales_user su ON su.id = ts.user_id
	               WHERE ts.teacher_id = ?
	               ORDER BY ts.id
	               LIMIT ? OFFSET ?`

	list := make([]model.TeacherSalesRow, 0, limit)
	rows, err := r.db.QueryContext(ctx, query, teacherID, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("query teacher_sales %d: %w", teacherID, err)
	}
	defer rows.Close()

	for rows.Next() {
		var s model.TeacherSalesRow
		if err := rows.Scan(&s.Phone, &s.Nickname, &s.DeptName, &s.BindTime); err != nil {
			return nil, 0, fmt.Errorf("scan teacher_sales row: %w", err)
		}
		list = append(list, s)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterate teacher_sales rows: %w", err)
	}
	return list, count, nil
}

// ReplaceTeacherSales 全量替换老师的绑定业务员：事务内先删后插，空数组即清空绑定。
func (r *Repository) ReplaceTeacherSales(ctx context.Context, teacherID int64, userIDs []int64) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("replace teacher_sales begin: %w", err)
	}
	defer tx.Rollback() // 已提交时为 no-op

	if _, err := tx.ExecContext(ctx,
		"DELETE FROM teacher_sales WHERE teacher_id = ?", teacherID,
	); err != nil {
		return fmt.Errorf("delete teacher_sales %d: %w", teacherID, err)
	}

	if len(userIDs) > 0 {
		values := make([]string, len(userIDs))
		args := make([]any, 0, len(userIDs)*2)
		for i, uid := range userIDs {
			values[i] = "(?, ?, NOW())"
			args = append(args, teacherID, uid)
		}
		query := "INSERT INTO teacher_sales (teacher_id, user_id, bind_time) VALUES " + strings.Join(values, ", ")
		if _, err := tx.ExecContext(ctx, query, args...); err != nil {
			return fmt.Errorf("insert teacher_sales %d: %w", teacherID, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("replace teacher_sales commit: %w", err)
	}
	return nil
}

// CountSalesUsers 统计给定 id 中真实存在的业务员数（用于绑定前校验全部存在）
func (r *Repository) CountSalesUsers(ctx context.Context, ids []int64) (int, error) {
	if len(ids) == 0 {
		return 0, nil
	}
	placeholders := strings.TrimRight(strings.Repeat("?,", len(ids)), ",")
	args := make([]any, len(ids))
	for i, id := range ids {
		args[i] = id
	}

	var count int
	if err := r.db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM sales_user WHERE id IN ("+placeholders+")", args...,
	).Scan(&count); err != nil {
		return 0, fmt.Errorf("count sales_user: %w", err)
	}
	return count, nil
}
