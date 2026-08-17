package database

import (
	"context"
	"database/sql"
	_ "embed"
	"fmt"
	"strings"
	"time"

	_ "github.com/go-sql-driver/mysql"

	"handicap-service/internal/config"
)

// 内嵌 SQL，随二进制发布，无需外部迁移工具
//
//go:embed schema.sql
var schemaSQL string

//
//go:embed seed.sql
var seedSQL string

//
//go:embed teacher_seed.sql
var teacherSeedSQL string

//
//go:embed resign_seed.sql
var resignSeedSQL string

//
//go:embed diagnose_seed.sql
var diagnoseSeedSQL string

// Connect 建立 MySQL 连接池并探活
func Connect(cfg *config.Config) (*sql.DB, error) {
	db, err := sql.Open("mysql", cfg.DSN)
	if err != nil {
		return nil, fmt.Errorf("open mysql: %w", err)
	}

	db.SetMaxOpenConns(20)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(time.Hour)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("ping mysql %s:%s %s, err: %w", cfg.DBHost, cfg.DBPort, cfg.DBName, err)
	}
	return db, nil
}

// Migrate 执行建表 DDL（CREATE TABLE IF NOT EXISTS，幂等）+ 存量库列型升级 + 存量库列清理（teacher_resign 移除 friend_count）
func Migrate(db *sql.DB) error {
	if err := execStatements(db, schemaSQL); err != nil {
		return fmt.Errorf("migrate: %w", err)
	}
	if err := migrateDiagnoseRemark(db); err != nil {
		return fmt.Errorf("migrate diagnose remark: %w", err)
	}
	if err := migrateTeacherResign(db); err != nil {
		return fmt.Errorf("migrate teacher_resign columns: %w", err)
	}
	return nil
}

// migrateDiagnoseRemark diagnose.remark VARCHAR(200) → TEXT（富文本化二期）。
// CREATE TABLE IF NOT EXISTS 不会改已建表，存量库靠此处幂等升级：
// INFORMATION_SCHEMA 查列类型，已是 text 直接跳过；列不存在（表未建）也跳过。
// VARCHAR→TEXT 是放宽转换，MODIFY 保留数据；8.0.11 不支持 TEXT 表达式默认值，故不带 DEFAULT。
func migrateDiagnoseRemark(db *sql.DB) error {
	var dataType string
	err := db.QueryRow(
		"SELECT DATA_TYPE FROM INFORMATION_SCHEMA.COLUMNS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'diagnose' AND COLUMN_NAME = 'remark'",
	).Scan(&dataType)
	if err == sql.ErrNoRows {
		return nil
	}
	if err != nil {
		return fmt.Errorf("check diagnose.remark type: %w", err)
	}
	if dataType == "text" {
		return nil // 已升级，幂等出口
	}
	// 与 schema.sql 中的列定义逐字一致，防两处漂移
	const alter = "ALTER TABLE diagnose MODIFY COLUMN remark TEXT NOT NULL COMMENT '用户备注（富文本 HTML，净化后存储）'"
	if _, err := db.Exec(alter); err != nil {
		return fmt.Errorf("alter diagnose.remark: %w", err)
	}
	return nil
}

// migrateTeacherResign teacher_resign 列清理（移除好友概念，2026-08）。
// CREATE TABLE IF NOT EXISTS 不改已建表，存量库靠此处幂等升级：
// 1) DROP COLUMN friend_count（新库无此列，COUNT=0 跳过；DROP 不可逆，该列已废弃）
// 2) MODIFY group_count 注释为新语义（纯元数据变更，与 schema.sql 逐字一致防漂移）
// 3) 清洗 transfer_content 残留 'friend'（首跑后 WHERE 永不命中，不触发 updated_at）
func migrateTeacherResign(db *sql.DB) error {
	var cnt int
	if err := db.QueryRow(
		"SELECT COUNT(*) FROM INFORMATION_SCHEMA.COLUMNS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'teacher_resign' AND COLUMN_NAME = 'friend_count'",
	).Scan(&cnt); err != nil {
		return fmt.Errorf("check teacher_resign.friend_count: %w", err)
	}
	if cnt > 0 {
		if _, err := db.Exec("ALTER TABLE teacher_resign DROP COLUMN friend_count"); err != nil {
			return fmt.Errorf("drop teacher_resign.friend_count: %w", err)
		}
	}

	var comment string
	err := db.QueryRow(
		"SELECT COLUMN_COMMENT FROM INFORMATION_SCHEMA.COLUMNS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'teacher_resign' AND COLUMN_NAME = 'group_count'",
	).Scan(&comment)
	if err == sql.ErrNoRows {
		return nil // 表未建（防御出口，正常不会走到）
	}
	if err != nil {
		return fmt.Errorf("check teacher_resign.group_count comment: %w", err)
	}
	// 与 schema.sql 中的列定义逐字一致，防两处漂移
	const want = "转移客户群数（=原老师绑定业务员数，后端计算入库）"
	if comment != want {
		const alter = "ALTER TABLE teacher_resign MODIFY COLUMN group_count INT NOT NULL DEFAULT 0 COMMENT '" + want + "'"
		if _, err := db.Exec(alter); err != nil {
			return fmt.Errorf("alter teacher_resign.group_count comment: %w", err)
		}
	}

	if _, err := db.Exec("UPDATE teacher_resign SET transfer_content = 'group' WHERE FIND_IN_SET('friend', transfer_content)"); err != nil {
		return fmt.Errorf("normalize teacher_resign.transfer_content: %w", err)
	}
	return nil
}

// Seed 各表为空时才插入种子数据（幂等：已有数据则跳过）
func Seed(db *sql.DB) error {
	if err := seedHouseUpDown(db); err != nil {
		return err
	}
	if err := seedTeacher(db); err != nil {
		return err
	}
	if err := seedResign(db); err != nil {
		return err
	}
	return seedDiagnose(db)
}

// seedHouseUpDown house_up_down_stats 空表种子
func seedHouseUpDown(db *sql.DB) error {
	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM house_up_down_stats").Scan(&count); err != nil {
		return fmt.Errorf("seed count: %w", err)
	}
	return seedIfEmpty(db, count, seedSQL)
}

// seedTeacher teacher 空表种子（teacher/sales_user/teacher_sales 一次事务写入）
func seedTeacher(db *sql.DB) error {
	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM teacher").Scan(&count); err != nil {
		return fmt.Errorf("seed teacher count: %w", err)
	}
	return seedIfEmpty(db, count, teacherSeedSQL)
}

// seedResign teacher_resign 空表种子（照抄前端 resign.js mock，保证联调数据一致）
func seedResign(db *sql.DB) error {
	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM teacher_resign").Scan(&count); err != nil {
		return fmt.Errorf("seed resign count: %w", err)
	}
	return seedIfEmpty(db, count, resignSeedSQL)
}

// seedDiagnose diagnose 空表种子（照抄前端 diagnose.js mock，主表+日志一次事务写入）
func seedDiagnose(db *sql.DB) error {
	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM diagnose").Scan(&count); err != nil {
		return fmt.Errorf("seed diagnose count: %w", err)
	}
	return seedIfEmpty(db, count, diagnoseSeedSQL)
}

// seedIfEmpty count 为 0 时在事务里逐条执行 seedSQL
func seedIfEmpty(db *sql.DB, count int, query string) error {
	if count > 0 {
		return nil
	}

	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("seed begin: %w", err)
	}
	if err := execStatements(tx, query); err != nil {
		_ = tx.Rollback()
		return fmt.Errorf("seed exec: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("seed commit: %w", err)
	}
	return nil
}

// execer 抽象 *sql.DB 与 *sql.Tx 的 Exec，共用脚本执行逻辑
type execer interface {
	Exec(query string, args ...any) (sql.Result, error)
}

// execStatements 逐条执行内嵌 SQL 脚本。
// go-sql-driver 默认一次 Exec 只允许一条语句（DSN 开 multiStatements 可解，
// 但该开关作用于整个连接池；这里改为应用层切分，能力收敛在建表/种子脚本内）。
func execStatements(e execer, script string) error {
	for _, stmt := range splitStatements(script) {
		if _, err := e.Exec(stmt); err != nil {
			return fmt.Errorf("exec %q: %w", firstLine(stmt), err)
		}
	}
	return nil
}

// splitStatements 按分号切分脚本，忽略单引号字符串内的分号与空白片段。
func splitStatements(script string) []string {
	var stmts []string
	var sb strings.Builder
	inString := false

	for i := 0; i < len(script); i++ {
		ch := script[i]
		switch {
		case ch == '\\' && inString && i+1 < len(script): // \' 转义引号
			sb.WriteByte(ch)
			sb.WriteByte(script[i+1])
			i++
		case ch == '\'':
			inString = !inString
			sb.WriteByte(ch)
		case ch == ';' && !inString:
			if s := strings.TrimSpace(sb.String()); s != "" {
				stmts = append(stmts, s)
			}
			sb.Reset()
		default:
			sb.WriteByte(ch)
		}
	}
	if s := strings.TrimSpace(sb.String()); s != "" {
		stmts = append(stmts, s)
	}
	return stmts
}

// firstLine 取语句首行（注释或 CREATE/INSERT 开头），用于报错定位
func firstLine(stmt string) string {
	if i := strings.IndexByte(stmt, '\n'); i > 0 {
		return stmt[:i]
	}
	return stmt
}
