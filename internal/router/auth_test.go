package router

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/redis/go-redis/v9"

	"gyz-service/internal/model"
	"gyz-service/internal/service"
)

const testSecret = "test-secret-32-bytes-xxxxxxxxxxxx"

// newAuthTestEnv 起 miniredis + 组装只挂 Auth 的路由（业务 handler 用探针替代，
// 断言放行后 context 里的用户信息），不依赖 MySQL。
func newAuthTestEnv(t *testing.T) (*gin.Engine, *redis.Client, *service.Service) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis: %v", err)
	}
	t.Cleanup(mr.Close)

	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })

	svc := service.New(nil, rdb, testSecret, time.Hour, "", nil)

	r := gin.New()
	r.GET("/probe", Auth(svc), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"user_id":  c.GetInt64(model.CtxKeyUserID),
			"username": c.GetString(model.CtxKeyUsername),
		})
	})
	return r, rdb, svc
}

// issueToken 签一个合法 token 并（可选）写入白名单，返回裸 token
func issueToken(svc *service.Service, rdb *redis.Client, userID int64, username string, whitelisted bool, ttl time.Duration) string {
	now := time.Now()
	claims := &service.AccessClaims{
		UserID:   userID,
		Username: username,
		RegisteredClaims: jwt.RegisteredClaims{
			ID:        "test-jti",
			ExpiresAt: jwt.NewNumericDate(now.Add(ttl)),
			IssuedAt:  jwt.NewNumericDate(now),
		},
	}
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(testSecret))
	if err != nil {
		panic(err)
	}
	if whitelisted {
		if err := rdb.Set(context.Background(), "auth:token:"+itoa(userID), claims.ID, ttl).Err(); err != nil {
			panic(err)
		}
	}
	return token
}

func itoa(n int64) string {
	return string(rune('0' + n%10)) // 测试只用个位数 userID，避免引入 strconv 仅为可读性
}

// getProbe 探针请求，返回 (状态码, user_id, username)
func getProbe(r *gin.Engine, authHeader string) (int, int64, string) {
	req := httptest.NewRequest(http.MethodGet, "/probe", nil)
	if authHeader != "" {
		req.Header.Set("Authorization", authHeader)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	var body struct {
		UserID   int64  `json:"user_id"`
		Username string `json:"username"`
		Msg      string `json:"msg"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &body)
	return w.Code, body.UserID, body.Username
}

// TestAuthMissingHeader 无 Authorization 头 → 401
func TestAuthMissingHeader(t *testing.T) {
	r, _, _ := newAuthTestEnv(t)
	code, _, _ := getProbe(r, "")
	if code != 401 {
		t.Errorf("no header: code=%d, want 401", code)
	}
}

// TestAuthMalformedHeader 非 Bearer 前缀 → 401
func TestAuthMalformedHeader(t *testing.T) {
	r, _, _ := newAuthTestEnv(t)
	if code, _, _ := getProbe(r, "Basic dXNlcjpwYXNz"); code != 401 {
		t.Errorf("basic header: code=%d, want 401", code)
	}
}

// TestAuthGarbageToken 乱串 token → 401
func TestAuthGarbageToken(t *testing.T) {
	r, _, _ := newAuthTestEnv(t)
	if code, _, _ := getProbe(r, "Bearer not-a-jwt"); code != 401 {
		t.Errorf("garbage token: code=%d, want 401", code)
	}
}

// TestAuthForgedSignature 错误密钥签发 → 401
func TestAuthForgedSignature(t *testing.T) {
	r, rdb, _ := newAuthTestEnv(t)
	// 用错误密钥签，且白名单写入（隔离「验签失败」这一层）
	forgedClaims := &service.AccessClaims{
		UserID:   7,
		Username: "hacker",
		RegisteredClaims: jwt.RegisteredClaims{
			ID:        "forged-jti",
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}
	forged, err := jwt.NewWithClaims(jwt.SigningMethodHS256, forgedClaims).SignedString([]byte("wrong-secret"))
	if err != nil {
		t.Fatal(err)
	}
	_ = rdb.Set(context.Background(), "auth:token:7", "forged-jti", time.Hour)

	if code, _, _ := getProbe(r, "Bearer "+forged); code != 401 {
		t.Errorf("forged signature: code=%d, want 401", code)
	}
}

// TestAuthNotWhitelisted 合法签发但白名单无记录（登出/被踢）→ 401
func TestAuthNotWhitelisted(t *testing.T) {
	r, _, svc := newAuthTestEnv(t)
	token := issueToken(svc, nil, 1, "admin", false, time.Hour)
	if code, _, _ := getProbe(r, "Bearer "+token); code != 401 {
		t.Errorf("not whitelisted: code=%d, want 401", code)
	}
}

// TestAuthStaleDevice 白名单已被新登录覆盖（jti 不符）→ 401
func TestAuthStaleDevice(t *testing.T) {
	r, rdb, svc := newAuthTestEnv(t)
	// 用户 1 已重新登录，白名单 jti 变成 new-jti；旧 token 仍是 test-jti
	_ = rdb.Set(context.Background(), "auth:token:1", "new-jti", time.Hour)
	token := issueToken(svc, rdb, 1, "admin", false, time.Hour) // 白名单已被覆盖，test-jti 过期

	if code, _, _ := getProbe(r, "Bearer "+token); code != 401 {
		t.Errorf("stale device token: code=%d, want 401", code)
	}
}

// TestAuthPassThrough 白名单命中 → 放行且 context 注入用户信息
func TestAuthPassThrough(t *testing.T) {
	r, rdb, svc := newAuthTestEnv(t)
	token := issueToken(svc, rdb, 1, "admin", true, time.Hour)

	code, userID, username := getProbe(r, "Bearer "+token)
	if code != 200 || userID != 1 || username != "admin" {
		t.Errorf("whitelisted: code=%d userID=%d username=%q, want 200/1/admin", code, userID, username)
	}
}

// TestAuthExpiredButWhitelisted token 过期但白名单仍在 → 401（验签先于白名单）
func TestAuthExpiredButWhitelisted(t *testing.T) {
	r, rdb, svc := newAuthTestEnv(t)
	token := issueToken(svc, rdb, 1, "admin", true, -time.Minute)

	if code, _, _ := getProbe(r, "Bearer "+token); code != 401 {
		t.Errorf("expired but whitelisted: code=%d, want 401", code)
	}
}

// TestAuthLogoutDelWhitelist logout 后同 token → 401（白名单互踢闭环）
func TestAuthLogoutDelWhitelist(t *testing.T) {
	r, rdb, svc := newAuthTestEnv(t)
	token := issueToken(svc, rdb, 1, "admin", true, time.Hour)

	if code, _, _ := getProbe(r, "Bearer "+token); code != 200 {
		t.Fatalf("before logout: code=%d, want 200", code)
	}
	if err := svc.Logout(context.Background(), &service.AccessClaims{UserID: 1}); err != nil {
		t.Fatalf("logout: %v", err)
	}
	if code, _, _ := getProbe(r, "Bearer "+token); code != 401 {
		t.Errorf("after logout: code=%d, want 401", code)
	}
}

// 兜底：确保没有测试因 gin debug 日志噪音失败（保持输出干净）
func TestMain(m *testing.M) {
	gin.SetMode(gin.TestMode)
	m.Run()
}

var _ = strings.TrimSpace // 保留 import 以防裁剪（无实义）
