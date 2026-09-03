package handler

import (
	"errors"
	"log/slog"

	"github.com/gin-gonic/gin"

	"gyz-service/internal/model"
	"gyz-service/internal/response"
	"gyz-service/internal/service"
)

// ResignHandler 老师离职转移 HTTP 层
type ResignHandler struct {
	svc *service.Service
}

func NewResign(svc *service.Service) *ResignHandler {
	return &ResignHandler{svc: svc}
}

// List POST /api/v1/dxsf/teacher/resign/list
//
// 错误映射：body 绑定失败/数值字段非法串 → 400；其他 → 500（真实原因只进日志）
//
//	@Summary		离职转移列表
//	@Description	分页查询离职转移记录（筛选条件走 JSON body，姓名类为模糊匹配，dept_id 精确匹配原老师部门快照，空值表示未填）
//	@Tags			离职转移
//	@Accept			json
//	@Produce		json
//	@Param			body body model.ResignListReq true "查询条件（姓名类模糊匹配，dept_id 精确，传空串/null 表示未填）"
//	@Success		200 {object} model.ResignListResp
//	@Failure		400 {object} response.Response "请求体非法"
//	@Failure		500 {object} response.Response "服务器内部错误"
//	@Security		ApiKeyAuth
//	@Router			/dxsf/teacher/resign/list [post]
func (h *ResignHandler) List(c *gin.Context) {
	var req model.ResignListReq
	if err := c.ShouldBindJSON(&req); err != nil {
		slog.Error("bind resign list request failed", "err", err)
		response.Fail(c, 400, 400, "请求体非法")
		return
	}

	var f model.ResignListFilter
	f.DeptID = derefOrZero(req.DeptID.Ptr())
	f.OriginalTeacher = req.OriginalTeacher
	f.ReplaceTeacher = req.ReplaceTeacher
	f.Salesman = req.Salesman
	f.TransferBeginTime = req.TransferBeginTime
	f.TransferEndTime = req.TransferEndTime
	f.PageIndex, f.PageSize = req.PageIndex, req.PageSize // 默认在 service 层兜底

	result, err := h.svc.ListResigns(c.Request.Context(), f)
	switch {
	case err != nil:
		slog.Error("list resigns failed", "err", err)
		response.Fail(c, 500, 500, "服务器内部错误")
	default:
		response.OKMsg(c, "success", result)
	}
}

// Add POST /api/v1/dxsf/teacher/resign/add
//
// 错误映射：body 绑定失败/同人/转移内容超长 → 400；
// 老师不存在 → 404；其他 → 500
//
//	@Summary		新增离职转移
//	@Description	原老师绑定业务员全部转移给接替老师（去重合并），姓名/部门等冗余快照由后端回查，group_count 按原老师绑定数计算；原老师无绑定业务员时 400
//	@Tags			离职转移
//	@Accept			json
//	@Produce		json
//	@Param			body body model.ResignAddReq true "新增离职转移请求体"
//	@Success		200 {object} model.ActionResp "msg 固定「转移成功」，data 恒为 null"
//	@Failure		400 {object} response.Response "请求体非法 / 原老师与接替老师相同 / 转移内容超过 200 字符 / 原老师无绑定业务员"
//	@Failure		404 {object} response.Response "老师不存在"
//	@Failure		500 {object} response.Response "服务器内部错误"
//	@Security		ApiKeyAuth
//	@Router			/dxsf/teacher/resign/add [post]
func (h *ResignHandler) Add(c *gin.Context) {
	var req model.ResignAddReq
	if err := c.ShouldBindJSON(&req); err != nil {
		slog.Error("bind resign add request failed", "err", err)
		response.Fail(c, 400, 400, "请求体非法")
		return
	}

	switch err := h.svc.AddResign(c.Request.Context(), req, c.ClientIP()); {
	case err == nil:
		response.OKMsg(c, "转移成功", nil) // mock 约定 msg
	case errors.Is(err, service.ErrTeacherNotFound):
		response.Fail(c, 404, 404, err.Error())
	case errors.Is(err, service.ErrSameTeacher):
		response.Fail(c, 400, 400, err.Error())
	case errors.Is(err, service.ErrTransferContentTooLong):
		response.Fail(c, 400, 400, err.Error())
	case errors.Is(err, service.ErrOriginalTeacherNoSalesman):
		response.Fail(c, 400, 400, err.Error())
	default:
		slog.Error("add resign failed", "original_teacher_id", req.OriginalTeacherID, "replace_teacher_id", req.ReplaceTeacherID, "err", err)
		response.Fail(c, 500, 500, "服务器内部错误")
	}
}
