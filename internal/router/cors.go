package router

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// CORS 跨域中间件（标准库实现，不引入 gin-contrib/cors）。
//
// 前端 gyz-admin 开发环境直连本服务（.env.development 的 VUE_APP_BASE_API
// 指向 http://localhost:8080），Origin 为 localhost 任意端口，故反射回显。
// 本项目无 Cookie 鉴权，不开 Allow-Credentials（开启时浏览器要求
// Allow-Origin 不能是 *，回显 Origin 已覆盖该场景）。
func CORS() gin.HandlerFunc {
	return func(c *gin.Context) {
		if origin := c.GetHeader("Origin"); origin != "" {
			c.Header("Access-Control-Allow-Origin", origin)
			c.Header("Vary", "Origin")
			c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
			c.Header("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Requested-With")
			c.Header("Access-Control-Max-Age", "86400") // 预检结果缓存一天，减少 OPTIONS
		}

		// 预检请求直接放行，不进业务 handler
		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	}
}
