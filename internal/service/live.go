package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"
)

// 直播（小鹅通透传）哨兵错误：文本即对外展示文案（对齐 auth.go 约定），handler 映射 502。
var (
	// ErrXeUpstream 上游 xe.login.url 业务失败（code != 0，最常见为 access_token 无效/过期）
	ErrXeUpstream = errors.New("获取小鹅通登录链接失败")
	// ErrXeEmptyLoginURL 上游 code=0 但 login_url 为空（异常数据）
	ErrXeEmptyLoginURL = errors.New("小鹅通未返回登录链接")
	// ErrXeUserRegister 上游 xe.user.register 业务失败（code != 0，最常见 access_token 无效/无接口权限）
	ErrXeUserRegister = errors.New("获取小鹅通用户失败")
	// ErrXeEmptyUserID 上游 code=0 但 user_id 为空（异常数据）
	ErrXeEmptyUserID = errors.New("小鹅通未返回用户ID")
)

const (
	xeLoginURLPath  = "/xe.login.url/1.0.0"    // 小鹅通统一接口路径（域名在配置 XIAOE_API_BASE）
	xeRegisterPath  = "/xe.user.register/1.0.0" // 注册用户接口路径（幂等，按手机号换 user_id）
	xeRespLimit     = 1 << 20                   // 上游响应体读取上限 1MB，防异常超大响应
)

// xeHTTPClient 上游专用 client：包级单例复用连接池；10s 覆盖建连+读写
// （login_url 时效仅 1 分钟，超时再长无意义）。本项目首个出站 HTTP 调用。
var xeHTTPClient = &http.Client{Timeout: 10 * time.Second}

// xeLoginReq 上游请求体（字段名即小鹅通文档原文，勿改）
type xeLoginReq struct {
	AccessToken string         `json:"access_token"`
	UserID      string         `json:"user_id"`
	Data        xeLoginReqData `json:"data"`
}

type xeLoginReqData struct {
	LoginType   int    `json:"login_type"`
	RedirectURI string `json:"redirect_uri"`
}

// xeLoginResp 上游响应：code=0 成功；data.login_url 有效期仅 1 分钟
type xeLoginResp struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
	Data struct {
		LoginURL            string `json:"login_url"`
		PermissionDeniedURL string `json:"permission_denied_url"`
	} `json:"data"`
}

// GetXeLoginURL 透传小鹅通 xe.login.url/1.0.0，返回 login_url 与 permission_denied_url。
// 凭证即入参 accessToken（有效性由小鹅通侧校验，本服务只做形状校验，见 handler/live.go）；
// login_url 即取即用（1 分钟时效），禁止任何缓存。wire 类型定义在本文件，model 留给 Swagger 文档类型。
func (s *Service) GetXeLoginURL(ctx context.Context, accessToken, userID string, loginType int, redirectURI string) (loginURL, permissionDeniedURL string, err error) {
	body, err := json.Marshal(xeLoginReq{
		AccessToken: accessToken,
		UserID:      userID,
		Data:        xeLoginReqData{LoginType: loginType, RedirectURI: redirectURI},
	})
	if err != nil {
		return "", "", fmt.Errorf("xe login url: marshal request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.xiaoeAPIBase+xeLoginURLPath, bytes.NewReader(body))
	if err != nil {
		return "", "", fmt.Errorf("xe login url: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := xeHTTPClient.Do(req)
	if err != nil {
		// 网络错与 ctx 取消都走这里；access_token 不落日志，只落 user_id
		slog.Error("xe login url: upstream request failed", "user_id", userID, "err", err)
		return "", "", fmt.Errorf("xe login url: do: %w", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(io.LimitReader(resp.Body, xeRespLimit))
	if err != nil {
		slog.Error("xe login url: read upstream body failed", "user_id", userID, "err", err)
		return "", "", fmt.Errorf("xe login url: read body: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		slog.Error("xe login url: unexpected upstream status", "user_id", userID, "status", resp.StatusCode)
		return "", "", fmt.Errorf("xe login url: upstream status %d", resp.StatusCode)
	}
	var xr xeLoginResp
	if err := json.Unmarshal(raw, &xr); err != nil {
		slog.Error("xe login url: decode upstream body failed", "user_id", userID, "err", err)
		return "", "", fmt.Errorf("xe login url: decode body: %w", err)
	}
	if xr.Code != 0 {
		// 业务失败最常见原因：access_token 无效/过期。上游 code/msg 只进日志不透出（文案不可控 + 防探测）
		slog.Warn("xe login url: upstream business error", "user_id", userID, "code", xr.Code, "msg", xr.Msg)
		return "", "", ErrXeUpstream
	}
	if xr.Data.LoginURL == "" {
		slog.Warn("xe login url: empty login_url", "user_id", userID)
		return "", "", ErrXeEmptyLoginURL
	}
	// 成功也打一条（联调排障用）：login_url 含 1 分钟时效的短时登录 token，风险可控
	slog.Info("xe login url: success", "user_id", userID, "login_url", xr.Data.LoginURL, "permission_denied_url", xr.Data.PermissionDeniedURL)
	return xr.Data.LoginURL, xr.Data.PermissionDeniedURL, nil
}

// xeRegisterReq 注册用户上游请求体（字段名即小鹅通文档原文，勿改；data.phone 与 wx_union_id 二选一）
type xeRegisterReq struct {
	AccessToken string            `json:"access_token"`
	Data        xeRegisterReqData `json:"data"`
}

type xeRegisterReqData struct {
	Phone string `json:"phone"`
}

// xeRegisterResp 注册用户上游响应：code=0 成功；user_exists 0=新建 1=已存在（幂等，两者都返回 user_id）
type xeRegisterResp struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
	Data struct {
		UserExists int    `json:"user_exists"`
		UserID     string `json:"user_id"`
	} `json:"data"`
}

// RegisterXeUser 透传小鹅通 xe.user.register/1.0.0：按手机号注册（幂等，已存在直接返回）换回小鹅通 user_id。
// user_id 即取即用（xe.login.url 的消费端会校验其在该店铺的存在性，8500「获取用户信息失败」即 user_id 无效）。
// access_token/phone 均不落日志（凭证/PII），成功日志只落 user_id 与 user_exists。
func (s *Service) RegisterXeUser(ctx context.Context, accessToken, phone string) (userID string, userExists int, err error) {
	body, err := json.Marshal(xeRegisterReq{AccessToken: accessToken, Data: xeRegisterReqData{Phone: phone}})
	if err != nil {
		return "", 0, fmt.Errorf("xe register user: marshal request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.xiaoeAPIBase+xeRegisterPath, bytes.NewReader(body))
	if err != nil {
		return "", 0, fmt.Errorf("xe register user: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := xeHTTPClient.Do(req)
	if err != nil {
		slog.Error("xe register user: upstream request failed", "err", err)
		return "", 0, fmt.Errorf("xe register user: do: %w", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(io.LimitReader(resp.Body, xeRespLimit))
	if err != nil {
		slog.Error("xe register user: read upstream body failed", "err", err)
		return "", 0, fmt.Errorf("xe register user: read body: %w", err)
	}
	// 联调排障：上游 /xe.user.register/1.0.0 响应原文直接落日志（body 仅 code/msg/user_id/user_exists，无凭证无手机号）
	slog.Info("xe register user: upstream response", "status", resp.StatusCode, "body", string(raw))
	if resp.StatusCode != http.StatusOK {
		slog.Error("xe register user: unexpected upstream status", "status", resp.StatusCode)
		return "", 0, fmt.Errorf("xe register user: upstream status %d", resp.StatusCode)
	}
	var xr xeRegisterResp
	if err := json.Unmarshal(raw, &xr); err != nil {
		slog.Error("xe register user: decode upstream body failed", "err", err)
		return "", 0, fmt.Errorf("xe register user: decode body: %w", err)
	}
	if xr.Code != 0 {
		// 最常见：access_token 无效/过期、无注册接口权限（2017）；上游 code/msg 只进日志不透出
		slog.Warn("xe register user: upstream business error", "code", xr.Code, "msg", xr.Msg)
		return "", 0, ErrXeUserRegister
	}
	if xr.Data.UserID == "" {
		slog.Warn("xe register user: empty user_id")
		return "", 0, ErrXeEmptyUserID
	}
	slog.Info("xe register user: success", "user_id", xr.Data.UserID, "user_exists", xr.Data.UserExists)
	return xr.Data.UserID, xr.Data.UserExists, nil
}
