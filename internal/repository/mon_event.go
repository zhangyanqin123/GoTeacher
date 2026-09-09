package repository

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"gyz-service/internal/model"
)

// 前端监控事件数据访问（mon_event，见 PLAN-frontend-monitor.md）。

// InsertMonEvents 批量写入事件行（ingest 双通道共用）。
// 行内 CapSyntax 已由 service 派生；不建唯一键——gif 弱网重发容忍 at-least-once
func (r *Repository) InsertMonEvents(ctx context.Context, rows []model.MonEventRow) error {
	if len(rows) == 0 {
		return nil
	}
	const cols = `event_type, env, ver, chrome_ver, osv, webview, net_type, device_model, os,
	              route, session_id, seq, capbad, cap_syntax, cap, ua, msg, stack, src, ip`
	one := "(" + strings.TrimSuffix(strings.Repeat("?, ", 20), ", ") + ")"
	q := fmt.Sprintf("INSERT INTO mon_event (%s) VALUES %s", cols,
		strings.TrimSuffix(strings.Repeat(one+", ", len(rows)), ", "))

	args := make([]any, 0, len(rows)*20)
	for _, e := range rows {
		args = append(args, e.EventType, e.Env, e.Ver, e.ChromeVer, e.Osv, e.Webview, e.NetType,
			e.DeviceModel, e.Os, e.Route, e.SessionID, e.Seq, e.Capbad, e.CapSyntax,
			e.Cap, e.Ua, e.Msg, e.Stack, e.Src, e.Ip)
	}
	if _, err := r.db.ExecContext(ctx, q, args...); err != nil {
		return fmt.Errorf("insert %d mon_event rows: %w", len(rows), err)
	}
	return nil
}

// ============ 查询（管理台多口径） ============

// monEventListCols 列表态 SELECT 列：不含 stack/cap/ua 全文，msg 截 100 字预览（详情另查）
const monEventListCols = `id, event_type, env, ver, chrome_ver, osv, webview, net_type, device_model, os,
       route, session_id, seq, capbad, cap_syntax, src, ip, LEFT(msg, 100) AS msg, created_at`

func scanMonEventRow(scan func(dest ...any) error) (model.MonEventRow, error) {
	var e model.MonEventRow
	err := scan(&e.ID, &e.EventType, &e.Env, &e.Ver, &e.ChromeVer, &e.Osv, &e.Webview, &e.NetType,
		&e.DeviceModel, &e.Os, &e.Route, &e.SessionID, &e.Seq, &e.Capbad, &e.CapSyntax,
		&e.Src, &e.Ip, &e.Msg, &e.CreatedAt)
	return e, err
}

// ListMonEvents 多口径动态 WHERE 分页查询（条件拼接同 abModuleWhere 先例：LIMIT/OFFSET 常量拼、参数尾部）
func (r *Repository) ListMonEvents(ctx context.Context, f model.MonEventListFilter) ([]model.MonEventRow, int, error) {
	where, args := monEventWhere(f)

	var count int
	if err := r.db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM mon_event WHERE "+where, args...,
	).Scan(&count); err != nil {
		return nil, 0, fmt.Errorf("count mon_event: %w", err)
	}

	rows, err := r.db.QueryContext(ctx,
		fmt.Sprintf("SELECT %s FROM mon_event WHERE %s ORDER BY id DESC LIMIT ? OFFSET ?",
			monEventListCols, where),
		append(args, f.Limit, f.Offset)...,
	)
	if err != nil {
		return nil, 0, fmt.Errorf("query mon_event list: %w", err)
	}
	defer rows.Close()

	list := make([]model.MonEventRow, 0, f.Limit)
	for rows.Next() {
		e, err := scanMonEventRow(rows.Scan)
		if err != nil {
			return nil, 0, fmt.Errorf("scan mon_event row: %w", err)
		}
		list = append(list, e)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterate mon_event rows: %w", err)
	}
	return list, count, nil
}

// GetMonEventByID 单行全字段（详情抽屉：含 stack/cap/ua 全文）。不存在返回 (nil, nil)
func (r *Repository) GetMonEventByID(ctx context.Context, id int64) (*model.MonEventRow, error) {
	const q = `SELECT id, event_type, env, ver, chrome_ver, osv, webview, net_type, device_model, os,
	                   route, session_id, seq, capbad, cap_syntax, src, ip, msg, created_at, ua, cap, stack
	           FROM mon_event WHERE id = ?`
	var e model.MonEventRow
	err := r.db.QueryRowContext(ctx, q, id).Scan(
		&e.ID, &e.EventType, &e.Env, &e.Ver, &e.ChromeVer, &e.Osv, &e.Webview, &e.NetType,
		&e.DeviceModel, &e.Os, &e.Route, &e.SessionID, &e.Seq, &e.Capbad, &e.CapSyntax,
		&e.Src, &e.Ip, &e.Msg, &e.CreatedAt, &e.Ua, &e.Cap, &e.Stack)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get mon_event %d: %w", id, err)
	}
	return &e, nil
}

// monEventWhere 多口径条件拼接。capbads 用 FIND_IN_SET（capbad 为无序逗号串，LIKE 会漏）；
// ver 用前缀匹配（探针 ver 含 commit hash 后缀，查询按版本号前缀即可）
func monEventWhere(f model.MonEventListFilter) (string, []any) {
	where := "1 = 1"
	var args []any
	where += " AND created_at >= ? AND created_at <= ?"
	args = append(args, f.Begin, f.End)
	if len(f.EventTypes) > 0 {
		where += " AND event_type IN (" + placeholders(len(f.EventTypes)) + ")"
		for _, t := range f.EventTypes {
			args = append(args, t)
		}
	}
	if f.Env != "" {
		where += " AND env = ?"
		args = append(args, f.Env)
	}
	if f.VerPrefix != "" {
		where += " AND ver LIKE CONCAT(?, '%')"
		args = append(args, f.VerPrefix)
	}
	if f.ChromeVer > 0 {
		where += " AND chrome_ver = ?"
		args = append(args, f.ChromeVer)
	}
	if f.SessionID != "" {
		where += " AND session_id = ?"
		args = append(args, f.SessionID)
	}
	if f.DeviceModel != "" {
		where += " AND device_model LIKE CONCAT('%', ?, '%')"
		args = append(args, f.DeviceModel)
	}
	for _, cb := range f.Capbads {
		where += " AND FIND_IN_SET(?, capbad)"
		args = append(args, cb)
	}
	if f.CapSyntax > 0 {
		where += " AND cap_syntax = 1"
	}
	if f.Route != "" {
		where += " AND route LIKE CONCAT('%', ?, '%')"
		args = append(args, f.Route)
	}
	if f.Msg != "" {
		where += " AND msg LIKE CONCAT('%', ?, '%')"
		args = append(args, f.Msg)
	}
	if f.Src != "" {
		where += " AND src LIKE CONCAT('%', ?, '%')"
		args = append(args, f.Src)
	}
	return where, args
}

// placeholders 生成 "?, ?, ?"（IN 子句用；数量受 service 白名单/入参上限约束）
func placeholders(n int) string {
	if n <= 0 {
		return ""
	}
	return strings.TrimSuffix(strings.Repeat("?, ", n), ", ")
}

// ============ 概览聚合 ============

// monRangeWhere 时间范围（+可选 env）条件，概览/告警统计共用
func monRangeWhere(begin, end, env string, types []string) (string, []any) {
	where := "created_at >= ? AND created_at <= ?"
	args := []any{begin, end}
	if env != "" {
		where += " AND env = ?"
		args = append(args, env)
	}
	if len(types) > 0 {
		where += " AND event_type IN (" + placeholders(len(types)) + ")"
		for _, t := range types {
			args = append(args, t)
		}
	}
	return where, args
}

// MonOverviewSummaryRow 概览指标单行聚合结果
type MonOverviewSummaryRow struct {
	Total    int
	Errors   int
	Sessions int
	Devices  int
	Syntax   int
}

// SumMonEvents 概览指标卡聚合（一次往返取全部标量；CAST 保证 SUM 扫进 int）。
// errors = 非错误事件（boot 启动存活 / device 设备画像）之外的全部事件
func (r *Repository) SumMonEvents(ctx context.Context, begin, end, env string) (MonOverviewSummaryRow, error) {
	where, args := monRangeWhere(begin, end, env, nil)
	var s MonOverviewSummaryRow
	err := r.db.QueryRowContext(ctx, `SELECT COUNT(*),
	       CAST(COALESCE(SUM(event_type NOT IN ('boot', 'device')), 0) AS SIGNED),
	       CAST(COUNT(DISTINCT session_id) AS SIGNED),
	       CAST(COUNT(DISTINCT NULLIF(device_model, '')) AS SIGNED),
	       CAST(COALESCE(SUM(cap_syntax), 0) AS SIGNED)
	       FROM mon_event WHERE `+where, args...,
	).Scan(&s.Total, &s.Errors, &s.Sessions, &s.Devices, &s.Syntax)
	if err != nil {
		return s, fmt.Errorf("sum mon_event overview: %w", err)
	}
	return s, nil
}

// GroupMonEventsByType 按事件类型计数（概览 by_type）
func (r *Repository) GroupMonEventsByType(ctx context.Context, begin, end, env string) (map[string]int, error) {
	where, args := monRangeWhere(begin, end, env, nil)
	rows, err := r.db.QueryContext(ctx,
		"SELECT event_type, CAST(COUNT(*) AS SIGNED) FROM mon_event WHERE "+where+" GROUP BY event_type", args...)
	if err != nil {
		return nil, fmt.Errorf("group mon_event by type: %w", err)
	}
	defer rows.Close()
	out := map[string]int{}
	for rows.Next() {
		var k string
		var n int
		if err := rows.Scan(&k, &n); err != nil {
			return nil, fmt.Errorf("scan group row: %w", err)
		}
		out[k] = n
	}
	return out, rows.Err()
}

// monGroupCols 可作为 GROUP BY 列的白名单（列名不能参数化，白名单防注入）
var monGroupCols = map[string]bool{
	"ver": true, "device_model": true, "chrome_ver": true, "route": true, "src": true, "msg": true,
}

// GroupMonEventsTop 通用 Top-N 聚合（概览与告警 detail 共用）。
// col 必须在 monGroupCols 白名单内；syntaxExtra=true 时附带 SUM(cap_syntax) 作 Extra（by_mdl 用）
func (r *Repository) GroupMonEventsTop(ctx context.Context, begin, end, env string, types []string,
	col string, n int, syntaxExtra bool) ([]model.MonGroupRow, error) {
	if !monGroupCols[col] {
		return nil, fmt.Errorf("group col %q not allowed", col)
	}
	where, args := monRangeWhere(begin, end, env, types)
	sel := fmt.Sprintf("SELECT %s, CAST(COUNT(*) AS SIGNED)", col)
	if syntaxExtra {
		sel += ", CAST(COALESCE(SUM(cap_syntax), 0) AS SIGNED)"
	}
	q := sel + " FROM mon_event WHERE " + where +
		fmt.Sprintf(" AND %s <> '' GROUP BY %s ORDER BY 2 DESC LIMIT %d", col, col, n)

	rows, err := r.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("group mon_event top by %s: %w", col, err)
	}
	defer rows.Close()

	list := make([]model.MonGroupRow, 0, n)
	for rows.Next() {
		var g model.MonGroupRow
		if syntaxExtra {
			if err := rows.Scan(&g.Key, &g.Count, &g.Extra); err != nil {
				return nil, fmt.Errorf("scan group top row: %w", err)
			}
		} else if err := rows.Scan(&g.Key, &g.Count); err != nil {
			return nil, fmt.Errorf("scan group top row: %w", err)
		}
		list = append(list, g)
	}
	return list, rows.Err()
}

// GroupMonEventsByCvAsc 内核版本升序分布（低内核置顶；chrome_ver 为数字列，单独走 CAST 拼接 Key）
func (r *Repository) GroupMonEventsByCvAsc(ctx context.Context, begin, end, env string, n int) ([]model.MonGroupRow, error) {
	where, args := monRangeWhere(begin, end, env, nil)
	rows, err := r.db.QueryContext(ctx,
		"SELECT CAST(chrome_ver AS CHAR), CAST(COUNT(*) AS SIGNED) FROM mon_event WHERE "+where+
			" GROUP BY chrome_ver ORDER BY chrome_ver ASC LIMIT "+fmt.Sprint(n), args...)
	if err != nil {
		return nil, fmt.Errorf("group mon_event by cv: %w", err)
	}
	defer rows.Close()
	list := make([]model.MonGroupRow, 0, n)
	for rows.Next() {
		var g model.MonGroupRow
		if err := rows.Scan(&g.Key, &g.Count); err != nil {
			return nil, fmt.Errorf("scan cv row: %w", err)
		}
		list = append(list, g)
	}
	return list, rows.Err()
}

// CountMonEventsWindow 告警窗口统计（types + env + 时间下界；上界恒 now 由调用方拼进 begin 即可）
func (r *Repository) CountMonEventsWindow(ctx context.Context, env string, types []string, begin string) (int, error) {
	where, args := monRangeWhere(begin, "9999-12-31 23:59:59", env, types)
	var n int
	if err := r.db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM mon_event WHERE "+where, args...).Scan(&n); err != nil {
		return 0, fmt.Errorf("count mon_event window: %w", err)
	}
	return n, nil
}

// FirstMonSidInWindow 窗口内样例 sid（告警 detail 提供追溯入口；无命中返回空串）
func (r *Repository) FirstMonSidInWindow(ctx context.Context, env string, types []string, begin string) (string, error) {
	where, args := monRangeWhere(begin, "9999-12-31 23:59:59", env, types)
	var sid string
	err := r.db.QueryRowContext(ctx,
		"SELECT session_id FROM mon_event WHERE "+where+" AND session_id <> '' ORDER BY id DESC LIMIT 1",
		args...).Scan(&sid)
	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("first mon sid: %w", err)
	}
	return sid, nil
}
