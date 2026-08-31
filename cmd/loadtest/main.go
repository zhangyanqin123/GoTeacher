// Package main 抢票场景压测工具(订单系统 Demo,设计见 plans/PLAN-loadtest.md):
// 模拟「瞬时大量并发、每次限购 1 件」的抢票流量,验证库存不击穿。
//
// 流程:登录拿 token → 直连 MySQL 记初始库存 → worker pool 并发 POST /orders(quantity 固定 1)
//   → 轮询等待库存/积分/通知三队列消费完 → 守恒校验:
//   剩余库存 == 初始库存 - 成功订单件数(status IN (1,2)),成立即「未击穿」。
//
// 统计口径:HTTP 200 只是下单受理(订单落库 status=1),最终成败以 DB 订单状态为准——
// 异步消费可能将订单转 status=3 已取消(库存不足回滚),压测侧计数仅作参考。
// 压测期间可开 http://localhost:15672 直观观察三队列堆积与消费曲线。
package main

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"handicap-service/internal/config"
	"handicap-service/internal/database"
)

// apiResp 通用响应骨架(只看 code/msg 分类,不关心 data)
type apiResp struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
}

// loginResp login 响应特例契约:token 在响应根而非 data 内(见 plans/PLAN-auth.md)
type loginResp struct {
	Code  int    `json:"code"`
	Msg   string `json:"msg"`
	Token string `json:"token"`
}

// stats HTTP 侧请求结果分类计数(atomic 并发累加;其他类错误只打首条样例定位问题)
type stats struct {
	accepted     atomic.Int64 // code=200 下单受理
	insufficient atomic.Int64 // code=400 且 msg=库存不足(下单预检拦截)
	other        atomic.Int64 // 请求体非法 / 5xx / 网络错误
	done         atomic.Int64 // 已完成的请求总数
	sampleOnce   sync.Once
}

func (s *stats) noteSample(format string, a ...any) {
	s.sampleOnce.Do(func() { fmt.Printf("  [其他失败样例] %s\n", fmt.Sprintf(format, a...)) })
}

func main() {
	baseURL := flag.String("base-url", "http://localhost:8080", "服务地址")
	username := flag.String("username", "admin", "登录账号")
	password := flag.String("password", "admin123", "登录密码")
	productID := flag.Int64("product", 1, "抢购商品ID")
	total := flag.Int("total", 3000, "总请求数")
	concurrency := flag.Int("concurrency", 50, "并发 worker 数")
	waitTimeout := flag.Duration("wait-timeout", 300*time.Second, "等待异步消费完成的超时")
	flag.Parse()
	if *total <= 0 || *concurrency <= 0 {
		fmt.Println("total 与 concurrency 必须为正整数")
		os.Exit(1)
	}

	// 1. 连 MySQL(复用 .env 配置;压测工具只读 product/orders 做校验,不写业务表)
	cfg := config.Load()
	db, err := database.Connect(cfg)
	if err != nil {
		fmt.Printf("连接 MySQL 失败(请在项目根目录跑,读同一 .env):%v\n", err)
		os.Exit(1)
	}
	defer db.Close()

	var productName string
	var initialStock int
	if err := db.QueryRow("SELECT product_name, stock FROM product WHERE id = ?", *productID).
		Scan(&productName, &initialStock); err == sql.ErrNoRows {
		fmt.Printf("商品 %d 不存在\n", *productID)
		os.Exit(1)
	} else if err != nil {
		fmt.Printf("查询商品失败:%v\n", err)
		os.Exit(1)
	}

	// 2. 登录拿 token(JWT Bearer;TTL 24h,压测期间不会过期)
	client := &http.Client{
		Timeout: 10 * time.Second,
		Transport: &http.Transport{MaxIdleConns: *concurrency, MaxIdleConnsPerHost: *concurrency},
	}
	token, err := login(client, *baseURL, *username, *password)
	if err != nil {
		fmt.Printf("登录失败:%v\n", err)
		os.Exit(1)
	}

	// 3. worker pool 并发下单(quantity 固定 1:抢票限购约束,不暴露为参数)
	start := time.Now()
	fmt.Printf("开始抢票压测:商品 #%d %s / 并发 %d / 总请求 %d / 初始库存 %d\n\n",
		*productID, productName, *concurrency, *total, initialStock)

	st := &stats{}
	jobs := make(chan struct{})
	var wg sync.WaitGroup
	for i := 0; i < *concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range jobs {
				placeOrder(client, *baseURL, token, *productID, st)
			}
		}()
	}
	// 进度播报:每 2s 打一行,避免几千行刷屏
	progressDone := make(chan struct{})
	go func() {
		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-progressDone:
				return
			case <-ticker.C:
				fmt.Printf("  进度:%d/%d(受理 %d / 库存不足 %d / 其他 %d)\n",
					st.done.Load(), *total, st.accepted.Load(), st.insufficient.Load(), st.other.Load())
			}
		}
	}()
	for i := 0; i < *total; i++ {
		jobs <- struct{}{}
	}
	close(jobs)
	wg.Wait()
	close(progressDone)
	orderElapsed := time.Since(start)
	tps := float64(*total) / orderElapsed.Seconds()

	// 4. 轮询等待三队列消费完。「消费完」的口径(与消费者回写语义对齐):
	//    - stock_status 每单必回写(成功=1 / 库存不足取消=2),是消化进度的主锚点;
	//    - 已取消单(status=3)的 points/notify 步骤按设计跳过不回写(列恒 0),不能纳入等待,
	//      否则小库存抢购场景大量取消单会让条件永远清不了零;
	//    - 未取消单(status 1/2)要求三列全部回写。
	//    限定压测开始后的订单,残留历史数据不干扰;DATETIME 秒级精度,回退 1s 兜底边界。
	fmt.Println("\n等待库存/积分/通知三队列异步消费……(可在 http://localhost:15672 观察队列堆积回落)")
	consumed := true
	deadline := time.Now().Add(*waitTimeout)
	for {
		var pending int
		if err := db.QueryRow(`SELECT COUNT(*) FROM orders
			WHERE created_at >= ?
			  AND (stock_status = 0
			       OR (status IN (1,2) AND (points_status = 0 OR notify_status = 0)))`,
			start.Add(-time.Second)).Scan(&pending); err != nil {
			fmt.Printf("轮询订单状态失败:%v\n", err)
			os.Exit(1)
		}
		if pending == 0 {
			break
		}
		if time.Now().After(deadline) {
			consumed = false
			fmt.Printf("  超时(%.0fs):仍有 %d 单未消费完,数据可能不完整\n", waitTimeout.Seconds(), pending)
			break
		}
		fmt.Printf("  剩余待处理订单:%d\n", pending)
		time.Sleep(2 * time.Second)
	}

	// 5. 守恒校验:成功订单件数以 DB 为准(status IN (1,2) 均占用库存;status=3 已取消不占,
	//    库存消费者扣不动时已回滚积分/通知)
	var sold, processing, doneCnt, cancelled int64
	queries := []struct {
		sql  string
		dest *int64
	}{
		{"SELECT IFNULL(SUM(quantity),0) FROM orders WHERE product_id = ? AND created_at >= ? AND status IN (1,2)", &sold},
		{"SELECT COUNT(*) FROM orders WHERE product_id = ? AND created_at >= ? AND status = 1", &processing},
		{"SELECT COUNT(*) FROM orders WHERE product_id = ? AND created_at >= ? AND status = 2", &doneCnt},
		{"SELECT COUNT(*) FROM orders WHERE product_id = ? AND created_at >= ? AND status = 3", &cancelled},
	}
	for _, q := range queries {
		if err := db.QueryRow(q.sql, *productID, start.Add(-time.Second)).Scan(q.dest); err != nil {
			fmt.Printf("统计订单失败:%v(SQL: %s)\n", err, q.sql)
			os.Exit(1)
		}
	}
	var finalStock int
	if err := db.QueryRow("SELECT stock FROM product WHERE id = ?", *productID).Scan(&finalStock); err != nil {
		fmt.Printf("查询剩余库存失败:%v\n", err)
		os.Exit(1)
	}

	expected := initialStock - int(sold)
	pass := finalStock == expected && int(sold) <= initialStock

	fmt.Printf(`
================ 压测汇总 ================
目标商品        : #%d %s
并发 / 总请求   : %d / %d
下单耗时        : %.1fs(实际 TPS %.1f)
HTTP 受理成功   : %d(code=200)
预检库存不足    : %d(code=400)
其他失败        : %d
最终订单分布    : 处理中 %d / 已完成 %d / 已取消 %d(成功占用库存 %d 件)
--- 守恒校验(不击穿判定)---
初始库存        : %d
剩余库存(实际) : %d
剩余库存(理论) : %d(初始 %d - 成功件数 %d)
`,
		*productID, productName, *concurrency, *total,
		orderElapsed.Seconds(), tps,
		st.accepted.Load(), st.insufficient.Load(), st.other.Load(),
		processing, doneCnt, cancelled, sold,
		initialStock, finalStock, expected, initialStock, sold)

	switch {
	case !consumed:
		fmt.Println("✗ 消费超时:请查看 consumer 进程与 15672 队列状态后重跑")
		os.Exit(2)
	case pass:
		fmt.Println("✓ 未击穿:库存守恒,成功下单件数未超过库存")
		os.Exit(0)
	default:
		fmt.Printf("✗ 击穿:实际剩余 %d ≠ 理论 %d,或成功件数 %d 超过初始库存 %d\n",
			finalStock, expected, sold, initialStock)
		os.Exit(1)
	}
}

// login 调 /api/v1/login 拿 token。注意 login 特例:失败也返回 HTTP 200 + code 400。
func login(client *http.Client, baseURL, username, password string) (string, error) {
	body, _ := json.Marshal(map[string]string{"username": username, "password": password})
	resp, err := client.Post(baseURL+"/api/v1/login", "application/json", bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("请求失败: %w", err)
	}
	defer resp.Body.Close()
	var lr loginResp
	if err := json.NewDecoder(resp.Body).Decode(&lr); err != nil {
		return "", fmt.Errorf("解析响应: %w", err)
	}
	if lr.Code != 200 || lr.Token == "" {
		return "", fmt.Errorf("code=%d msg=%s", lr.Code, lr.Msg)
	}
	return lr.Token, nil
}

// placeOrder 发一笔下单请求并归类结果。
// 失败分类:code=400 且 msg=库存不足 → 预检拦截;其余非受理结果统一归「其他」并打样例。
func placeOrder(client *http.Client, baseURL, token string, productID int64, st *stats) {
	body, _ := json.Marshal(map[string]any{"product_id": productID, "quantity": 1})
	req, err := http.NewRequest(http.MethodPost, baseURL+"/api/v1/orders", bytes.NewReader(body))
	if err != nil {
		st.other.Add(1)
		st.noteSample("构造请求失败: %v", err)
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := client.Do(req)
	if err != nil {
		st.other.Add(1)
		st.noteSample("网络错误: %v", err)
		return
	}
	defer resp.Body.Close()
	var r apiResp
	_ = json.NewDecoder(resp.Body).Decode(&r)
	switch {
	case resp.StatusCode == http.StatusOK && r.Code == 200:
		st.accepted.Add(1)
	case resp.StatusCode == http.StatusBadRequest && r.Msg == "库存不足":
		st.insufficient.Add(1)
	default:
		st.other.Add(1)
		st.noteSample("http=%d code=%d msg=%q", resp.StatusCode, r.Code, r.Msg)
	}
	st.done.Add(1)
}
