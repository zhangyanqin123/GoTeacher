package router

import (
	"database/sql"

	"github.com/gin-gonic/gin"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"

	"handicap-service/internal/handler"
	"handicap-service/internal/repository"
	"handicap-service/internal/service"
)

// New 组装依赖（repo → service → handler）并注册路由
func New(db *sql.DB) *gin.Engine {
	r := gin.Default()
	r.Use(CORS())

	// Swagger 文档（docs 包由 swag init 生成，见 cmd/server/main.go 头注释）
	r.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))

	repo := repository.New(db)
	svc := service.New(repo)
	th := handler.NewTeacher(svc)
	rh := handler.NewResign(svc)
	dh := handler.NewDiagnose(svc)

	// 老师管理（路径与前端 teacher.js 注释里的 URL 完全一致）
	dxsf := r.Group("/api/v1/dxsf")
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
	diag := r.Group("/api/v1/dxsf/diagnose")
	diag.GET("/list", dh.List)
	diag.GET("/detail", dh.Detail)
	diag.POST("/submitReport", dh.SubmitReport)
	diag.POST("/audit", dh.Audit)
	return r
}
