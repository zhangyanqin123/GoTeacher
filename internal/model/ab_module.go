package model

// AB 版模块配置请求与行模型（ab_module / ab_module_item 表，见 PLAN-ab-module.md）。
// 两级结构：模块（H5 页面域，如 spacestation 空间站 / f10）→ 配置项（模块内 UI 块）→ 可见版本（mass 大众版 / data 数据版）。
//
// 约定：
//   - module_key / item_key 是 H5 代码引用的业务标识，创建后不可改（编辑请求不含此字段）
//   - item_key 的值是 camelCase 原文（如 topBanner），属业务数据不受 JSON 键名 snake_case 约束
//   - versions 库内存逗号串（'mass,data'），接口输出/输入均为字符串数组

// AbModule AB 版模块（父级）行。ItemCount 由列表查询子查询带出（不落列，bind_sales_count 同模式）
type AbModule struct {
	ID         int64          `json:"id"          db:"id"`
	ModuleKey  string         `json:"module_key"  db:"module_key"`
	ModuleName string         `json:"module_name" db:"module_name"`
	SortNo     int            `json:"sort_no"     db:"sort_no"`
	ItemCount  int            `json:"item_count"  db:"-"`
	CreatedAt  DateTimeString `json:"created_at"  db:"created_at"`
	UpdatedAt  DateTimeString `json:"updated_at"  db:"updated_at"`
}

// AbModuleOption 模块下拉项（GET /ab/modules/options，配置项管理页选父模块用）
type AbModuleOption struct {
	ID         int64  `json:"id"          db:"id"`
	ModuleKey  string `json:"module_key"  db:"module_key"`
	ModuleName string `json:"module_name" db:"module_name"`
}

// AbModuleItem AB 版模块配置项（子级）行。ModuleKey 由列表查询 JOIN 带出展示；
// Versions 库内为逗号串，repository 扫局部 string 后 Split 塞入（db:"-" 不直接扫描）
type AbModuleItem struct {
	ID         int64          `json:"id"         db:"id"`
	ModuleID   int64          `json:"module_id"  db:"module_id"`
	ModuleKey  string         `json:"module_key" db:"module_key"`
	ItemKey    string         `json:"item_key"   db:"item_key"`
	ItemName   string         `json:"item_name"  db:"item_name"`
	Versions   []string       `json:"versions"   db:"-"`
	SortNo     int            `json:"sort_no"    db:"sort_no"`
	CreatedAt  DateTimeString `json:"created_at" db:"created_at"`
	UpdatedAt  DateTimeString `json:"updated_at" db:"updated_at"`
}

// ---------- 模块管理请求 ----------

// AbModuleAddReq 新增模块。module_key 格式与唯一性在 service 校验（中文文案）
type AbModuleAddReq struct {
	ModuleKey  string `json:"module_key"  binding:"required,max=50"  example:"spacestation"`
	ModuleName string `json:"module_name" binding:"required,max=100" example:"空间站"`
	SortNo     int    `json:"sort_no"     example:"1"`
}

// AbModuleEditReq 编辑模块。不含 module_key：业务标识创建后不可改（H5 代码按此引用，改=断链）
type AbModuleEditReq struct {
	ID         int64  `json:"id"          binding:"required,gt=0"    example:"1"`
	ModuleName string `json:"module_name" binding:"required,max=100" example:"空间站"`
	SortNo     int    `json:"sort_no"     example:"1"`
}

// AbModuleDeleteReq 删除模块（模块下存在配置项时被 service 拦截，不级联删）
type AbModuleDeleteReq struct {
	ID int64 `json:"id" binding:"required,gt=0" example:"1"`
}

// AbModuleListReq 模块列表查询（POST body，module_key/module_name 均模糊匹配）
type AbModuleListReq struct {
	ModuleKey  string `json:"module_key"  example:"space"`
	ModuleName string `json:"module_name" example:"空间"`
	PageIndex  int    `json:"page_index"  example:"1"`
	PageSize   int    `json:"page_size"   example:"10"`
}

// AbModuleListFilter service 归一化分页后传给 repository
type AbModuleListFilter struct {
	ModuleKey  string
	ModuleName string
	Offset     int
	Limit      int
}

// ---------- 配置项管理请求 ----------

// AbModuleItemAddReq 新增配置项。versions 值域 mass/data 在 service 校验；
// item_key 创建后不可改
type AbModuleItemAddReq struct {
	ModuleID int64    `json:"module_id"  binding:"required,gt=0"                  example:"1"`
	ItemKey  string   `json:"item_key"   binding:"required,max=50"                example:"topBanner"`
	ItemName string   `json:"item_name"  binding:"required,max=100"               example:"顶部图"`
	Versions []string `json:"versions"   binding:"required,min=1,max=10,dive,max=20" example:"mass,data"`
	SortNo   int      `json:"sort_no"    example:"1"`
}

// AbModuleItemEditReq 编辑配置项。不含 item_key（业务标识不可改）；module_id 可改（挪模块）
type AbModuleItemEditReq struct {
	ID        int64    `json:"id"         binding:"required,gt=0"                  example:"1"`
	ModuleID  int64    `json:"module_id"  binding:"required,gt=0"                  example:"1"`
	ItemName  string   `json:"item_name"  binding:"required,max=100"               example:"顶部图"`
	Versions  []string `json:"versions"   binding:"required,min=1,max=10,dive,max=20" example:"mass,data"`
	SortNo    int      `json:"sort_no"    example:"1"`
}

// AbModuleItemDeleteReq 删除配置项
type AbModuleItemDeleteReq struct {
	ID int64 `json:"id" binding:"required,gt=0" example:"1"`
}

// AbModuleItemListReq 配置项列表查询。module_id 传 0/不传 = 不过滤
// （与「传 0 是有效过滤值」约定的差异：那条针对值域含 0 的 status 类字段，module_id 值域从 1 起）
type AbModuleItemListReq struct {
	ModuleID  int64  `json:"module_id"  example:"1"`
	ItemKey   string `json:"item_key"   example:"topBanner"`
	PageIndex int    `json:"page_index" example:"1"`
	PageSize  int    `json:"page_size"  example:"10"`
}

// AbModuleItemListFilter service 归一化分页后传给 repository
type AbModuleItemListFilter struct {
	ModuleID int64
	ItemKey  string
	Offset   int
	Limit    int
}
