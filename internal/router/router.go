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
	"handicap-service/internal/mq"
	"handicap-service/internal/repository"
	"handicap-service/internal/response"
	"handicap-service/internal/service"
)

// New 组装依赖（repo → service → handler）并注册路由。
// 鉴权：JWT + Redis 白名单（见 PLAN-auth.md），除 login、swagger 与 /guyuzhoudb/live/**（小鹅通透传，
// 公开，见 PLAN-live.md）外全部挂 Auth 中间件。
// publisher：订单事件 order.created 的发布端（连接由 main 持有，见 PLAN-order.md）。
func New(db *sql.DB, rdb *redis.Client, cfg *config.Config, publisher mq.Publisher) *gin.Engine {
	r := gin.Default()
	r.Use(CORS())

	// Swagger 文档（docs 包由 swag init 生成，见 cmd/server/main.go 头注释）；公开，不挂鉴权
	r.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))

	// 存活探针（容器 healthcheck 用）：公开免鉴权，纯进程探活不 ping 依赖——
	// 中间件挂了重启 server 容器无意义且中断在途请求；依赖可达性由启动 fail-fast 保证，见 PLAN-docker.md
	r.GET("/health", func(c *gin.Context) { response.OK(c, nil) })

	repo := repository.New(db)
	svc := service.New(repo, rdb, cfg.JWTSecret, time.Duration(cfg.JWTTTLHours)*time.Hour, cfg.XiaoeAPIBase, publisher)
	th := handler.NewTeacher(svc)
	rh := handler.NewResign(svc)
	dh := handler.NewDiagnose(svc)
	ah := handler.NewAuth(svc)
	auh := handler.NewAdminUser(svc)
	lh := handler.NewLive(svc)
	oh := handler.NewOrder(svc)
	abh := handler.NewAbModule(svc)

	// 鉴权公开接口（login 签发 token；logout/getinfo 需登录态放 authed 组）
	r.POST("/api/v1/login", ah.Login)

	// 直播（小鹅通透传，mofang C 端，见 PLAN-live.md）：公开不挂 Auth——
	// mofang 是另一 token 体系本服务验不了，/guyuzhoudb 前缀独立于 /api/v1，凭证即入参 access_token 由小鹅通校验
	r.GET("/guyuzhoudb/live/get_login_url", lh.GetXeLoginURL)
	r.GET("/guyuzhoudb/live/register_user", lh.RegisterXeUser)

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
	dxsf.POST("/teacher/resign/list", rh.List)
	dxsf.POST("/teacher/resign/add", rh.Add)

	// 诊股记录（路径与前端 diagnose.js 注释里的 URL 完全一致）
	diag := r.Group("/api/v1/dxsf/teacher/diagnose", Auth(svc))
	diag.POST("/list", dh.List)
	diag.GET("/detail", dh.Detail)
	diag.POST("/submit/report", dh.SubmitReport)
	diag.POST("/audit", dh.Audit)

	// 用户管理（登录账号 CRUD，见 PLAN-admin-user.md；admin_user 是系统账号域，不挂 /dxsf）
	admin := r.Group("/api/v1/admin", Auth(svc))
	admin.POST("/user/list", auh.List)
	admin.POST("/user/add", auh.Add)
	admin.POST("/user/edit", auh.Edit)
	admin.POST("/user/delete", auh.Delete)

	// 订单系统 Demo（Gin → MySQL → RabbitMQ 异步链路，见 PLAN-order.md）：
	// 创建后发 order.created 广播给库存/积分/通知三队列，消费者为独立进程 cmd/consumer
	authed.POST("/orders", oh.Create)
	authed.POST("/orders/list", oh.List)
	authed.GET("/orders/products", oh.Products)
	authed.POST("/points/list", oh.PointsList)
	authed.POST("/notifications/list", oh.NotificationsList)

	// 商品管理 CRUD（product 表维护，见 PLAN-product-crud.md；/orders/products 全量下拉保留不动）
	authed.POST("/products/list", oh.ProductList)
	authed.POST("/products/add", oh.ProductAdd)
	authed.POST("/products/edit", oh.ProductEdit)
	authed.POST("/products/delete", oh.ProductDelete)

	// AB 版模块配置管理台 CRUD（C 端 H5 gyz-h5-spacestation 显隐配置，见 PLAN-ab-module.md）
	ab := r.Group("/api/v1/ab", Auth(svc))
	ab.POST("/modules/list", abh.ModuleList)
	ab.GET("/modules/options", abh.ModuleOptions)
	ab.POST("/modules/add", abh.ModuleAdd)
	ab.POST("/modules/edit", abh.ModuleEdit)
	ab.POST("/modules/delete", abh.ModuleDelete)
	ab.POST("/items/list", abh.ItemList)
	ab.POST("/items/add", abh.ItemAdd)
	ab.POST("/items/edit", abh.ItemEdit)
	ab.POST("/items/delete", abh.ItemDelete)

	// AB 聚合查询：免鉴权直挂引擎（H5 无本服务登录态，公网域名直访，login 同款先例），
	// 返回全量配置两级 map，语义区别于 modules 资源的分页列表
	r.GET("/api/v1/ab/config", abh.AbConfig)
	return r
}
