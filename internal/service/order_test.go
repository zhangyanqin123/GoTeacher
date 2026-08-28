package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"handicap-service/internal/model"
)

// newOrderTestService 不依赖 DB/MQ 的最小构造（CreateOrder 的库访问走不到，
// 仅测查库前的前置校验与纯函数；publisher nil 跳过发布）
func newOrderTestService() *Service {
	return New(nil, nil, testSecret, time.Hour, "", nil)
}

// TestCalcPoints 积分计算：1 元 1 分按金额向下取整
func TestCalcPoints(t *testing.T) {
	cases := []struct {
		amount float64
		want   int
	}{
		{1999.0, 1999},
		{399.5, 399},
		{0.99, 0},
		{899.99, 899},
	}
	for _, c := range cases {
		if got := CalcPoints(c.amount); got != c.want {
			t.Errorf("CalcPoints(%v) = %d, want %d", c.amount, got, c.want)
		}
	}
}

// TestGenOrderNo 订单号格式：14 位时间戳 + 4 位随机，共 18 位纯数字
func TestGenOrderNo(t *testing.T) {
	no := genOrderNo()
	if len(no) != 18 {
		t.Fatalf("order no length = %d, want 18 (%q)", len(no), no)
	}
	for _, ch := range no {
		if ch < '0' || ch > '9' {
			t.Fatalf("order no contains non-digit %q in %q", ch, no)
		}
	}
}

// TestCreateOrderPrecheck 创建订单前置校验（查库之前，repo=nil 安全）：
// 数量越界 → ErrInvalidQuantity；商品ID 非正 → ErrProductNotFound
func TestCreateOrderPrecheck(t *testing.T) {
	s := newOrderTestService()
	ctx := context.Background()

	cases := []struct {
		name      string
		req       model.OrderCreateReq
		wantError error
	}{
		{"数量为0", model.OrderCreateReq{ProductID: 1, Quantity: 0}, ErrInvalidQuantity},
		{"数量负数", model.OrderCreateReq{ProductID: 1, Quantity: -1}, ErrInvalidQuantity},
		{"数量超上限", model.OrderCreateReq{ProductID: 1, Quantity: 1000}, ErrInvalidQuantity},
		{"商品ID为0", model.OrderCreateReq{ProductID: 0, Quantity: 1}, ErrProductNotFound},
		{"商品ID负数", model.OrderCreateReq{ProductID: -1, Quantity: 1}, ErrProductNotFound},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := s.CreateOrder(ctx, c.req, 1)
			if !errors.Is(err, c.wantError) {
				t.Errorf("CreateOrder(%+v) err = %v, want %v", c.req, err, c.wantError)
			}
		})
	}
}
