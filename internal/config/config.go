package config

import (
	"os"
	"strconv"
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
		DBName:     getEnv("DB_NAME", "handicap_db"),
		ServerPort: getEnv("SERVER_PORT", "8080"),
		LogLevel:   getEnv("LOG_LEVEL", "info"),

		RedisAddr:     getEnv("REDIS_ADDR", "127.0.0.1:6379"),
		RedisPassword: getEnv("REDIS_PASSWORD", ""),
		RedisDB:       getEnvInt("REDIS_DB", 0),
		JWTSecret:     getEnv("JWT_SECRET", ""),
		JWTTTLHours:   getEnvInt("JWT_TTL_HOURS", 24),

		XiaoeAPIBase: getEnv("XIAOE_API_BASE", "https://api.xiaoe-tech.com"),
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
