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
	h := handler.NewHouseUpDown(svc)
	th := handler.NewTeacher(svc)

	v1 := r.Group("/handicap/v1")
	v1.GET("/index-points/houses_up_or_down", h.Get)

	// 老师管理（路径与前端 teacher.js 注释里的 URL 完全一致）
	chat := r.Group("/api/v1/dxsf/chatSys")
	chat.GET("/teacher/list", th.List)
	chat.GET("/teacher/options", th.Options)
	chat.PUT("/teacher/update", th.Update)
	chat.GET("/teacher/bindSales/list", th.SalesList)
	chat.POST("/teacher/bindSales", th.Bind)
	return r
}
