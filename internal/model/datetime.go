package model

import (
	"fmt"
	"time"
)

// dateTimeLayout DATETIME 的字符串形态（前端直接展示，不能是 RFC3339）
const dateTimeLayout = "2006-01-02 15:04:05"

// DateTimeString DATETIME 列的字符串载体。
//
// 背景：DSN 开了 ParseTime 后驱动返回 time.Time，database/sql 扫进普通 string
// 会按 RFC3339 格式化（"2025-01-15T09:30:00+08:00"），而前端直接展示、不转换，
// 约定是 "2025-01-15 09:30:00"。实现 sql.Scanner 在扫描点做一次格式化，
// JSON 序列化仍是普通字符串。
type DateTimeString string

// Scan 实现 sql.Scanner
func (d *DateTimeString) Scan(src any) error {
	switch v := src.(type) {
	case nil:
		*d = ""
	case time.Time:
		*d = DateTimeString(v.Format(dateTimeLayout))
	case []byte:
		*d = DateTimeString(v)
	case string:
		*d = DateTimeString(v)
	default:
		return fmt.Errorf("DateTimeString.Scan: unsupported src type %T", src)
	}
	return nil
}
