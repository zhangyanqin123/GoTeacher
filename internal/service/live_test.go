package service

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// newXeUpstream 起一个模拟小鹅通 xe.login.url 的 httptest 服务。
// handler 内部记录收到的请求快照（path/Content-Type/body），供调用方断言透传正确性。
func newXeUpstream(t *testing.T, status int, respBody string) (*httptest.Server, *http.Request) {
	t.Helper()
	var got *http.Request
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Clone(context.Background())
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(respBody))
	}))
	t.Cleanup(srv.Close)
	return srv, got
}

// newXeSvc 用模拟上游地址构造 Service（repo/rdb 传 nil：GetXeLoginURL 不触达 DB/Redis）
func newXeSvc(srv *httptest.Server) *Service {
	return New(nil, nil, "test", time.Hour, srv.URL)
}

// TestGetXeLoginURLOK 成功路径：请求打到固定路径、Content-Type 为 JSON、四字段全透传，返回两个 URL
func TestGetXeLoginURLOK(t *testing.T) {
	srv, got := newXeUpstream(t, http.StatusOK,
		`{"code":0,"msg":"success","data":{"login_url":"https://h5.xiaoe-tech.com/platform/login_cooperate/h5_login?token=abc","permission_denied_url":"https://app_x.h5.xiaoeknow.com/denied"}}`)
	svc := newXeSvc(srv)

	loginURL, deniedURL, err := svc.GetXeLoginURL(context.Background(), "tok-1", "u_9", 1, "https://app1.pc.xiaoe-tech.com/live_pc/l_1")
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
	if loginURL != "https://h5.xiaoe-tech.com/platform/login_cooperate/h5_login?token=abc" || deniedURL != "https://app_x.h5.xiaoeknow.com/denied" {
		t.Errorf("urls = %q / %q", loginURL, deniedURL)
	}

	if got == nil {
		t.Fatal("upstream received no request")
	}
	if got.URL.Path != "/xe.login.url/1.0.0" {
		t.Errorf("upstream path = %q, want /xe.login.url/1.0.0", got.URL.Path)
	}
	if ct := got.Header.Get("Content-Type"); ct != "application/json" {
		t.Errorf("upstream content-type = %q, want application/json", ct)
	}
	var body struct {
		AccessToken string `json:"access_token"`
		UserID      string `json:"user_id"`
		Data        struct {
			LoginType   int    `json:"login_type"`
			RedirectURI string `json:"redirect_uri"`
		} `json:"data"`
	}
	if err := json.NewDecoder(got.Body).Decode(&body); err != nil {
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
