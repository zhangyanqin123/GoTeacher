package handler

import (
	"errors"
	"log/slog"

	"github.com/gin-gonic/gin"

	"handicap-service/internal/response"
	"handicap-service/internal/service"
)

// HouseUpDownHandler HTTP 层（对应 Spring Boot 的 Controller）
type HouseUpDownHandler struct {
	svc *service.Service
}

func NewHouseUpDown(svc *service.Service) *HouseUpDownHandler {
	return &HouseUpDownHandler{svc: svc}
}

// Get GET /handicap/v1/index-points/houses_up_or_down
//
// 错误映射：
//   - 缺 secuMarket / 非法 range  → 400
//   - 数据库查询失败              → 500（真实原因只进日志，不对外泄露）
//   - 查无数据                    → 200 + data:null（正常业务结果，非 404）
func (h *HouseUpDownHandler) Get(c *gin.Context) {
	data, err := h.svc.GetHouseUpDown(c.Request.Context(), c.Query("secuMarket"), c.Query("range"))
	switch {
	case errors.Is(err, service.ErrMissingMarket):
		response.Fail(c, 400, 400, "secuMarket is required")
	case errors.Is(err, service.ErrInvalidRange):
		response.Fail(c, 400, 400, "range must be today/week/month")
	case err != nil:
		slog.Error("query house_up_down failed", "err", err)
		response.Fail(c, 500, 500, "internal server error")
	case data == nil:
		response.OK(c, nil)
	default:
		response.OK(c, data)
	}
}