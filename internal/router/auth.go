package router

import (
	"errors"
	"log/slog"
	"strings"

	"github.com/gin-gonic/gin"

	"handicap-service/internal/model"
	"handicap-service/internal/response"
	"handicap-service/internal/service"
)

// bearerPrefix Authorization 头固定前缀
const bearerPrefix = "Bearer "

// Auth JWT 鉴权中间件（薄壳：解析头 + 调 service.VerifyAccessToken，见 PLAN-auth.md）。
//
// 校验失败统一 401 + ErrUnauthorized 文案（不区分缺头/坏 token/验签失败/白名单不命中，
// 防探测），具体原因打 debug 日志；Redis 故障映 500（fail-closed：校验不了就不放行）。
// 通过后把用户信息写入 context（model.CtxKeyUserID/CtxKeyUsername）供 handler 读取。
func Auth(svc *service.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		raw := c.GetHeader("Authorization")
		if !strings.HasPrefix(raw, bearerPrefix) {
			slog.Debug("auth: missing or malformed Authorization header")
			response.Fail(c, 401, 401, service.ErrUnauthorized.Error())
			c.Abort()
			return
		}
		token := strings.TrimPrefix(raw, bearerPrefix)

		claims, err := svc.VerifyAccessToken(c.Request.Context(), token)
		switch {
		case err == nil:
			c.Set(model.CtxKeyUserID, claims.UserID)
			c.Set(model.CtxKeyUsername, claims.Username)
			c.Next()
		case errors.Is(err, service.ErrUnauthorized):
			// 注意：err 非 nil 时 claims 恒为 nil（验签失败/白名单不命中都不返回半成品 claims）
			slog.Debug("auth: token rejected")
			response.Fail(c, 401, 401, service.ErrUnauthorized.Error())
			c.Abort()
		default: // Redis 故障等基础设施错误
			slog.Error("auth: verify token failed", "err", err)
			response.Fail(c, 500, 500, "服务器内部错误")
			c.Abort()
		}
	}
}
