package router

import (
	"database/sql"

	"github.com/gin-gonic/gin"

	"handicap-service/internal/handler"
	"handicap-service/internal/repository"
	"handicap-service/internal/service"
)

// New 组装依赖（repo → service → handler）并注册路由
func New(db *sql.DB) *gin.Engine {
	r := gin.Default()
	r.Use(CORS())

	repo := repository.New(db)
	svc := service.New(repo)
	th := handler.NewTeacher(svc)
	rh := handler.NewResign(svc)
	dh := handler.NewDiagnose(svc)

	// 老师管理（路径与前端 teacher.js 注释里的 URL 完全一致）
	chat := r.Group("/api/v1/dxsf/chatSys")
	chat.GET("/teacher/list", th.List)
	chat.GET("/teacher/options", th.Options)
	chat.PUT("/teacher/update", th.Update)
	chat.GET("/teacher/bindSales/list", th.SalesList)
	chat.GET("/teacher/bindSales/boundUserIds", th.BoundUserIds)
	chat.POST("/teacher/bindSales", th.Bind)

	// 离职转移（路径与前端 resign.js 注释里的 URL 完全一致）
	chat.GET("/resign/list", rh.List)
	chat.POST("/resign/add", rh.Add)

	// 诊股记录（路径与前端 diagnose.js 注释里的 URL 完全一致）
	diag := r.Group("/api/v1/dxsf/diagnose")
	diag.GET("/list", dh.List)
	diag.GET("/detail", dh.Detail)
	diag.POST("/submitReport", dh.SubmitReport)
	diag.POST("/audit", dh.Audit)
	return r
}
