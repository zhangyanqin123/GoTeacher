package model

import (
	"fmt"
	"strconv"
)

// FlexInt64 宽容数值载体：JSON 里数字、数字字符串（"80"）、空串、null 都能收。
//
// 背景：前端筛选字段类型不统一——el-input 产出字符串 "5"、部门树来自 admin
// 真实接口（Java Long 序列化为 "80"）、重置时置空串 ''、初始又是 null，
// 严格 *int64 绑定会 400。UnmarshalJSON 在解析点归一为整数；
// 空串/null 归一为未设置（Ptr 为 nil），非法串仍报错（400 提示格式问题）。
type FlexInt64 struct {
	Val int64
	Set bool // 收到有效值（区分"未传"与"传 0"）
}

// UnmarshalJSON 实现 json.Unmarshaler
func (f *FlexInt64) UnmarshalJSON(data []byte) error {
	s := string(data)
	if s == "null" || s == `""` {
		return nil // 保持零值：未设置
	}
	// 去掉 JSON 字符串引号，数字与字符串统一走整数解析
	if len(s) >= 2 && s[0] == '"' {
		s = s[1 : len(s)-1]
	}
	v, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return fmt.Errorf("FlexInt64: %q is not an integer", s)
	}
	f.Val, f.Set = v, true
	return nil
}

// Ptr 返回 *int64 语义视图：未设置返回 nil（不过滤），设置返回值指针。
// 调用方拿到的指针指向自身字段副本生命周期安全的副本。
func (f FlexInt64) Ptr() *int64 {
	if !f.Set {
		return nil
	}
	v := f.Val
	return &v
}
