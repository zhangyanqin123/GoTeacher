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

// Migrate 执行建表 DDL（CREATE TABLE IF NOT EXISTS，幂等）
func Migrate(db *sql.DB) error {
	if err := execStatements(db, schemaSQL); err != nil {
		return fmt.Errorf("migrate: %w", err)
	}
	return nil
}

// Seed 各表为空时才插入种子数据（幂等：已有数据则跳过）
func Seed(db *sql.DB) error {
	if err := seedHouseUpDown(db); err != nil {
		return err
	}
	return seedTeacher(db)
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
