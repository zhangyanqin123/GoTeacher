package service

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const testSecret = "test-secret-32-bytes-xxxxxxxxxxxx"

// newTestService 不依赖 DB/Redis 的最小构造（Login 走不到，仅测签发/验签纯函数路径）
func newTestService(ttl time.Duration) *Service {
	return New(nil, nil, testSecret, ttl)
}

// signTestToken 用测试密钥签一个白名单外的裸 token（不走 Redis）
func signTestToken(s *Service, claims *AccessClaims) string {
	t, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(s.jwtSecret)
	if err != nil {
		panic(err)
	}
	return t
}

// TestSignAndParseClaims 签发 → 验签：claims 字段无损往返（白名单命中路径需 Redis，见 router 测试）
func TestSignAndParseClaims(t *testing.T) {
	s := newTestService(time.Hour)

	now := time.Now()
	claims := &AccessClaims{
		UserID:   42,
		Username: "admin",
		RegisteredClaims: jwt.RegisteredClaims{
			ID:        newJTI(),
			ExpiresAt: jwt.NewNumericDate(now.Add(time.Hour)),
			IssuedAt:  jwt.NewNumericDate(now),
		},
	}
	token := signTestToken(s, claims)

	got := &AccessClaims{}
	_, err := jwt.ParseWithClaims(token, got, func(*jwt.Token) (any, error) {
		return s.jwtSecret, nil
	}, jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}))
	if err != nil {
		t.Fatalf("parse token: %v", err)
	}
	if got.UserID != 42 || got.Username != "admin" || got.ID != claims.ID {
		t.Errorf("claims round-trip mismatch: got userID=%d username=%q jti=%q", got.UserID, got.Username, got.ID)
	}
}

// TestVerifyTamperedPayload 篡改 payload 后验签必须失败
func TestVerifyTamperedPayload(t *testing.T) {
	s := newTestService(time.Hour)
	now := time.Now()
	claims := &AccessClaims{
		UserID:   42,
		Username: "admin",
		RegisteredClaims: jwt.RegisteredClaims{
			ID:        newJTI(),
			ExpiresAt: jwt.NewNumericDate(now.Add(time.Hour)),
			IssuedAt:  jwt.NewNumericDate(now),
		},
	}
	parts := strings.Split(signTestToken(s, claims), ".")
	parts[1] += "x" // 破坏 payload
	tampered := strings.Join(parts, ".")

	if _, err := s.VerifyAccessToken(context.Background(), tampered); err == nil {
		t.Error("tampered token should be rejected")
	}
}

// TestVerifyExpiredToken 过期 token 必须被拒（Redis 缺失时先报基础设施错误，验签错误在前面拦截）
func TestVerifyExpiredToken(t *testing.T) {
	s := newTestService(time.Hour)
	claims := &AccessClaims{
		UserID:   42,
		Username: "admin",
		RegisteredClaims: jwt.RegisteredClaims{
			ID:        newJTI(),
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(-time.Minute)), // 已过期
			IssuedAt:  jwt.NewNumericDate(time.Now().Add(-time.Hour)),
		},
	}
	if _, err := s.VerifyAccessToken(context.Background(), signTestToken(s, claims)); err == nil {
		t.Error("expired token should be rejected")
	}
}

// TestVerifyWrongSignature 不同密钥签的 token 必须被拒
func TestVerifyWrongSignature(t *testing.T) {
	s := newTestService(time.Hour)
	forged := newTestService(time.Hour)
	forged.jwtSecret = []byte("another-secret-entirely-different!!")

	claims := &AccessClaims{
		UserID:   42,
		Username: "admin",
		RegisteredClaims: jwt.RegisteredClaims{
			ID:        newJTI(),
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}
	if _, err := s.VerifyAccessToken(context.Background(), signTestToken(forged, claims)); err == nil {
		t.Error("token signed with wrong secret should be rejected")
	}
}

// TestTokenKeyFormat 白名单 key 格式：auth:token:{user_id}
func TestTokenKeyFormat(t *testing.T) {
	if got := tokenKey(42); got != "auth:token:42" {
		t.Errorf("tokenKey(42) = %q, want %q", got, "auth:token:42")
	}
}

// TestBcryptCompare 密码比对（生成→匹配/不匹配）
func TestBcryptCompare(t *testing.T) {
	hash, err := bcryptGenerate("admin123")
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if err := bcryptCompare(hash, "admin123"); err != nil {
		t.Errorf("correct password should match: %v", err)
	}
	if err := bcryptCompare(hash, "wrong"); err == nil {
		t.Error("wrong password should not match")
	}
}
