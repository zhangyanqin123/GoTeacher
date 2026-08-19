package service

import "golang.org/x/crypto/bcrypt"

// bcryptGenerate 生成密码哈希（与 bcryptCompare 对称，供种子/测试使用）
func bcryptGenerate(plaintext string) (string, error) {
	b, err := bcrypt.GenerateFromPassword([]byte(plaintext), bcrypt.DefaultCost)
	return string(b), err
}

// bcryptCompare 密码比对。单独抽函数：service/auth.go 主流程保持线性可读，
// 同时给 auth_test.go 一个不依赖 DB 的单测切口。
func bcryptCompare(hash, plaintext string) error {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(plaintext))
}
