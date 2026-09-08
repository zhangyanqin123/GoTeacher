package handler

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"gyz-service/internal/model"
	"gyz-service/internal/response"
	"gyz-service/internal/service"
)

// MonHandler 前端监控 HTTP 层（ingest 公开 + 查询/告警鉴权，见 PLAN-frontend-monitor.md）
type MonHandler struct {
	svc *service.Service
}

func NewMon(svc *service.Service) *MonHandler {
	return &MonHandler{svc: svc}
}

// 透明 1x1 gif（43 字节）：GET 通道回给 <img>，与静态打点文件行为一致，浏览器 console 零噪音
var monPixelGif = []byte{
	0x47, 0x49, 0x46, 0x38, 0x39, 0x61, 0x01, 0x00, 0x01, 0x00, 0x80, 0x00, 0x00,
	0x00, 0x00, 0x00, 0xff, 0xff, 0xff, 0x21, 0xf9, 0x04, 0x01, 0x00, 0x00, 0x00,
	0x00, 0x2c, 0x00, 0x00, 0x00, 0x00, 0x01, 0x00, 0x01, 0x00, 0x00, 0x02, 0x02,
	0x44, 0x01, 0x00, 0x3b,
}

// monIngestLimits 上限防御（伪造超长流量）：GET d 参数 / POST body
const (
	monGetDMax    = 8 << 10  // 8KB：探针 gif URL 实测 ~3KB，留余量
	monPostMax    = 64 << 10 // 64KB：单条 ~1KB × 批量 20 条量级
	monBatchMax   = 50       // POST 数组条数上限
)

// IngestGet GET /api/v1/mon/event?d=<urlencoded JSON>
// 探针 gif 通道（一期 H5 的 ?d= 协议原样兼容）：img.src 直打本接口。
// 任何异常均回 204/200 空 gif——探针不读响应，报错只会污染浏览器 console，问题仅记服务端日志
//
//	@Summary		事件上报（探针 gif 通道）
//	@Description	H5 探针 window.__GYZMON__ 的 ?d= 上报通道，返回 1x1 透明 gif。免鉴权（H5 无登录态）。解析失败/超限静默丢弃仅记日志
//	@Tags			前端监控
//	@Produce		gif
//	@Param			d query string true "URL-encoded 的事件 JSON（探针 payload）"
//	@Success		200 {file} file "1x1 透明 gif"
//	@Router			/mon/event [get]
func (h *MonHandler) IngestGet(c *gin.Context) {
	d := c.Query("d")
	if d == "" || len(d) > monGetDMax {
		if d != "" {
			slog.Warn("mon ingest get drop oversized", "len", len(d), "ip", c.ClientIP())
		}
		c.Data(http.StatusOK, "image/gif", monPixelGif)
		return
	}
	var e model.MonEventIngest
	if err := json.Unmarshal([]byte(d), &e); err != nil || e.T == "" {
		slog.Warn("mon ingest get drop bad json", "err", err, "ip", c.ClientIP())
		c.Data(http.StatusOK, "image/gif", monPixelGif)
		return
	}
	h.svc.IngestEvents(c.Request.Context(), []model.MonEventIngest{e}, c.ClientIP())
	c.Data(http.StatusOK, "image/gif", monPixelGif)
}

// IngestPost POST /api/v1/mon/event
// 批量/未来通道（sendBeacon POST text/plain——读 raw body 手动 Unmarshal，不校验 Content-Type）
//
//	@Summary		事件上报（POST 批量）
//	@Description	接收单对象或数组（≤50 条）。读 raw body 不校验 Content-Type（sendBeacon 发 text/plain）。免鉴权
//	@Tags			前端监控
//	@Accept			json
//	@Produce		json
//	@Success		200 {object} model.MonIngestResp
//	@Failure		400 {object} response.Response "请求体非法"
//	@Router			/mon/event [post]
func (h *MonHandler) IngestPost(c *gin.Context) {
	body, err := io.ReadAll(io.LimitReader(c.Request.Body, monPostMax+1))
	if err != nil || len(body) > monPostMax || len(body) == 0 {
		response.Fail(c, 400, 400, "请求体非法")
		return
	}

	var events []model.MonEventIngest
	// 先试数组，失败再试单对象（sendBeacon 单条场景 body 是裸对象）
	if err := json.Unmarshal(body, &events); err != nil {
		var one model.MonEventIngest
		if err2 := json.Unmarshal(body, &one); err2 != nil {
			response.Fail(c, 400, 400, "请求体非法")
			return
		}
		events = []model.MonEventIngest{one}
	}
	if len(events) > monBatchMax {
		events = events[:monBatchMax]
	}

	accepted, rejected := h.svc.IngestEvents(c.Request.Context(), events, c.ClientIP())
	response.OKMsg(c, "success", model.MonIngestResp{Accepted: accepted, Rejected: rejected})
}

// ---------- 管理台查询/告警（Auth 组） ----------

// EventList POST /api/v1/mon/event/list
//
//	@Summary		事件多口径查询
//	@Description	多条件分页查询监控事件（时间范围强制：缺省近 24h，跨度 ≤31 天）；capbads 为 FIND_IN_SET 语义；list 项 msg 为 100 字预览，全文走详情接口
//	@Tags			前端监控
//	@Accept			json
//	@Produce		json
//	@Param			body body model.MonEventListReq true "查询条件（全可选除时间）"
//	@Success		200 {object} model.MonEventListResp
//	@Failure		400 {object} response.Response "时间范围非法"
//	@Failure		500 {object} response.Response "服务器内部错误"
//	@Security		ApiKeyAuth
//	@Router			/mon/event/list [post]
func (h *MonHandler) EventList(c *gin.Context) {
	var req model.MonEventListReq
	if err := c.ShouldBindJSON(&req); err != nil {
		slog.Error("bind mon event list request failed", "err", err)
		response.Fail(c, 400, 400, "请求体非法")
		return
	}
	switch result, err := h.svc.ListMonEvents(c.Request.Context(), req); {
	case err != nil:
		if isMonClientError(err) {
			response.Fail(c, 400, 400, err.Error())
			return
		}
		slog.Error("list mon events failed", "err", err)
		response.Fail(c, 500, 500, "服务器内部错误")
	default:
		response.OKMsg(c, "success", result)
	}
}

// EventDetail GET /api/v1/mon/event/detail?id=
//
//	@Summary		事件详情
//	@Description	单事件全字段（含 stack/cap/ua 全文），详情抽屉用
//	@Tags			前端监控
//	@Produce		json
//	@Param			id query int true "事件 ID"
//	@Success		200 {object} model.MonEventDetailResp
//	@Failure		404 {object} response.Response "事件不存在"
//	@Failure		500 {object} response.Response "服务器内部错误"
//	@Security		ApiKeyAuth
//	@Router			/mon/event/detail [get]
func (h *MonHandler) EventDetail(c *gin.Context) {
	id, err := strconv.ParseInt(c.Query("id"), 10, 64)
	if err != nil || id <= 0 {
		response.Fail(c, 400, 400, "请求参数非法")
		return
	}
	switch row, err2 := h.svc.GetMonEvent(c.Request.Context(), id); {
	case err2 != nil:
		if errors.Is(err2, service.ErrMonEventNotFound) {
			response.Fail(c, 404, 404, err2.Error())
			return
		}
		slog.Error("get mon event failed", "id", id, "err", err2)
		response.Fail(c, 500, 500, "服务器内部错误")
	default:
		response.OKMsg(c, "success", row)
	}
}

// Overview POST /api/v1/mon/overview
//
//	@Summary		监控概览聚合
//	@Description	指标卡（总数/错误/会话/机型/语法不兼容）+ 五个 Top 聚合（版本/机型/内核/路由/资源404）。时间缺省近 24h，跨度 ≤7 天
//	@Tags			前端监控
//	@Accept			json
//	@Produce		json
//	@Param			body body model.MonOverviewReq true "概览查询条件"
//	@Success		200 {object} model.MonOverviewRespWrap
//	@Failure		400 {object} response.Response "时间范围非法"
//	@Failure		500 {object} response.Response "服务器内部错误"
//	@Security		ApiKeyAuth
//	@Router			/mon/overview [post]
func (h *MonHandler) Overview(c *gin.Context) {
	var req model.MonOverviewReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, 400, 400, "请求体非法")
		return
	}
	switch result, err := h.svc.MonOverview(c.Request.Context(), req); {
	case err != nil:
		if isMonClientError(err) {
			response.Fail(c, 400, 400, err.Error())
			return
		}
		slog.Error("mon overview failed", "err", err)
		response.Fail(c, 500, 500, "服务器内部错误")
	default:
		response.OKMsg(c, "success", result)
	}
}

// AlertList POST /api/v1/mon/alert/list
//
//	@Summary		告警列表
//	@Description	多条件分页查询告警记录（status 缺省查 pending；最新在前）
//	@Tags			前端监控
//	@Accept			json
//	@Produce		json
//	@Param			body body model.MonAlertListReq true "查询条件"
//	@Success		200 {object} model.MonAlertListResp
//	@Failure		500 {object} response.Response "服务器内部错误"
//	@Security		ApiKeyAuth
//	@Router			/mon/alert/list [post]
func (h *MonHandler) AlertList(c *gin.Context) {
	var req model.MonAlertListReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, 400, 400, "请求体非法")
		return
	}
	switch result, err := h.svc.ListMonAlerts(c.Request.Context(), req); {
	case err != nil:
		slog.Error("list mon alerts failed", "err", err)
		response.Fail(c, 500, 500, "服务器内部错误")
	default:
		response.OKMsg(c, "success", result)
	}
}

// AlertAck POST /api/v1/mon/alert/ack
//
//	@Summary		确认告警
//	@Description	确认人取鉴权上下文；仅 pending 可确认（并发重复确认按 404 语义）
//	@Tags			前端监控
//	@Accept			json
//	@Produce		json
//	@Param			body body model.MonAlertAckReq true "确认请求"
//	@Success		200 {object} model.ActionResp "msg 固定「确认成功」，data 恒为 null"
//	@Failure		400 {object} response.Response "请求体非法"
//	@Failure		404 {object} response.Response "告警不存在或已确认"
//	@Failure		500 {object} response.Response "服务器内部错误"
//	@Security		ApiKeyAuth
//	@Router			/mon/alert/ack [post]
func (h *MonHandler) AlertAck(c *gin.Context) {
	var req model.MonAlertAckReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, 400, 400, "请求体非法")
		return
	}
	ackUser := c.GetString(model.CtxKeyUsername)
	switch err := h.svc.AckMonAlert(c.Request.Context(), req.ID, ackUser, req.AckNote); {
	case err == nil:
		response.OKMsg(c, "确认成功", nil)
	case errors.Is(err, service.ErrMonAlertNotPending):
		response.Fail(c, 404, 404, err.Error())
	default:
		slog.Error("ack mon alert failed", "id", req.ID, "err", err)
		response.Fail(c, 500, 500, "服务器内部错误")
	}
}

// isMonClientError 监控查询的客户端侧哨兵错误（400 语义）统一判定
func isMonClientError(err error) bool {
	return errors.Is(err, service.ErrMonTimeRange) || errors.Is(err, service.ErrMonOverviewRange)
}
