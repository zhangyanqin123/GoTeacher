package database

import (
	"context"
	"database/sql/driver"
	"fmt"
	"log/slog"
	"reflect"
	"strings"
	"sync"
	"time"
)

// SQL 执行日志包装器：包在 mysql 驱动的 driver.Connector 外，拦截所有 SQL 调用打 slog.Debug 日志。
// 零新依赖、零 repository 改动；事务（tx.ExecContext 复用同一 driver.Conn）与启动期 Migrate/Seed 自动覆盖。
//
// 日志点位置（关键）：
//   - 项目 DSN 未开 interpolateParams，带参 SQL 在 Conn 层被 mysql 驱动以 driver.ErrSkip 拒绝，
//     database/sql 随即走 Prepare → Stmt 路径——带参 SQL 的主日志点在 Stmt.ExecContext/QueryContext，
//     无参 DDL/COUNT 走 Conn.ExecContext/QueryContext。Conn 层透传 ErrSkip 时绝不能打日志（是路径探测）。
//   - BEGIN/COMMIT/ROLLBACK 不经过 Exec/Query，在 Conn.Begin(BeginTx)/Tx.Commit/Rollback 处单独打。
//   - QueryContext 的 cost 只算到驱动返回首行元数据（MySQL 协议流式）；行数与全程耗时在 Rows.Close 补充。
//
// 开关：LOG_LEVEL=debug 时可见（默认 info 零输出，且 logSQL 先做 Enabled 检查，不做参数格式化）。

// maxArgLen 单个参数打印上限：diagnose.remark 富文本 HTML 可能很长，防刷屏
const maxArgLen = 512

// ---- Connector ----

// sqllogConnector 包装内层 connector，Connect 返回的 Conn 继续被打日志
type sqllogConnector struct {
	inner driver.Connector
}

// NewConnector 包装 mysql 驱动的 connector，配合 sql.OpenDB 使用
func NewConnector(inner driver.Connector) driver.Connector {
	return &sqllogConnector{inner: inner}
}

func (c *sqllogConnector) Connect(ctx context.Context) (driver.Conn, error) {
	conn, err := c.inner.Connect(ctx)
	if err != nil {
		return nil, err
	}
	// 仅新建物理连接时打（低频高价值，排查连接风暴/泄漏）；放回池不算
	logSQL("connect", "", nil, 0, -1, nil)
	return &sqllogConn{inner: conn}, nil
}

func (c *sqllogConnector) Driver() driver.Driver {
	return c.inner.Driver()
}

// ---- Conn ----

type sqllogConn struct {
	inner driver.Conn
}

func (c *sqllogConn) Prepare(query string) (driver.Stmt, error) {
	stmt, err := c.inner.Prepare(query)
	if err != nil {
		return nil, err
	}
	return &sqllogStmt{inner: stmt, query: query}, nil
}

func (c *sqllogConn) Close() error {
	return c.inner.Close()
}

func (c *sqllogConn) Begin() (driver.Tx, error) {
	start := time.Now()
	tx, err := c.inner.Begin()
	logSQL("BEGIN", "", nil, time.Since(start), -1, err)
	if err != nil {
		return nil, err
	}
	return &sqllogTx{inner: tx}, nil
}

func (c *sqllogConn) PrepareContext(ctx context.Context, query string) (driver.Stmt, error) {
	if pc, ok := c.inner.(driver.ConnPrepareContext); ok {
		stmt, err := pc.PrepareContext(ctx, query)
		if err != nil {
			return nil, err
		}
		return &sqllogStmt{inner: stmt, query: query}, nil
	}
	return c.Prepare(query)
}

func (c *sqllogConn) ExecContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Result, error) {
	ex, ok := c.inner.(driver.ExecerContext)
	if !ok {
		return nil, driver.ErrSkip // 内层未实现，交还 database/sql 走 Prepare 路径
	}
	start := time.Now()
	res, err := ex.ExecContext(ctx, query, args)
	if err == driver.ErrSkip {
		return nil, err // 路径探测回退，不是执行；打日志会同一条 SQL 打两条
	}
	rows := rowsAffected(res)
	logSQL("exec", query, args, time.Since(start), rows, err)
	if err != nil {
		return nil, err
	}
	return &sqllogResult{inner: res}, nil
}

func (c *sqllogConn) QueryContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	qy, ok := c.inner.(driver.QueryerContext)
	if !ok {
		return nil, driver.ErrSkip // 同 ExecContext
	}
	start := time.Now()
	rows, err := qy.QueryContext(ctx, query, args)
	if err == driver.ErrSkip {
		return nil, err // 同 ExecContext
	}
	logSQL("query", query, args, time.Since(start), -1, err)
	if err != nil {
		return nil, err
	}
	return &sqllogRows{inner: rows, query: query, start: start}, nil
}

func (c *sqllogConn) BeginTx(ctx context.Context, opts driver.TxOptions) (driver.Tx, error) {
	start := time.Now()
	if bt, ok := c.inner.(driver.ConnBeginTx); ok {
		tx, err := bt.BeginTx(ctx, opts)
		logSQL("BEGIN", "", nil, time.Since(start), -1, err)
		if err != nil {
			return nil, err
		}
		return &sqllogTx{inner: tx}, nil
	}
	return c.Begin()
}

// 以下纯委托：mysql 驱动实现了这些可选接口，丢失会导致静默退化——
// ResetSession 丢 → 连接放回池时被关闭重连；Validator 丢 → 坏连接检测退化；
// Pinger 丢 → 探活空转；NamedValueChecker 丢 → model.StringSlice 等 Valuer 转换退回默认转换器。

func (c *sqllogConn) Ping(ctx context.Context) error {
	if p, ok := c.inner.(driver.Pinger); ok {
		return p.Ping(ctx)
	}
	return nil
}

func (c *sqllogConn) ResetSession(ctx context.Context) error {
	if s, ok := c.inner.(driver.SessionResetter); ok {
		return s.ResetSession(ctx)
	}
	return nil
}

func (c *sqllogConn) IsValid() bool {
	if v, ok := c.inner.(driver.Validator); ok {
		return v.IsValid()
	}
	return true
}

func (c *sqllogConn) CheckNamedValue(nv *driver.NamedValue) error {
	if nvc, ok := c.inner.(driver.NamedValueChecker); ok {
		return nvc.CheckNamedValue(nv)
	}
	return driver.ErrSkip
}

// ---- Stmt（带参 SQL 的主执行路径）----

type sqllogStmt struct {
	inner driver.Stmt
	query string // Prepare 时记录的 SQL 原文
}

func (s *sqllogStmt) Close() error {
	return s.inner.Close()
}

func (s *sqllogStmt) NumInput() int {
	return s.inner.NumInput()
}

func (s *sqllogStmt) Exec(args []driver.Value) (driver.Result, error) {
	start := time.Now()
	res, err := s.inner.Exec(args)
	rows := rowsAffected(res)
	logSQL("exec", s.query, valuesToNamed(args), time.Since(start), rows, err)
	if err != nil {
		return nil, err
	}
	return &sqllogResult{inner: res}, nil
}

func (s *sqllogStmt) Query(args []driver.Value) (driver.Rows, error) {
	start := time.Now()
	rows, err := s.inner.Query(args)
	logSQL("query", s.query, valuesToNamed(args), time.Since(start), -1, err)
	if err != nil {
		return nil, err
	}
	return &sqllogRows{inner: rows, query: s.query, start: start}, nil
}

func (s *sqllogStmt) ExecContext(ctx context.Context, args []driver.NamedValue) (driver.Result, error) {
	se, ok := s.inner.(driver.StmtExecContext)
	if !ok {
		return s.Exec(namedToValues(args)) // 回退无 ctx 版（database/sql 外层已取消保护）
	}
	start := time.Now()
	res, err := se.ExecContext(ctx, args)
	rows := rowsAffected(res)
	logSQL("exec", s.query, args, time.Since(start), rows, err)
	if err != nil {
		return nil, err
	}
	return &sqllogResult{inner: res}, nil
}

func (s *sqllogStmt) QueryContext(ctx context.Context, args []driver.NamedValue) (driver.Rows, error) {
	sq, ok := s.inner.(driver.StmtQueryContext)
	if !ok {
		return s.Query(namedToValues(args)) // 回退无 ctx 版
	}
	start := time.Now()
	rows, err := sq.QueryContext(ctx, args)
	logSQL("query", s.query, args, time.Since(start), -1, err)
	if err != nil {
		return nil, err
	}
	return &sqllogRows{inner: rows, query: s.query, start: start}, nil
}

func (s *sqllogStmt) CheckNamedValue(nv *driver.NamedValue) error {
	if nvc, ok := s.inner.(driver.NamedValueChecker); ok {
		return nvc.CheckNamedValue(nv)
	}
	return driver.ErrSkip
}

func (s *sqllogStmt) ColumnConverter(idx int) driver.ValueConverter {
	if cc, ok := s.inner.(driver.ColumnConverter); ok {
		return cc.ColumnConverter(idx)
	}
	return driver.DefaultParameterConverter
}

// ---- Rows ----

type sqllogRows struct {
	inner driver.Rows
	query string
	count int64     // 已取行数
	start time.Time // 首行元数据返回时刻（QueryContext 计时起点）
	once  sync.Once // Close 幂等，日志只打一次
}

func (r *sqllogRows) Columns() []string {
	return r.inner.Columns()
}

func (r *sqllogRows) Close() error {
	err := r.inner.Close()
	r.once.Do(func() {
		// op=rows：行数 + 取完全部行的耗时（主日志的 cost 只到首行，此处补全程）
		logSQL("rows", r.query, nil, time.Since(r.start), r.count, err)
	})
	return err
}

func (r *sqllogRows) Next(dest []driver.Value) error {
	err := r.inner.Next(dest)
	if err == nil {
		r.count++
	}
	return err
}

// 以下委托防将来退化：调用 sql.Rows.ColumnTypes()/NextResultSet() 时拿到的是 mysql 驱动的真实元数据

func (r *sqllogRows) HasNextResultSet() bool {
	if nr, ok := r.inner.(driver.RowsNextResultSet); ok {
		return nr.HasNextResultSet()
	}
	return false
}

func (r *sqllogRows) NextResultSet() error {
	if nr, ok := r.inner.(driver.RowsNextResultSet); ok {
		return nr.NextResultSet()
	}
	return driver.ErrSkip
}

func (r *sqllogRows) ColumnTypeDatabaseTypeName(i int) string {
	if ct, ok := r.inner.(driver.RowsColumnTypeDatabaseTypeName); ok {
		return ct.ColumnTypeDatabaseTypeName(i)
	}
	return ""
}

func (r *sqllogRows) ColumnTypeNullable(i int) (nullable, ok bool) {
	if ct, ok := r.inner.(driver.RowsColumnTypeNullable); ok {
		return ct.ColumnTypeNullable(i)
	}
	return false, false
}

func (r *sqllogRows) ColumnTypePrecisionScale(i int) (precision, scale int64, ok bool) {
	if ct, ok := r.inner.(driver.RowsColumnTypePrecisionScale); ok {
		return ct.ColumnTypePrecisionScale(i)
	}
	return 0, 0, false
}

func (r *sqllogRows) ColumnTypeScanType(i int) reflect.Type {
	if ct, ok := r.inner.(driver.RowsColumnTypeScanType); ok {
		return ct.ColumnTypeScanType(i)
	}
	return nil
}

// ---- Result / Tx ----

type sqllogResult struct {
	inner driver.Result
}

func (r *sqllogResult) LastInsertId() (int64, error) {
	return r.inner.LastInsertId()
}

func (r *sqllogResult) RowsAffected() (int64, error) {
	return r.inner.RowsAffected()
}

type sqllogTx struct {
	inner driver.Tx
}

func (t *sqllogTx) Commit() error {
	start := time.Now()
	err := t.inner.Commit()
	logSQL("COMMIT", "", nil, time.Since(start), -1, err)
	return err
}

func (t *sqllogTx) Rollback() error {
	start := time.Now()
	err := t.inner.Rollback()
	logSQL("ROLLBACK", "", nil, time.Since(start), -1, err)
	return err
}

// ---- helpers ----

// rowsAffected 取 RowsAffected 用于日志，取不到（res 为 nil 或驱动不支持）返回 -1 表示不输出
func rowsAffected(res driver.Result) int64 {
	if res == nil {
		return -1
	}
	n, err := res.RowsAffected()
	if err != nil {
		return -1
	}
	return n
}

// logSQL 输出一条 SQL 日志。rows < 0 表示不输出该字段。
// 先 Enabled 再 formatArgs：级别关闭时参数格式化零成本（slog 参数在关闭时仍会求值）。
func logSQL(op, query string, args []driver.NamedValue, cost time.Duration, rows int64, err error) {
	logger := slog.Default()
	if !logger.Enabled(context.Background(), slog.LevelDebug) {
		return
	}
	attrs := []any{"op", op}
	if query != "" {
		attrs = append(attrs, "query", truncate(query, maxArgLen))
	}
	if s := formatArgs(args); s != "" {
		attrs = append(attrs, "args", s)
	}
	if rows >= 0 {
		attrs = append(attrs, "rows", rows)
	}
	attrs = append(attrs, "cost", cost.String())
	if err != nil {
		attrs = append(attrs, "err", err)
	}
	logger.Debug("[SQL]", attrs...)
}

// valuesToNamed Stmt.Exec/Query（无 ctx 版）的 []driver.Value 转为 NamedValue 复用 formatArgs
func valuesToNamed(vals []driver.Value) []driver.NamedValue {
	if len(vals) == 0 {
		return nil
	}
	nvs := make([]driver.NamedValue, len(vals))
	for i, v := range vals {
		nvs[i] = driver.NamedValue{Value: v}
	}
	return nvs
}

// namedToValues 反向转换：Stmt ctx 版回退到无 ctx 版时使用
func namedToValues(nvs []driver.NamedValue) []driver.Value {
	if len(nvs) == 0 {
		return nil
	}
	vals := make([]driver.Value, len(nvs))
	for i, nv := range nvs {
		vals[i] = nv.Value
	}
	return vals
}

// formatArgs 参数列表格式化为可读字符串，如 [42, group, NULL]
func formatArgs(args []driver.NamedValue) string {
	if len(args) == 0 {
		return ""
	}
	parts := make([]string, len(args))
	for i, a := range args {
		parts[i] = formatValue(a.Value)
	}
	return "[" + strings.Join(parts, ", ") + "]"
}

// formatValue 单参数格式化：nil→NULL、[]byte→字符串（避免打出字节数组）、time 按库内 DATETIME 展示
func formatValue(v driver.Value) string {
	switch val := v.(type) {
	case nil:
		return "NULL"
	case []byte:
		return truncate(string(val), maxArgLen)
	case time.Time:
		return val.Format(time.DateTime)
	default:
		return truncate(fmt.Sprintf("%v", val), maxArgLen)
	}
}

// truncate 超长内容截断加省略号
func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "...(truncated)"
}

// 编译期断言：防止手滑漏实现可选接口（丢失不报错、只退化，这是第一道防线）
var (
	_ driver.Connector                             = (*sqllogConnector)(nil)
	_ driver.Conn                                  = (*sqllogConn)(nil)
	_ driver.ConnPrepareContext                    = (*sqllogConn)(nil)
	_ driver.ExecerContext                         = (*sqllogConn)(nil)
	_ driver.QueryerContext                        = (*sqllogConn)(nil)
	_ driver.ConnBeginTx                           = (*sqllogConn)(nil)
	_ driver.Pinger                                = (*sqllogConn)(nil)
	_ driver.SessionResetter                       = (*sqllogConn)(nil)
	_ driver.Validator                             = (*sqllogConn)(nil)
	_ driver.NamedValueChecker                     = (*sqllogConn)(nil)
	_ driver.Stmt                                  = (*sqllogStmt)(nil)
	_ driver.StmtExecContext                       = (*sqllogStmt)(nil)
	_ driver.StmtQueryContext                      = (*sqllogStmt)(nil)
	_ driver.Rows                                  = (*sqllogRows)(nil)
	_ driver.RowsNextResultSet                     = (*sqllogRows)(nil)
	_ driver.RowsColumnTypeDatabaseTypeName        = (*sqllogRows)(nil)
	_ driver.RowsColumnTypeNullable                = (*sqllogRows)(nil)
	_ driver.RowsColumnTypePrecisionScale          = (*sqllogRows)(nil)
	_ driver.RowsColumnTypeScanType                = (*sqllogRows)(nil)
	_ driver.Result                                = (*sqllogResult)(nil)
	_ driver.Tx                                    = (*sqllogTx)(nil)
)
