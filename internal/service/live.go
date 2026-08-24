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
)

const (
	xeLoginURLPath = "/xe.login.url/1.0.0" // 小鹅通统一接口路径（域名在配置 XIAOE_API_BASE）
	xeRespLimit    = 1 << 20               // 上游响应体读取上限 1MB，防异常超大响应
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
