package handler

import (
	"errors"
	"log/slog"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"handicap-service/internal/response"
	"handicap-service/internal/service"
)

// LiveHandler 直播（小鹅通透传）HTTP 层（设计决策见 PLAN-live.md）
type LiveHandler struct {
	svc *service.Service
}

func NewLive(svc *service.Service) *LiveHandler {
	return &LiveHandler{svc: svc}
}

// GetXeLoginURL GET /guyuzhoudb/live/get_login_url
//
// 公开接口不挂 Auth（mofang C 端是另一 token 体系，本服务验不了；/guyuzhoudb 前缀独立于
// /api/v1 鉴权域；凭证即入参 access_token，有效性由小鹅通侧校验），前端附带的 token 参数一律忽略。
//
// 错误映射：参数形状校验失败 → 400；上游失败（业务错/网络错）→ 502。
// 注意：mofang 对 HTTP 4xx/5xx 只弹通用「未知错误」，中文文案需看 Network/curl。
//
//	@Summary		获取小鹅通登录链接
//	@Description	透传小鹅通 xe.login.url/1.0.0：拼装 {access_token, user_id, data:{login_type, redirect_uri}} 调上游，成功返回 login_url（有效期仅 1 分钟，即取即跳勿缓存）与 permission_denied_url。公开无鉴权。注意：全局 @BasePath 为 /api/v1，Swagger UI 显示路径会多出 /api/v1 前缀，实际路径以本注解为准
//	@Tags			直播（小鹅通透传）
//	@Produce		json
//	@Param			access_token query string true "小鹅通 access_token（get_access_token 接口取得，有效性由小鹅通校验）"
//	@Param			user_id query string true "商家侧用户唯一标识（透传）"
//	@Param			login_type query int true "登录类型：1=PC 2=H5 3=App" Enums(1,2,3)
//	@Param			redirect_uri query string false "登录成功后跳转的小鹅通页面完整链接（http(s):// 开头；不传由小鹅通默认跳转）"
//	@Success		200 {object} model.XeLoginURLResp
//	@Failure		400 {object} response.Response "参数缺失或格式非法"
//	@Failure		502 {object} response.Response "小鹅通上游失败（业务错/网络错）"
//	@Router			/guyuzhoudb/live/get_login_url [get]
func (h *LiveHandler) GetXeLoginURL(c *gin.Context) {
	accessToken := c.Query("access_token")
	userID := c.Query("user_id")
	redirectURI := c.Query("redirect_uri")

	// 只做形状校验不做语义校验：凭证有效性交给上游；长度上限挡滥用。
	// 透传字符串不做 HTML 净化（json.Marshal 天然转义，且无存储回显面，与 diagnose 富文本 XSS 场景不同）。
	switch {
	case accessToken == "":
		response.Fail(c, 400, 400, "参数 access_token 不能为空")
		return
	case len(accessToken) > 512:
		response.Fail(c, 400, 400, "参数 access_token 长度超限")
		return
	case userID == "":
		response.Fail(c, 400, 400, "参数 user_id 不能为空")
		return
	case len(userID) > 64:
		response.Fail(c, 400, 400, "参数 user_id 长度超限")
		return
	}
	loginType, err := strconv.Atoi(c.Query("login_type"))
	if err != nil || loginType < 1 || loginType > 3 {
		response.Fail(c, 400, 400, "参数 login_type 必须为 1、2 或 3 的整数")
		return
	}
	if redirectURI != "" {
		if !strings.HasPrefix(redirectURI, "http://") && !strings.HasPrefix(redirectURI, "https://") {
			response.Fail(c, 400, 400, "参数 redirect_uri 必须以 http:// 或 https:// 开头")
			return
		}
		if len(redirectURI) > 2048 {
			response.Fail(c, 400, 400, "参数 redirect_uri 长度超限")
			return
		}
	}

	loginURL, deniedURL, err := h.svc.GetXeLoginURL(c.Request.Context(), accessToken, userID, loginType, redirectURI)
	switch {
	case err == nil:
		response.OKMsg(c, "success", gin.H{"login_url": loginURL, "permission_denied_url": deniedURL})
	case errors.Is(err, service.ErrXeUpstream), errors.Is(err, service.ErrXeEmptyLoginURL):
		response.Fail(c, 502, 502, err.Error()) // 哨兵文本即对外文案
	default:
		slog.Error("xe login url failed", "user_id", userID, "err", err)
		response.Fail(c, 502, 502, "小鹅通服务暂不可用，请稍后重试")
	}
}

// RegisterXeUser GET /guyuzhoudb/live/register_user
//
// 公开接口不挂 Auth（同 GetXeLoginURL，凭证即入参 access_token）。phone 不落日志（PII）。
// 上游幂等：已存在用户（user_exists=1）也返回 user_id，可重复调用。
//
// 错误映射：参数形状校验失败 → 400；上游失败 → 502。
//
//	@Summary		注册小鹅通用户换取 user_id
//	@Description	透传 xe.user.register/1.0.0：按手机号注册（幂等，已存在直接返回）换回小鹅通 user_id，供 get_login_url 指定登录账号。公开无鉴权。注意：全局 @BasePath 为 /api/v1，Swagger UI 显示路径会多出 /api/v1 前缀，实际路径以本注解为准
//	@Tags			直播（小鹅通透传）
//	@Produce		json
//	@Param			access_token query string true "小鹅通 access_token（get_access_token 取得，有效性由小鹅通校验）"
//	@Param			phone query string true "用户手机号（11 位数字）"
//	@Success		200 {object} model.XeRegisterUserResp
//	@Failure		400 {object} response.Response "参数缺失或格式非法"
//	@Failure		502 {object} response.Response "小鹅通上游失败（业务错/网络错）"
//	@Router			/guyuzhoudb/live/register_user [get]
func (h *LiveHandler) RegisterXeUser(c *gin.Context) {
	accessToken := c.Query("access_token")
	phone := c.Query("phone")

	// 形状校验同 GetXeLoginURL：凭证有效性交上游；phone 限定 11 位数字（国内手机号）
	switch {
	case accessToken == "":
		response.Fail(c, 400, 400, "参数 access_token 不能为空")
		return
	case len(accessToken) > 512:
		response.Fail(c, 400, 400, "参数 access_token 长度超限")
		return
	case len(phone) != 11:
		response.Fail(c, 400, 400, "参数 phone 必须是 11 位手机号")
		return
	}
	for _, r := range phone {
		if r < '0' || r > '9' {
			response.Fail(c, 400, 400, "参数 phone 必须是 11 位手机号")
			return
		}
	}

	userID, userExists, err := h.svc.RegisterXeUser(c.Request.Context(), accessToken, phone)
	switch {
	case err == nil:
		response.OKMsg(c, "success", gin.H{"user_id": userID, "user_exists": userExists})
	case errors.Is(err, service.ErrXeUserRegister), errors.Is(err, service.ErrXeEmptyUserID):
		response.Fail(c, 502, 502, err.Error()) // 哨兵文本即对外文案
	default:
		slog.Error("xe register user failed", "err", err)
		response.Fail(c, 502, 502, "小鹅通服务暂不可用，请稍后重试")
	}
}
