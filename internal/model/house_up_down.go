package model

import "time"

// HouseUpDown 涨跌家数分布统计（对应 Spring Boot 的 Entity/DTO）
//
// json tag 与接口字段名完全一致，直接用于响应序列化；
// db tag 标注列名（snake_case），便于阅读与将来接入 sqlx。
type HouseUpDown struct {
	ID           int64     `json:"-"                            db:"id"`
	SecuMarket   string    `json:"-"                            db:"secu_market"`
	Range        string    `json:"-"                            db:"stat_range"`
	Above7       int       `json:"above7"                       db:"above7"`
	Between5_7   int       `json:"between5_7"                   db:"between5_7"`
	Between3_5   int       `json:"between3_5"                   db:"between3_5"`
	Between0_3   int       `json:"between0_3"                   db:"between0_3"`
	Equal0       int       `json:"equal0"                       db:"equal0"`
	BetweenN3_0  int       `json:"betweenN3_0"                  db:"between_n3_0"`
	BetweenN5_N3 int       `json:"betweenN5_N3"                 db:"between_n5_n3"`
	BetweenN7_N5 int       `json:"betweenN7_N5"                 db:"between_n7_n5"`
	BelowN7      int       `json:"belowN7"                      db:"below_n7"`
	Total        int       `json:"total"                        db:"total"`
	UpCount      int       `json:"upCount"                      db:"up_count"`
	DownCount    int       `json:"downCount"                    db:"down_count"`
	FlatCount    int       `json:"flatCount"                    db:"flat_count"`
	StatDate     string    `json:"-"                            db:"stat_date"`
	CreatedAt    time.Time `json:"-"                            db:"created_at"`
	UpdatedAt    time.Time `json:"-"                            db:"updated_at"`
}