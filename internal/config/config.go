package config

import (
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/go-sql-driver/mysql"
	"github.com/joho/godotenv"
)

// Config 应用配置：由环境变量 + 默认值组成
type Config struct {
	DBHost     string
	DBPort     string
	DBUser     string
	DBPassword string
	DBName     string
	ServerPort string
	LogLevel   string // 日志级别：debug/info/warn/error，debug 时输出全部 SQL 执行日志
	DSN        string // MySQL 连接串，由 mysql.Config 组装

	RedisAddr     string // Redis 地址（鉴权白名单）
	RedisPassword string
	RedisDB       int
	JWTSecret     string // JWT HS256 签发密钥（必填，空值启动退出）
	JWTTTLHours   int    // JWT 有效期（小时），同时是 Redis 白名单 TTL

	XiaoeAPIBase string // 小鹅通开放平台 API 域名（直播登录链接透传的上游 base，见 PLAN-live.md）

	RabbitMQURL string // RabbitMQ 连接串（订单事件 order.created 发布/消费，见 PLAN-order.md）

	// 前端监控告警（mon_event/mon_alert，见 PLAN-frontend-monitor.md）
	MonAlertEnabled            bool     // 告警扫描任务开关（false 时 main 不起 goroutine）
	MonAlertIntervalMin        int      // 扫描间隔（分钟）
	MonAlertWindowMin          int      // 统计滚动窗口（分钟）
	MonAlertEnvs               []string // 参与告警的环境（默认仅 production，test 刷数据不告警）
	MonAlertProbeFailThreshold int      // PROBE_FAIL 触发阈值（窗口内 > 阈值；默认 0 即 >0）
	MonAlertChunkThreshold     int      // CHUNK_LOAD_SURGE 阈值
	MonAlertErrorThreshold     int      // ERROR_SURGE 阈值（win+vue+unhandled 合计）
	MonRetentionDays           int      // mon_event 保留天数（每日分批清理）
	MonAlertWebhookURL         string   // 告警 webhook（空=仅落表不推送）
	MonEventPersistEnabled     bool     // 事件落库开关（默认 true；false 时不上报入库、chunk_load_error 直推企微不依赖 DB/ticker）
}

// Load 加载配置并组装 DSN。
// 支持两种取值来源：进程环境变量，或项目根目录 .env 文件（godotenv）。
func Load() *Config {
	// .env 不存在时忽略错误，不影响使用
	_ = godotenv.Load()

	c := &Config{
		DBHost:     getEnv("DB_HOST", "127.0.0.1"),
		DBPort:     getEnv("DB_PORT", "3306"),
		DBUser:     getEnv("DB_USER", "root"),
		DBPassword: getEnv("DB_PASSWORD", ""),
		DBName:     getEnv("DB_NAME", "gyz_db"),
		ServerPort: getEnv("SERVER_PORT", "8080"),
		LogLevel:   getEnv("LOG_LEVEL", "info"),

		RedisAddr:     getEnv("REDIS_ADDR", "127.0.0.1:6379"),
		RedisPassword: getEnv("REDIS_PASSWORD", ""),
		RedisDB:       getEnvInt("REDIS_DB", 0),
		JWTSecret:     getEnv("JWT_SECRET", ""),
		JWTTTLHours:   getEnvInt("JWT_TTL_HOURS", 24),

		XiaoeAPIBase: getEnv("XIAOE_API_BASE", "https://api.xiaoe-tech.com"),

		RabbitMQURL: getEnv("RABBITMQ_URL", "amqp://guest:guest@127.0.0.1:5672/"),

		MonAlertEnabled:            getEnvBool("MON_ALERT_ENABLED", true),
		MonAlertIntervalMin:        getEnvInt("MON_ALERT_INTERVAL_MIN", 5),
		MonAlertWindowMin:          getEnvInt("MON_ALERT_WINDOW_MIN", 10),
		MonAlertEnvs:               splitComma(getEnv("MON_ALERT_ENVS", "production")),
		MonAlertProbeFailThreshold: getEnvInt("MON_ALERT_PROBE_FAIL_THRESHOLD", 0),
		MonAlertChunkThreshold:     getEnvInt("MON_ALERT_CHUNK_THRESHOLD", 10),
		MonAlertErrorThreshold:     getEnvInt("MON_ALERT_ERROR_THRESHOLD", 50),
		MonRetentionDays:           getEnvInt("MON_RETENTION_DAYS", 90),
		MonAlertWebhookURL:         getEnv("MON_ALERT_WEBHOOK_URL", ""),
		MonEventPersistEnabled:     getEnvBool("MON_EVENT_PERSIST_ENABLED", true),
	}

	mc := mysql.Config{
		User:      c.DBUser,
		Passwd:    c.DBPassword,
		Net:       "tcp",
		Addr:      c.DBHost + ":" + c.DBPort,
		DBName:    c.DBName,
		ParseTime: true,       // 必须：让 DATETIME 列扫描进 time.Time，否则报错
		Loc:       time.Local, // Go 侧时区（MySQL 连接默认 utf8mb4 字符集）
	}
	c.DSN = mc.FormatDSN()
	return c
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// getEnvInt 整型环境变量：缺失/非法值回落默认（非法不报错，保持与其他配置一致的无害降级）
func getEnvInt(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return fallback
}

// getEnvBool 布尔环境变量：仅 "true"/"1" 视为真，其余（含缺失）回落默认
func getEnvBool(key string, fallback bool) bool {
	if v := os.Getenv(key); v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			return b
		}
	}
	return fallback
}

// splitComma 逗号分隔串转切片（去空白/空项；空串返回空切片）
func splitComma(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}
