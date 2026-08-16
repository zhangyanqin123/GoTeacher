package model

import (
	"database/sql/driver"
	"fmt"
	"strings"
)

// StringSlice 逗号分隔 VARCHAR 列的数组载体。
//
// 背景：transfer_content 库里存 'group,friend'，接口需输出 ["group","friend"]。
// Scan 在扫描点按逗号切分；Value 供写库时拼回字符串（空数组存 ”）。
type StringSlice []string

// Scan 实现 sql.Scanner：'group,friend' → ["group","friend"]
func (s *StringSlice) Scan(src any) error {
	switch v := src.(type) {
	case nil:
		*s = nil
	case []byte:
		*s = splitComma(string(v))
	case string:
		*s = splitComma(v)
	default:
		return fmt.Errorf("StringSlice.Scan: unsupported src type %T", src)
	}
	return nil
}

// Value 实现 driver.Valuer：["group","friend"] → 'group,friend'
func (s StringSlice) Value() (driver.Value, error) {
	return strings.Join(s, ","), nil
}

// splitComma 空串返回空切片（与列 NOT NULL DEFAULT ” 匹配）
func splitComma(s string) StringSlice {
	if s == "" {
		return StringSlice{}
	}
	return strings.Split(s, ",")
}
