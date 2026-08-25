package service

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

// xeUpstreamCall 模拟上游收到的请求快照。
// 在 handler goroutine 内同步读完 body 存 bytes（handler 返回后 server 会关闭 r.Body，
// 外部再读 *http.Request 属 use-after-close；跨 goroutine 裸读写指针也有可见性竞态），mutex 保护读写。
type xeUpstreamCall struct {
	mu          sync.Mutex
	path        string
	contentType string
	body        []byte
}

func (c *xeUpstreamCall) snapshot(r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	c.mu.Lock()
	c.path, c.contentType, c.body = r.URL.Path, r.Header.Get("Content-Type"), body
	c.mu.Unlock()
}

func (c *xeUpstreamCall) get() (path, contentType string, body []byte) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.path, c.contentType, c.body
}

// newXeUpstream 起一个模拟小鹅通开放平台（xe.login.url / xe.user.register 共用）的 httptest 服务，
// 按入参返回状态码与 JSON 响应体，并把收到的请求快照进 call 供调用方断言透传正确性。
func newXeUpstream(t *testing.T, status int, respBody string) (*httptest.Server, *xeUpstreamCall) {
	t.Helper()
	call := &xeUpstreamCall{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		call.snapshot(r)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(respBody))
	}))
	t.Cleanup(srv.Close)
	return srv, call
}

// newXeSvc 用模拟上游地址构造 Service（repo/rdb 传 nil：直播域方法不触达 DB/Redis）
func newXeSvc(srv *httptest.Server) *Service {
	return New(nil, nil, "test", time.Hour, srv.URL)
}

// TestGetXeLoginURLOK 成功路径：请求打到固定路径、Content-Type 为 JSON、四字段全透传，返回两个 URL
func TestGetXeLoginURLOK(t *testing.T) {
	srv, call := newXeUpstream(t, http.StatusOK,
		`{"code":0,"msg":"success","data":{"login_url":"https://h5.xiaoe-tech.com/platform/login_cooperate/h5_login?token=abc","permission_denied_url":"https://app_x.h5.xiaoeknow.com/denied"}}`)
	svc := newXeSvc(srv)

	loginURL, deniedURL, err := svc.GetXeLoginURL(context.Background(), "tok-1", "u_9", 1, "https://app1.pc.xiaoe-tech.com/live_pc/l_1")
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
	if loginURL != "https://h5.xiaoe-tech.com/platform/login_cooperate/h5_login?token=abc" || deniedURL != "https://app_x.h5.xiaoeknow.com/denied" {
		t.Errorf("urls = %q / %q", loginURL, deniedURL)
	}

	path, contentType, rawBody := call.get()
	if path != "/xe.login.url/1.0.0" {
		t.Errorf("upstream path = %q, want /xe.login.url/1.0.0", path)
	}
	if contentType != "application/json" {
		t.Errorf("upstream content-type = %q, want application/json", contentType)
	}
	var body struct {
		AccessToken string `json:"access_token"`
		UserID      string `json:"user_id"`
		Data        struct {
			LoginType   int    `json:"login_type"`
			RedirectURI string `json:"redirect_uri"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rawBody, &body); err != nil {
		t.Fatalf("decode upstream body: %v", err)
	}
	if body.AccessToken != "tok-1" || body.UserID != "u_9" || body.Data.LoginType != 1 || body.Data.RedirectURI != "https://app1.pc.xiaoe-tech.com/live_pc/l_1" {
		t.Errorf("upstream body = %+v, want all four passthrough fields", body)
	}
}

// TestGetXeLoginURLUpstreamBizError 上游 code != 0（如 access_token 无效）→ ErrXeUpstream
func TestGetXeLoginURLUpstreamBizError(t *testing.T) {
	srv, _ := newXeUpstream(t, http.StatusOK, `{"code":400,"msg":"invalid access_token","data":{}}`)
	svc := newXeSvc(srv)

	_, _, err := svc.GetXeLoginURL(context.Background(), "bad-tok", "u_9", 2, "")
	if !errors.Is(err, ErrXeUpstream) {
		t.Errorf("err = %v, want ErrXeUpstream", err)
	}
}

// TestGetXeLoginURLEmptyLoginURL 上游 code=0 但 login_url 为空 → ErrXeEmptyLoginURL
func TestGetXeLoginURLEmptyLoginURL(t *testing.T) {
	srv, _ := newXeUpstream(t, http.StatusOK, `{"code":0,"msg":"success","data":{"login_url":"","permission_denied_url":""}}`)
	svc := newXeSvc(srv)

	_, _, err := svc.GetXeLoginURL(context.Background(), "tok-1", "u_9", 3, "")
	if !errors.Is(err, ErrXeEmptyLoginURL) {
		t.Errorf("err = %v, want ErrXeEmptyLoginURL", err)
	}
}

// TestGetXeLoginURLUpstreamHTTP500 上游非 200 → 非哨兵错误（handler 走 default 502）
func TestGetXeLoginURLUpstreamHTTP500(t *testing.T) {
	srv, _ := newXeUpstream(t, http.StatusInternalServerError, `internal error`)
	svc := newXeSvc(srv)

	_, _, err := svc.GetXeLoginURL(context.Background(), "tok-1", "u_9", 1, "")
	if err == nil || errors.Is(err, ErrXeUpstream) || errors.Is(err, ErrXeEmptyLoginURL) {
		t.Errorf("err = %v, want non-sentinel error", err)
	}
}

// TestGetXeLoginURLBadJSON 上游返回坏 JSON → 非哨兵错误
func TestGetXeLoginURLBadJSON(t *testing.T) {
	srv, _ := newXeUpstream(t, http.StatusOK, `{not-json`)
	svc := newXeSvc(srv)

	_, _, err := svc.GetXeLoginURL(context.Background(), "tok-1", "u_9", 1, "")
	if err == nil || errors.Is(err, ErrXeUpstream) || errors.Is(err, ErrXeEmptyLoginURL) {
		t.Errorf("err = %v, want non-sentinel error", err)
	}
}

// TestGetXeLoginURLNetworkError 上游不可达（服务已关）→ 非哨兵错误
func TestGetXeLoginURLNetworkError(t *testing.T) {
	srv, _ := newXeUpstream(t, http.StatusOK, `{}`)
	svc := newXeSvc(srv)
	srv.Close() // 模拟上游宕机

	_, _, err := svc.GetXeLoginURL(context.Background(), "tok-1", "u_9", 1, "")
	if err == nil || errors.Is(err, ErrXeUpstream) || errors.Is(err, ErrXeEmptyLoginURL) {
		t.Errorf("err = %v, want non-sentinel error", err)
	}
}

// TestRegisterXeUserOK 成功：路径/Content-Type/body 两字段全透传，返回 user_id 与 user_exists
func TestRegisterXeUserOK(t *testing.T) {
	srv, call := newXeUpstream(t, http.StatusOK, `{"code":0,"msg":"ok","data":{"user_exists":1,"user_id":"u_api_1"}}`)
	svc := newXeSvc(srv)

	userID, userExists, err := svc.RegisterXeUser(context.Background(), "tok-1", "18820205724")
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
	if userID != "u_api_1" || userExists != 1 {
		t.Errorf("got = %q / %d, want u_api_1 / 1", userID, userExists)
	}

	path, contentType, rawBody := call.get()
	if path != "/xe.user.register/1.0.0" {
		t.Errorf("upstream path = %q, want /xe.user.register/1.0.0", path)
	}
	if contentType != "application/json" {
		t.Errorf("upstream content-type = %q, want application/json", contentType)
	}
	var body struct {
		AccessToken string `json:"access_token"`
		Data        struct {
			Phone string `json:"phone"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rawBody, &body); err != nil {
		t.Fatalf("decode upstream body: %v", err)
	}
	if body.AccessToken != "tok-1" || body.Data.Phone != "18820205724" {
		t.Errorf("upstream body = %+v, want access_token and data.phone passthrough", body)
	}
}

// TestRegisterXeUserUpstreamBizError 上游 code != 0（如 2017 无接口权限）→ ErrXeUserRegister
func TestRegisterXeUserUpstreamBizError(t *testing.T) {
	srv, _ := newXeUpstream(t, http.StatusOK, `{"code":2017,"msg":"no permission","data":{}}`)
	svc := newXeSvc(srv)

	_, _, err := svc.RegisterXeUser(context.Background(), "tok-1", "18820205724")
	if !errors.Is(err, ErrXeUserRegister) {
		t.Errorf("err = %v, want ErrXeUserRegister", err)
	}
}

// TestRegisterXeUserEmptyUserID 上游 code=0 但 user_id 为空 → ErrXeEmptyUserID
func TestRegisterXeUserEmptyUserID(t *testing.T) {
	srv, _ := newXeUpstream(t, http.StatusOK, `{"code":0,"msg":"ok","data":{"user_exists":0,"user_id":""}}`)
	svc := newXeSvc(srv)

	_, _, err := svc.RegisterXeUser(context.Background(), "tok-1", "18820205724")
	if !errors.Is(err, ErrXeEmptyUserID) {
		t.Errorf("err = %v, want ErrXeEmptyUserID", err)
	}
}

// TestRegisterXeUserNetworkError 上游不可达（服务已关）→ 非哨兵错误
func TestRegisterXeUserNetworkError(t *testing.T) {
	srv, _ := newXeUpstream(t, http.StatusOK, `{}`)
	svc := newXeSvc(srv)
	srv.Close() // 模拟上游宕机

	_, _, err := svc.RegisterXeUser(context.Background(), "tok-1", "18820205724")
	if err == nil || errors.Is(err, ErrXeUserRegister) || errors.Is(err, ErrXeEmptyUserID) {
		t.Errorf("err = %v, want non-sentinel error", err)
	}
}
