package database

import (
	"bytes"
	"context"
	"database/sql/driver"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"
)

// sqllog 包装器单测：fake 桩只实现被测路径方法，验证（a）日志包含 SQL 原文/参数/行数/耗时
// （b）调用正确委托到内层（c）ErrSkip 透传且不打日志（防同条 SQL 双日志）
// （d）可选接口委托（防连接池静默退化）。不依赖真实 MySQL。

// ---- fake 桩 ----

type fakeResult struct{ rows int64 }

func (f *fakeResult) LastInsertId() (int64, error) { return 0, nil }
func (f *fakeResult) RowsAffected() (int64, error) { return f.rows, nil }

type fakeRows struct {
	cols []string
	n    int // 总行数
	i    int // 已取行数
}

func (f *fakeRows) Columns() []string { return f.cols }
func (f *fakeRows) Close() error      { return nil }
func (f *fakeRows) Next(dest []driver.Value) error {
	if f.i >= f.n {
		return driver.ErrSkip
	}
	dest[0] = int64(f.i)
	f.i++
	return nil
}

type fakeStmt struct {
	conn *fakeConn
}

func (f *fakeStmt) Close() error  { return nil }
func (f *fakeStmt) NumInput() int { return -1 }
func (f *fakeStmt) Exec(args []driver.Value) (driver.Result, error) {
	return f.conn.execResult, nil
}
func (f *fakeStmt) Query(args []driver.Value) (driver.Rows, error) {
	return &fakeRows{cols: []string{"id"}, n: 2}, nil
}
func (f *fakeStmt) ExecContext(ctx context.Context, args []driver.NamedValue) (driver.Result, error) {
	return f.conn.execResult, nil
}

type fakeTx struct{}

func (f *fakeTx) Commit() error   { return nil }
func (f *fakeTx) Rollback() error { return nil }

// fakeConn 内嵌 nil driver.Conn，只覆写被测方法；可选接口按需开启
type fakeConn struct {
	driver.Conn // embed nil

	execQ string
	execA []driver.NamedValue
	execN int
	qryQ  string
	qryN  int
	prepN int

	execErr error
	skip    bool // true 时 Exec/Query 返回 driver.ErrSkip

	execResult driver.Result

	resetN int
	validN int
	pingN  int
	checkN int
}

func (f *fakeConn) Prepare(query string) (driver.Stmt, error) {
	f.prepN++
	return &fakeStmt{conn: f}, nil
}
func (f *fakeConn) Close() error              { return nil }
func (f *fakeConn) Begin() (driver.Tx, error) { return &fakeTx{}, nil }

func (f *fakeConn) ExecContext(ctx context.Context, q string, a []driver.NamedValue) (driver.Result, error) {
	f.execN++
	f.execQ, f.execA = q, a
	if f.skip {
		return nil, driver.ErrSkip
	}
	return f.execResult, f.execErr
}

func (f *fakeConn) QueryContext(ctx context.Context, q string, a []driver.NamedValue) (driver.Rows, error) {
	f.qryN++
	f.qryQ = q
	if f.skip {
		return nil, driver.ErrSkip
	}
	return &fakeRows{cols: []string{"id"}, n: 2}, nil
}

// 可选接口（用于 TestOptionalInterfacesDelegated 计数）
func (f *fakeConn) ResetSession(ctx context.Context) error { f.resetN++; return nil }
func (f *fakeConn) IsValid() bool                          { f.validN++; return true }
func (f *fakeConn) Ping(ctx context.Context) error         { f.pingN++; return nil }
func (f *fakeConn) CheckNamedValue(nv *driver.NamedValue) error {
	f.checkN++
	return nil
}

type fakeConnector struct{ conn driver.Conn }

func (f *fakeConnector) Connect(ctx context.Context) (driver.Conn, error) { return f.conn, nil }
func (f *fakeConnector) Driver() driver.Driver                            { return nil }

// newTestLogger 把 slog 输出重定向到 buf（debug 级别），测试结束恢复默认 logger
func newTestLogger(t *testing.T) *bytes.Buffer {
	t.Helper()
	buf := &bytes.Buffer{}
	old := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(buf, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { slog.SetDefault(old) })
	return buf
}

// wrapConn 构造被测的包装 Conn
func wrapConn(inner driver.Conn) *sqllogConn {
	return &sqllogConn{inner: inner}
}

// ---- 用例 ----

func TestExecContextLogsAndDelegates(t *testing.T) {
	buf := newTestLogger(t)
	fc := &fakeConn{execResult: &fakeResult{rows: 3}}

	res, err := wrapConn(fc).ExecContext(context.Background(),
		"UPDATE teacher SET title = ? WHERE id = ?", []driver.NamedValue{
			{Value: "首席"}, {Value: int64(7)},
		})
	if err != nil {
		t.Fatalf("ExecContext 返回错误: %v", err)
	}
	if n, _ := res.RowsAffected(); n != 3 {
		t.Fatalf("RowsAffected = %d, want 3", n)
	}

	// 委托正确性：fake 收到相同 query/args，且只调一次
	if fc.execQ != "UPDATE teacher SET title = ? WHERE id = ?" {
		t.Errorf("fake 收到 query = %q", fc.execQ)
	}
	if len(fc.execA) != 2 || fc.execA[0].Value != "首席" || fc.execA[1].Value != int64(7) {
		t.Errorf("fake 收到 args = %v", fc.execA)
	}
	if fc.execN != 1 {
		t.Errorf("内层 Exec 调用 %d 次, want 1", fc.execN)
	}

	// 日志内容：query 原文 + args + rows + cost（slog text 对含空格值加引号，故只匹配值本身）
	for _, want := range []string{"op=exec", "UPDATE teacher SET title", "[首席, 7]", "rows=3", "cost="} {
		if !strings.Contains(buf.String(), want) {
			t.Errorf("日志缺少 %q，实际: %s", want, buf.String())
		}
	}
}

func TestQueryContextLogsAndDelegates(t *testing.T) {
	buf := newTestLogger(t)
	fc := &fakeConn{}

	rows, err := wrapConn(fc).QueryContext(context.Background(),
		"SELECT id FROM teacher WHERE dept = ?", []driver.NamedValue{{Value: "投顾部"}})
	if err != nil {
		t.Fatalf("QueryContext 返回错误: %v", err)
	}
	if fc.qryQ != "SELECT id FROM teacher WHERE dept = ?" || fc.qryN != 1 {
		t.Errorf("委托异常: q=%q n=%d", fc.qryQ, fc.qryN)
	}

	// 返回的 Rows 可正常取数
	cols := rows.Columns()
	if len(cols) != 1 || cols[0] != "id" {
		t.Fatalf("Columns() = %v", cols)
	}
	dest := make([]driver.Value, 1)
	if err := rows.Next(dest); err != nil {
		t.Fatalf("Next 出错: %v", err)
	}

	if !strings.Contains(buf.String(), "op=query") || !strings.Contains(buf.String(), "SELECT id FROM teacher") {
		t.Errorf("日志缺少 query 信息: %s", buf.String())
	}
	// 取完全部行后 Close 应打 op=rows（行数日志）
	if err := rows.Close(); err != nil {
		t.Fatalf("Close 出错: %v", err)
	}
	if !strings.Contains(buf.String(), "op=rows") {
		t.Errorf("Rows.Close 未打行数日志: %s", buf.String())
	}
}

func TestErrSkipNotLogged(t *testing.T) {
	buf := newTestLogger(t)
	fc := &fakeConn{skip: true}

	_, err := wrapConn(fc).ExecContext(context.Background(), "SELECT 1", nil)
	if !errors.Is(err, driver.ErrSkip) {
		t.Fatalf("ErrSkip 未原样透传: %v", err)
	}
	_, err = wrapConn(fc).QueryContext(context.Background(), "SELECT 1", nil)
	if !errors.Is(err, driver.ErrSkip) {
		t.Fatalf("ErrSkip 未原样透传: %v", err)
	}

	if buf.Len() != 0 {
		t.Errorf("ErrSkip 不应打日志，实际: %s", buf.String())
	}
}

func TestStmtExecLogs(t *testing.T) {
	buf := newTestLogger(t)
	fc := &fakeConn{execResult: &fakeResult{rows: 1}}

	const q = "UPDATE teacher_sales SET teacher_id = ? WHERE sales_user_id = ?"
	stmt, err := wrapConn(fc).PrepareContext(context.Background(), q)
	if err != nil {
		t.Fatalf("PrepareContext 出错: %v", err)
	}
	if fc.prepN != 1 {
		t.Errorf("内层 Prepare 调用 %d 次, want 1", fc.prepN)
	}

	_, err = stmt.(driver.StmtExecContext).ExecContext(context.Background(), []driver.NamedValue{
		{Value: int64(2)}, {Value: int64(1001)},
	})
	if err != nil {
		t.Fatalf("Stmt.ExecContext 出错: %v", err)
	}

	// 日志中的 query 必须是 Prepare 时的 SQL 原文（带参 SQL 主路径）
	for _, want := range []string{"op=exec", q, "[2, 1001]", "rows=1"} {
		if !strings.Contains(buf.String(), want) {
			t.Errorf("日志缺少 %q，实际: %s", want, buf.String())
		}
	}
}

func TestTxBoundariesLogged(t *testing.T) {
	buf := newTestLogger(t)
	fc := &fakeConn{}

	tx, err := wrapConn(fc).BeginTx(context.Background(), driver.TxOptions{})
	if err != nil {
		t.Fatalf("BeginTx 出错: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit 出错: %v", err)
	}

	tx2, err := wrapConn(fc).Begin()
	if err != nil {
		t.Fatalf("Begin 出错: %v", err)
	}
	if err := tx2.Rollback(); err != nil {
		t.Fatalf("Rollback 出错: %v", err)
	}

	for _, want := range []string{"op=BEGIN", "op=COMMIT", "op=ROLLBACK"} {
		if !strings.Contains(buf.String(), want) {
			t.Errorf("日志缺少 %q，实际: %s", want, buf.String())
		}
	}
}

func TestFormatArgs(t *testing.T) {
	ts := time.Date(2026, 8, 18, 10, 30, 0, 0, time.Local)
	long := strings.Repeat("a", 600)
	cases := []struct {
		name string
		args []driver.NamedValue
		want string
	}{
		{"nil 参数", []driver.NamedValue{{Value: nil}}, "[NULL]"},
		{"字节切片转字符串", []driver.NamedValue{{Value: []byte("富文本")}}, "[富文本]"},
		{"时间格式化", []driver.NamedValue{{Value: ts}}, "[2026-08-18 10:30:00]"},
		{"多参数", []driver.NamedValue{{Value: int64(1)}, {Value: "group"}, {Value: nil}}, "[1, group, NULL]"},
		{"超长截断", []driver.NamedValue{{Value: long}}, "[" + strings.Repeat("a", 512) + "...(truncated)]"},
		{"空参数", nil, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := formatArgs(c.args); got != c.want {
				t.Errorf("formatArgs() = %q, want %q", got, c.want)
			}
		})
	}
}

func TestOptionalInterfacesDelegated(t *testing.T) {
	newTestLogger(t) // 吸收日志输出
	fc := &fakeConn{}
	wrapped := wrapConn(fc)
	ctx := context.Background()

	if err := wrapped.ResetSession(ctx); err != nil {
		t.Fatalf("ResetSession 出错: %v", err)
	}
	if !wrapped.IsValid() {
		t.Fatal("IsValid 应为 true")
	}
	if err := wrapped.Ping(ctx); err != nil {
		t.Fatalf("Ping 出错: %v", err)
	}
	if err := wrapped.CheckNamedValue(&driver.NamedValue{Value: 1}); err != nil {
		t.Fatalf("CheckNamedValue 出错: %v", err)
	}

	if fc.resetN != 1 || fc.validN != 1 || fc.pingN != 1 || fc.checkN != 1 {
		t.Errorf("可选接口委托计数异常: reset=%d valid=%d ping=%d check=%d",
			fc.resetN, fc.validN, fc.pingN, fc.checkN)
	}
}

func TestConnectorConnectLogsAndWraps(t *testing.T) {
	buf := newTestLogger(t)
	fc := &fakeConn{}
	c := NewConnector(&fakeConnector{conn: fc})

	conn, err := c.Connect(context.Background())
	if err != nil {
		t.Fatalf("Connect 出错: %v", err)
	}
	if _, ok := conn.(*sqllogConn); !ok {
		t.Fatalf("Connect 应返回包装后的 Conn，实际 %T", conn)
	}
	if !strings.Contains(buf.String(), "op=connect") {
		t.Errorf("建连日志缺失: %s", buf.String())
	}
}
