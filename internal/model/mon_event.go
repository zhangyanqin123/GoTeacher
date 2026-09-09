package model

import "encoding/json"

// 前端监控事件（mon_event 表，见 PLAN-frontend-monitor.md）。
// 数据源：gyz-h5-personalcenter 内联探针 window.__GYZMON__ 的 gif GET 上报
// （GET /mon/event?d=<urlencoded JSON>，payload 字段为探针短名 t/cap/...）。

// MonEventIngest 探针上报 payload（json tag 对齐探针字段短名，不能改——改=断链）。
// 探针端只保证 t 有效，其余字段可能缺省；长度上限由探针端截断 + service 二次截断双保险。
// Cap 是探针的能力探测结果对象（{"fnok":1,...}），用 RawMessage 原样接收、落库时转串
type MonEventIngest struct {
	T      string          `json:"t"      example:"chunk_load_error"`
	Proj   string          `json:"proj"   example:"personalCenter"`
	Cap    json.RawMessage `json:"cap"    example:"{\"fnok\":1,\"nullish\":1,\"optchain\":1}"`
	Capbad string          `json:"capbad" example:"syntax,at"`
	Ua     string          `json:"ua"     example:"Mozilla/5.0 (Linux; Android 12) ... Chrome/77.0"`
	Cv     int             `json:"cv"     example:"77"`
	Osv    string          `json:"osv"    example:"iOS 18.5"`
	Wv     string          `json:"wv"     example:"TBS046011"`
	Net    string          `json:"net"    example:"WIFI"`
	Ver    string          `json:"ver"    example:"1.2.22.0+126ef6a"`
	Env    string          `json:"env"    example:"production"`
	Rt     string          `json:"rt"     example:"#/pages/produPkg/index"`
	Sid    string          `json:"sid"    example:"mtqzwhcqjelh6q"`
	Seq    int             `json:"seq"    example:"2"`
	Mdl    string          `json:"mdl"    example:"HBN-AL00"`
	Os     string          `json:"os"     example:"HarmonyOS 4.0"`
	Msg    string          `json:"msg"    example:"Unexpected token '??>'"`
	Stack  string          `json:"stack"  example:"SyntaxError: ...\n    at ..."`
	Src    string          `json:"src"    example:"https://cdn.example.com/assets/pages-index-abc.js"`
}

// MonEventRow mon_event 行模型。
// 列表查询不 SELECT stack/cap/ua 全文（msg 由 SQL LEFT 截断为预览），详情查询单独补齐
type MonEventRow struct {
	ID          int64          `json:"id"           db:"id"`
	Project     string         `json:"project"      db:"project"`
	EventType   string         `json:"event_type"   db:"event_type"`
	Env         string         `json:"env"          db:"env"`
	Ver         string         `json:"ver"          db:"ver"`
	ChromeVer   int            `json:"chrome_ver"   db:"chrome_ver"`
	Osv         string         `json:"osv"          db:"osv"`
	Webview     string         `json:"webview"      db:"webview"`
	NetType     string         `json:"net_type"     db:"net_type"`
	DeviceModel string         `json:"device_model" db:"device_model"`
	DeviceBrand string         `json:"device_brand" db:"-"` // 代号→品牌（service 填充，展示用；不改写原始代号）
	Os          string         `json:"os"           db:"os"`
	Route       string         `json:"route"        db:"route"`
	SessionID   string         `json:"session_id"   db:"session_id"`
	Seq         int            `json:"seq"          db:"seq"`
	Capbad      string         `json:"capbad"       db:"capbad"`
	CapSyntax   int            `json:"cap_syntax"   db:"cap_syntax"`
	Src         string         `json:"src"          db:"src"`
	Ip          string         `json:"ip"           db:"ip"`
	CreatedAt   DateTimeString `json:"created_at"   db:"created_at"`
	// 列表态附带：msg 前 100 字预览（详情态为全文）
	Msg  string `json:"msg"           db:"msg"`
	Ua   string `json:"ua,omitempty"  db:"ua"`   // 仅详情查询填充
	Cap  string `json:"cap,omitempty" db:"cap"`  // 仅详情查询填充
	Stack string `json:"stack,omitempty" db:"stack"` // 仅详情查询填充
}

// MonIngestResp POST 通道回执：accepted 入库条数 / rejected 清洗剔除条数（含批量截断）
type MonIngestResp struct {
	Accepted int `json:"accepted" example:"2"`
	Rejected int `json:"rejected" example:"0"`
}

// ---------- 查询/概览（管理台，Auth 组接口） ----------

// MonEventListReq 事件多口径查询（除 begin/end 外全可选）。
// begin/end 后端强制：缺省回退近 24h，跨度 >31 天拒绝（防全表扫描）
type MonEventListReq struct {
	Project     string   `json:"project"       example:"personalCenter"` // 精确匹配；空=不过滤
	Begin       string   `json:"begin"        example:"2026-09-07 00:00:00"`
	End         string   `json:"end"          example:"2026-09-07 23:59:59"`
	EventTypes  []string `json:"event_types"  example:"chunk_load_error,probe_fail"`
	Env         string   `json:"env"          example:"production"`
	Ver         string   `json:"ver"          example:"1.2.22"`     // 前缀匹配（忽略 commit hash）
	ChromeVer   int      `json:"chrome_ver"   example:"77"`         // 0=不过滤
	SessionID   string   `json:"session_id"   example:"mtqzwhcqjelh6q"`
	DeviceModel string   `json:"device_model" example:"HBN-AL00"`
	Capbads     []string `json:"capbads"      example:"syntax,at"`  // FIND_IN_SET 语义（无序逗号串）
	CapSyntax   int      `json:"cap_syntax"   example:"1"`          // 0=不过滤 1=仅语法不兼容机型
	Route       string   `json:"route"        example:"produPkg"`
	Msg         string   `json:"msg"          example:"Unexpected token"`
	Src         string   `json:"src"          example:"chunk-"`
	PageIndex   int      `json:"page_index"   example:"1"`
	PageSize    int      `json:"page_size"    example:"20"`
}

// MonEventListFilter service 归一化（时间校验+分页）后传 repository
type MonEventListFilter struct {
	Project     string
	Begin       string
	End         string
	EventTypes  []string
	Env         string
	VerPrefix   string
	ChromeVer   int
	SessionID   string
	DeviceModel string
	Capbads     []string
	CapSyntax   int
	Route       string
	Msg         string
	Src         string
	Offset      int
	Limit       int
}

// MonOverviewReq 概览查询。时间缺省回退近 24h，跨度 >7 天拒绝（聚合成本控制）
type MonOverviewReq struct {
	Begin string `json:"begin" example:"2026-09-06 20:00:00"`
	End   string `json:"end"   example:"2026-09-07 20:00:00"`
	Env   string `json:"env"   example:"production"`
}

// MonOverviewSummary 概览指标卡
type MonOverviewSummary struct {
	Total    int            `json:"total"`             // 全部事件
	Errors   int            `json:"errors"`            // 错误类事件（total − boot − device）
	Sessions int            `json:"sessions"`          // 去重会话数
	Devices  int            `json:"devices"`           // 去重机型数
	Syntax   int            `json:"syntax"`            // 语法不兼容事件（cap_syntax=1）
	ByType   map[string]int `json:"by_type"`           // 按事件类型计数
}

// MonGroupRow 通用聚合行（Key 为分组值，Extra 复用作 by_mdl 的语法不兼容数）
type MonGroupRow struct {
	Key   string `json:"key"   example:"HBN-AL00"`
	Count int    `json:"count" example:"80"`
	Extra int    `json:"extra,omitempty" example:"3"` // by_mdl 复用作语法不兼容数
	Brand string `json:"brand,omitempty" example:"华为/荣耀"` // by_mdl 的代号品牌（service 填充）
}

// MonOverviewResp 概览响应：指标卡 + 五个 Top 聚合
type MonOverviewResp struct {
	Summary MonOverviewSummary `json:"summary"`
	ByVer   []MonGroupRow      `json:"by_ver"`   // Top10 版本（版本相关故障定位）
	ByMdl   []MonGroupRow      `json:"by_mdl"`   // Top10 机型（Extra=语法不兼容数）
	ByCv    []MonGroupRow      `json:"by_cv"`    // 内核版本分布（升序，低内核置顶；0=iOS/未知）
	ByRoute []MonGroupRow      `json:"by_route"` // Top10 路由
	BySrc   []MonGroupRow      `json:"by_src"`   // resource_error Top10 资源 URL
}
