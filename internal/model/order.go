package model

// Order 订单行（订单系统 Demo，见 PLAN-order.md）
//
// 兼容约定（前端 gyz-admin orderDemo 页面直接展示，勿改）：
//   - Amount DECIMAL(10,2) 扫 float64，接口输出 399.0（对齐 diagnose.buy_price）
//   - Status 及三个 *_status 用 string：库里 TINYINT，接口输出 "1"/"0"（对齐 teacher.status 约定）
//   - 时间用 DateTimeString：扫描点格式化 "2006-01-02 15:04:05"
type Order struct {
	ID           int64          `json:"id"             db:"id"`
	OrderNo      string         `json:"order_no"       db:"order_no"`
	UserID       int64          `json:"user_id"        db:"user_id"`
	ProductID    int64          `json:"product_id"     db:"product_id"`
	ProductName  string         `json:"product_name"   db:"product_name"`
	Quantity     int            `json:"quantity"       db:"quantity"`
	Amount       float64        `json:"amount"         db:"amount"`
	Status       string         `json:"status"         db:"status"`         // 1处理中 2已完成 3已取消（库存不足）
	StockStatus  string         `json:"stock_status"   db:"stock_status"`   // 0待处理 1成功 2失败（order.stock 消费者回写）
	PointsStatus string         `json:"points_status" db:"points_status"`   // 0待处理 1成功 2失败（order.points 消费者回写）
	NotifyStatus string         `json:"notify_status" db:"notify_status"`   // 0待处理 1成功 2失败（order.notify 消费者回写）
	CreatedAt    DateTimeString `json:"created_at"     db:"created_at"`
	UpdatedAt    DateTimeString `json:"updated_at"     db:"updated_at"`
}

// Product 商品行（创建订单下拉与快照回查用）
type Product struct {
	ID          int64          `json:"id"           db:"id"`
	ProductName string         `json:"product_name" db:"product_name"`
	Price       float64        `json:"price"        db:"price"` // DECIMAL(10,2) 扫 float64
	Stock       int            `json:"stock"        db:"stock"`
	CreatedAt   DateTimeString `json:"created_at"   db:"created_at"`
	UpdatedAt   DateTimeString `json:"updated_at"   db:"updated_at"`
}

// PointsRecord 积分行（order.points 消费者写入，order_id 唯一键 = MQ 消息幂等）
type PointsRecord struct {
	ID        int64          `json:"id"         db:"id"`
	UserID    int64          `json:"user_id"    db:"user_id"`
	OrderID   int64          `json:"order_id"   db:"order_id"`
	OrderNo   string         `json:"order_no"   db:"order_no"`
	Points    int            `json:"points"     db:"points"`
	Remark    string         `json:"remark"     db:"remark"`
	CreatedAt DateTimeString `json:"created_at" db:"created_at"`
}

// Notification 通知行（order.notify 消费者写入，order_id 唯一键 = MQ 消息幂等）
type Notification struct {
	ID        int64          `json:"id"         db:"id"`
	UserID    int64          `json:"user_id"    db:"user_id"`
	OrderID   int64          `json:"order_id"   db:"order_id"`
	Title     string         `json:"title"      db:"title"`
	Content   string         `json:"content"    db:"content"`
	IsRead    string         `json:"is_read"    db:"is_read"` // "1"/"0"
	CreatedAt DateTimeString `json:"created_at" db:"created_at"`
}

// OrderCreatedEvent order.created 事件体（RabbitMQ fanout 广播给三队列，JSON 编码）。
// 冗余 product_name/amount 快照：消费者不回查商品表，消息自包含。
type OrderCreatedEvent struct {
	OrderID     int64   `json:"order_id"`
	OrderNo     string  `json:"order_no"`
	UserID      int64   `json:"user_id"`
	ProductID   int64   `json:"product_id"`
	ProductName string  `json:"product_name"`
	Quantity    int     `json:"quantity"`
	Amount      float64 `json:"amount"`
}

// OrderInsert 创建订单落库参数（service 组装完快照/金额后传给 repository）
type OrderInsert struct {
	OrderNo     string
	UserID      int64
	ProductID   int64
	ProductName string
	Quantity    int
	Amount      float64
}

// PointsInsert 积分落库参数（order.points 消费者组装）
type PointsInsert struct {
	UserID  int64
	OrderID int64
	OrderNo string
	Points  int
	Remark  string
}

// NotificationInsert 通知落库参数（order.notify 消费者组装）
type NotificationInsert struct {
	UserID  int64
	OrderID int64
	Title   string
	Content string
}

// 订单状态机常量（库里 TINYINT，接口层以字符串输出）
const (
	OrderStatusProcessing int = 1 // 处理中（三步骤未全部回写）
	OrderStatusDone       int = 2 // 已完成（stock/points/notify 三列全 1）
	OrderStatusCancelled  int = 3 // 已取消（库存不足）
)

// 消费步骤列（orders 表回写列白名单，repository.MarkOrderStepDone 据此拼 SQL）
const (
	OrderStepStock  = "stock_status"
	OrderStepPoints = "points_status"
	OrderStepNotify = "notify_status"
)

// OrderCreateReq 创建订单请求体（POST /orders）。
// 商品名/金额等冗余字段不接收，一律从 product 表回查（单一事实来源）。
type OrderCreateReq struct {
	ProductID int64 `json:"product_id"` // 商品ID
	Quantity  int   `json:"quantity"`   // 购买数量 1-999
}

// 商品管理请求体（product CRUD，POST /products/*，见 PLAN-product-crud.md）。
// price=0 被 required 拒（demo 无 0 元商品）；stock=0 合法（售罄），故不加 required。

// ProductAddReq 新增商品
type ProductAddReq struct {
	ProductName string  `json:"product_name" binding:"required,max=100" example:"无线充电板"`
	Price       float64 `json:"price"        binding:"required,gt=0"      example:"129.00"`
	Stock       int     `json:"stock"        binding:"gte=0"              example:"500"`
}

// ProductEditReq 编辑商品
type ProductEditReq struct {
	ID          int64   `json:"id"           binding:"required,gt=0"      example:"5"`
	ProductName string  `json:"product_name" binding:"required,max=100" example:"无线充电板"`
	Price       float64 `json:"price"        binding:"required,gt=0"      example:"129.00"`
	Stock       int     `json:"stock"        binding:"gte=0"              example:"500"`
}

// ProductDeleteReq 删除商品
type ProductDeleteReq struct {
	ID int64 `json:"id" binding:"required,gt=0" example:"5"`
}

// ProductListReq 商品列表查询（POST body，同 admin/user 先例）
type ProductListReq struct {
	ProductName string `json:"product_name" example:"键盘"`
	PageIndex   int    `json:"page_index"   example:"1"`
	PageSize    int    `json:"page_size"    example:"10"`
}

// ProductListFilter service 归一化分页后传给 repository
type ProductListFilter struct {
	ProductName string
	Offset      int
	Limit       int
}

// OrderListReq 订单列表查询请求体（POST /orders/list）。
// 数值筛选用指针：null/缺省不过滤，传 0 是有效过滤值（status 白名单 1/2/3，0 会被 handler 拒）。
type OrderListReq struct {
	OrderNo     string `json:"order_no"`     // 精确
	ProductName string `json:"product_name"` // 模糊，匹配快照列
	Status      *int   `json:"status"`       // 精确 1/2/3
	PageIndex   int    `json:"page_index"`   // 默认 1（service 层兜底）
	PageSize    int    `json:"page_size"`    // 默认 10，上限 100
}

// OrderListFilter 订单列表查询条件（零值字段不参与过滤）
type OrderListFilter struct {
	OrderNo     string
	ProductName string
	Status      int
	PageIndex   int
	PageSize    int
}

// PointsListReq 积分列表查询请求体（POST /points/list）
type PointsListReq struct {
	OrderNo   string `json:"order_no"`   // 精确，匹配快照列
	PageIndex int    `json:"page_index"` // 默认 1
	PageSize  int    `json:"page_size"`  // 默认 10，上限 100
}

// PointsListFilter 积分列表查询条件
type PointsListFilter struct {
	OrderNo   string
	PageIndex int
	PageSize  int
}

// NotificationListReq 通知列表查询请求体（POST /notifications/list）
type NotificationListReq struct {
	Title     string `json:"title"`     // 模糊
	PageIndex int    `json:"page_index"` // 默认 1
	PageSize  int    `json:"page_size"`  // 默认 10，上限 100
}

// NotificationListFilter 通知列表查询条件
type NotificationListFilter struct {
	Title     string
	PageIndex int
	PageSize  int
}
