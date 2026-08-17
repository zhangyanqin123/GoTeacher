package repository

import (
	"context"
	"fmt"
	"strings"

	"handicap-service/internal/model"
)

// ListResigns 按筛选条件分页查询离职转移记录。
// 动态 WHERE：非零值条件才拼接；模糊条件用 LIKE CONCAT('%',?,'%')；
// 排序 id 倒序（mock add 后 unshift 到队首，最新在上，同构）。
func (r *Repository) ListResigns(ctx context.Context, f model.ResignListFilter) ([]model.Resign, int, error) {
	where, args := resignWhere(f)

	// 1. 总数
	var count int
	if err := r.db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM teacher_resign r WHERE "+where, args...,
	).Scan(&count); err != nil {
		return nil, 0, fmt.Errorf("count teacher_resign: %w", err)
	}

	// 2. 当前页（LIMIT/OFFSET 只能拼常量，参数放最后）
	const query = `SELECT r.id, r.original_teacher_id, r.original_teacher_name, r.original_teacher_dept_id, r.original_teacher_dept,
	                      r.replace_teacher_id, r.replace_teacher_name, r.replace_teacher_dept,
	                      r.salesman_name, r.salesman_dept, r.transfer_content, r.group_count,
	                      r.operator, r.operate_ip, r.transfer_time, r.remark, r.created_at, r.updated_at
	               FROM teacher_resign r
	               WHERE %s
	               ORDER BY r.id DESC
	               LIMIT ? OFFSET ?`

	list := make([]model.Resign, 0, f.PageSize)
	rows, err := r.db.QueryContext(ctx,
		fmt.Sprintf(query, where), append(args, f.PageSize, (f.PageIndex-1)*f.PageSize)...,
	)
	if err != nil {
		return nil, 0, fmt.Errorf("query teacher_resign list: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var it model.Resign
		if err := rows.Scan(
			&it.ID, &it.OriginalTeacherID, &it.OriginalTeacherName, &it.OriginalTeacherDeptID, &it.OriginalTeacherDept,
			&it.ReplaceTeacherID, &it.ReplaceTeacherName, &it.ReplaceTeacherDept,
			&it.SalesmanName, &it.SalesmanDept, &it.TransferContent, &it.GroupCount,
			&it.Operator, &it.OperateIP, &it.TransferTime, &it.Remark, &it.CreatedAt, &it.UpdatedAt,
		); err != nil {
			return nil, 0, fmt.Errorf("scan teacher_resign row: %w", err)
		}
		list = append(list, it)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterate teacher_resign rows: %w", err)
	}
	return list, count, nil
}

// resignWhere 拼接筛选条件，返回 WHERE 片段（不含 WHERE 关键字）与参数。
func resignWhere(f model.ResignListFilter) (string, []any) {
	var conds []string
	var args []any

	if f.DeptID != 0 {
		conds = append(conds, "r.original_teacher_dept_id = ?")
		args = append(args, f.DeptID)
	}
	if f.OriginalTeacherID != 0 {
		conds = append(conds, "r.original_teacher_id = ?")
		args = append(args, f.OriginalTeacherID)
	}
	if f.ReplaceTeacherID != 0 {
		conds = append(conds, "r.replace_teacher_id = ?")
		args = append(args, f.ReplaceTeacherID)
	}
	if f.SalesmanName != "" {
		conds = append(conds, "r.salesman_name LIKE CONCAT('%', ?, '%')")
		args = append(args, f.SalesmanName)
	}
	// 日期闭区间：EndTime 加一天走开区间，避免对列套函数失索引
	if f.TransferBeginTime != "" && f.TransferEndTime != "" {
		conds = append(conds, "r.transfer_time >= ? AND r.transfer_time < DATE_ADD(?, INTERVAL 1 DAY)")
		args = append(args, f.TransferBeginTime, f.TransferEndTime)
	}

	if len(conds) == 0 {
		return "1 = 1", args
	}
	return strings.Join(conds, " AND "), args
}

// GetTeachersByIDs 按ID批量取老师简要信息（add 时回查姓名/部门与存在性校验）
func (r *Repository) GetTeachersByIDs(ctx context.Context, ids []int64) ([]model.TeacherBrief, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	placeholders := strings.TrimRight(strings.Repeat("?,", len(ids)), ",")
	args := make([]any, len(ids))
	for i, id := range ids {
		args[i] = id
	}

	rows, err := r.db.QueryContext(ctx,
		"SELECT id, name, dept_id, dept_name FROM teacher WHERE id IN ("+placeholders+")", args...,
	)
	if err != nil {
		return nil, fmt.Errorf("query teacher briefs: %w", err)
	}
	defer rows.Close()

	list := make([]model.TeacherBrief, 0, len(ids))
	for rows.Next() {
		var t model.TeacherBrief
		if err := rows.Scan(&t.ID, &t.Name, &t.DeptID, &t.DeptName); err != nil {
			return nil, fmt.Errorf("scan teacher brief: %w", err)
		}
		list = append(list, t)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate teacher briefs: %w", err)
	}
	return list, nil
}

// ListTeacherSalesmen 老师绑定的全部业务员（add 时冗余 salesman_name/dept 快照）。
// 无绑定返回空切片（属正常路径，不视为错误）。
func (r *Repository) ListTeacherSalesmen(ctx context.Context, teacherID int64) ([]model.TeacherSalesmanBrief, error) {
	const query = `SELECT su.nickname, su.dept_name
	               FROM teacher_sales ts
	               JOIN sales_user su ON su.id = ts.user_id
	               WHERE ts.teacher_id = ?
	               ORDER BY ts.id`

	rows, err := r.db.QueryContext(ctx, query, teacherID)
	if err != nil {
		return nil, fmt.Errorf("query teacher salesmen %d: %w", teacherID, err)
	}
	defer rows.Close()

	list := make([]model.TeacherSalesmanBrief, 0)
	for rows.Next() {
		var s model.TeacherSalesmanBrief
		if err := rows.Scan(&s.Nickname, &s.DeptName); err != nil {
			return nil, fmt.Errorf("scan teacher salesman: %w", err)
		}
		list = append(list, s)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate teacher salesmen: %w", err)
	}
	return list, nil
}

// InsertResign 写入一条离职转移记录（transfer_time 库端 NOW()）。
// 单条 INSERT 无需事务（对齐 UpdateTeacher；事务仅多语句场景用）。
func (r *Repository) InsertResign(ctx context.Context, rec model.ResignInsert) error {
	const query = `INSERT INTO teacher_resign
	               (original_teacher_id, original_teacher_name, original_teacher_dept_id, original_teacher_dept,
	                replace_teacher_id, replace_teacher_name, replace_teacher_dept,
	                salesman_name, salesman_dept, transfer_content, group_count,
	                operator, operate_ip, transfer_time, remark)
	               VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, NOW(), ?)`
	content, err := rec.TransferContent.Value()
	if err != nil {
		return fmt.Errorf("insert teacher_resign value: %w", err)
	}
	if _, err := r.db.ExecContext(ctx, query,
		rec.OriginalTeacherID, rec.OriginalTeacherName, rec.OriginalTeacherDeptID, rec.OriginalTeacherDept,
		rec.ReplaceTeacherID, rec.ReplaceTeacherName, rec.ReplaceTeacherDept,
		rec.SalesmanName, rec.SalesmanDept, content, rec.GroupCount,
		rec.Operator, rec.OperateIP, rec.Remark,
	); err != nil {
		return fmt.Errorf("insert teacher_resign: %w", err)
	}
	return nil
}
