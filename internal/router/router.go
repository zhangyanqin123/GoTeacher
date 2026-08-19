package router

import (
	"database/sql"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"

	"handicap-service/internal/config"
	"handicap-service/internal/handler"
	"handicap-service/internal/repository"
	"handicap-service/internal/service"
)

// New 组装依赖（repo → service → handler）并注册路由。
// 鉴权：JWT + Redis 白名单（见 PLAN-auth.md），除 login 与 swagger 外全部挂 Auth 中间件。
func New(db *sql.DB, rdb *redis.Client, cfg *config.Config) *gin.Engine {
	r := gin.Default()
	r.Use(CORS())

	// Swagger 文档（docs 包由 swag init 生成，见 cmd/server/main.go 头注释）；公开，不挂鉴权
	r.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))

	repo := repository.New(db)
	svc := service.New(repo, rdb, cfg.JWTSecret, time.Duration(cfg.JWTTTLHours)*time.Hour)
	th := handler.NewTeacher(svc)
	rh := handler.NewResign(svc)
	dh := handler.NewDiagnose(svc)
	ah := handler.NewAuth(svc)

	// 鉴权公开接口（login 签发 token；logout/getinfo 需登录态放 authed 组）
	r.POST("/api/v1/login", ah.Login)

	authed := r.Group("/api/v1", Auth(svc))
	authed.POST("/logout", ah.Logout)
	authed.GET("/getinfo", ah.GetInfo)

	// 老师管理（路径与前端 teacher.js 注释里的 URL 完全一致）
	dxsf := r.Group("/api/v1/dxsf", Auth(svc))
	dxsf.POST("/teacher/list", th.List)
	dxsf.GET("/teacher/options", th.Options)
	dxsf.GET("/teacher/detail", th.Detail)
	dxsf.POST("/teacher/edit", th.Update)
	dxsf.GET("/teacher/bind/salesman/list", th.SalesList)
	dxsf.GET("/teacher/bind/salesman/users", th.BoundUserIds)
	dxsf.POST("/teacher/bind/salesman", th.Bind)

	// 离职转移（路径与前端 resign.js 注释里的 URL 完全一致）
	dxsf.GET("/resign/list", rh.List)
	dxsf.POST("/resign/add", rh.Add)

	// 诊股记录（路径与前端 diagnose.js 注释里的 URL 完全一致）
	diag := r.Group("/api/v1/dxsf/diagnose", Auth(svc))
	diag.GET("/list", dh.List)
	diag.GET("/detail", dh.Detail)
	diag.POST("/submitReport", dh.SubmitReport)
	diag.POST("/audit", dh.Audit)
	return r
}
