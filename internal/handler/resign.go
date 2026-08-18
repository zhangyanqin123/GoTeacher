package handler

import (
	"errors"
	"log/slog"
	"strconv"

	"github.com/gin-gonic/gin"

	"handicap-service/internal/model"
	"handicap-service/internal/response"
	"handicap-service/internal/service"
)

// ResignHandler 老师离职转移 HTTP 层
type ResignHandler struct {
	svc *service.Service
}

func NewResign(svc *service.Service) *ResignHandler {
	return &ResignHandler{svc: svc}
}

// List GET /api/v1/dxsf/resign/list
//
// 错误映射：数字参数格式错误 → 400；其他 → 500（真实原因只进日志）
//
//	@Summary		离职转移列表
//	@Description	分页查询离职转移记录，零值筛选字段不参与过滤；dept_id 匹配原老师部门
//	@Tags			离职转移
//	@Accept			json
//	@Produce		json
//	@Param			dept_id query integer false "原老师部门 ID"
//	@Param			original_teacher_id query integer false "原老师 ID（精确）"
//	@Param			replace_teacher_id query integer false "接替老师 ID（精确）"
//	@Param			salesman_name query string false "业务员姓名（模糊）"
//	@Param			transfer_begin_time query string false "转移时间起（yyyy-MM-dd，与 transfer_end_time 成对生效）"
//	@Param			transfer_end_time query string false "转移时间止（yyyy-MM-dd）"
//	@Param			page_index query integer false "页码（默认 1）"
//	@Param			page_size query integer false "页大小（默认 10，上限 100）"
//	@Success		200 {object} model.ResignListResp
//	@Failure		400 {object} response.Response "数字参数格式错误"
//	@Failure		500 {object} response.Response "服务器内部错误"
//	@Router			/resign/list [get]
func (h *ResignHandler) List(c *gin.Context) {
	var f model.ResignListFilter

	for _, num := range []struct {
		key   string
		dst   *int64
		label string
	}{
		{"dept_id", &f.DeptID, "dept_id"},
		{"original_teacher_id", &f.OriginalTeacherID, "original_teacher_id"},
		{"replace_teacher_id", &f.ReplaceTeacherID, "replace_teacher_id"},
	} {
		s := c.Query(num.key)
		if s == "" {
			continue
		}
		v, err := strconv.ParseInt(s, 10, 64)
		if err != nil {
			response.Fail(c, 400, 400, num.label+" must be an integer")
			return
		}
		*num.dst = v
	}
	f.SalesmanName = c.Query("salesman_name")
	f.TransferBeginTime = c.Query("transfer_begin_time")
	f.TransferEndTime = c.Query("transfer_end_time")
	f.PageIndex, f.PageSize = queryPage(c) // 默认在 service 层兜底

	result, err := h.svc.ListResigns(c.Request.Context(), f)
	switch {
	case err != nil:
		slog.Error("list resigns failed", "err", err)
		response.Fail(c, 500, 500, "internal server error")
	default:
		response.OKMsg(c, "success", result)
	}
}

// Add POST /api/v1/dxsf/resign/add
//
// 错误映射：body 绑定失败/同人/内容白名单/remark 超长 → 400；
// 老师不存在 → 404；其他 → 500
//
//	@Summary		新增离职转移
//	@Description	原老师绑定业务员全部转移给接替老师（去重合并），姓名/部门等冗余快照由后端回查，group_count 按原老师绑定数计算
//	@Tags			离职转移
//	@Accept			json
//	@Produce		json
//	@Param			body body model.ResignAddReq true "新增离职转移请求体"
//	@Success		200 {object} model.ActionResp "msg 固定「转移成功」，data 恒为 null"
//	@Failure		400 {object} response.Response "请求体非法 / 原老师与接替老师相同 / transfer_content 白名单校验失败 / remark 超过 200 字符"
//	@Failure		404 {object} response.Response "老师不存在"
//	@Failure		500 {object} response.Response "服务器内部错误"
//	@Router			/resign/add [post]
func (h *ResignHandler) Add(c *gin.Context) {
	var req model.ResignAddReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, 400, 400, "invalid request body: "+err.Error())
		return
	}

	switch err := h.svc.AddResign(c.Request.Context(), req, c.ClientIP()); {
	case err == nil:
		response.OKMsg(c, "转移成功", nil) // mock 约定 msg
	case errors.Is(err, service.ErrTeacherNotFound):
		response.Fail(c, 404, 404, "teacher not found")
	case errors.Is(err, service.ErrSameTeacher):
		response.Fail(c, 400, 400, "original teacher and replace teacher must differ")
	case errors.Is(err, service.ErrInvalidTransferContent):
		response.Fail(c, 400, 400, "transfer_content must be a non-empty subset of [group]")
	case errors.Is(err, service.ErrRemarkTooLong):
		response.Fail(c, 400, 400, "remark must be at most 200 characters")
	default:
		slog.Error("add resign failed", "original_teacher_id", req.OriginalTeacherID, "replace_teacher_id", req.ReplaceTeacherID, "err", err)
		response.Fail(c, 500, 500, "internal server error")
	}
}
