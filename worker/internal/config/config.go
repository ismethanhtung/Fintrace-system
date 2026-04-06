// Package config loads và cung cấp cấu hình runtime cho worker.
// Tất cả giá trị đều đọc từ biến môi trường; nếu không có thì dùng giá trị mặc định.
package config

import (
	"os"
	"strconv"
	"strings"
)

// Config giữ toàn bộ cấu hình của worker.
type Config struct {
	// Redis
	RedisAddr     string
	RedisPassword string
	RedisDB       int

	// DNSE WebSocket
	DnseAPIKey    string
	DnseAPISecret string
	DnseWsURL     string

	// Danh sách board DNSE sẽ subscribe (G1=HOSE, G2=HNX, G3=UPCOM)
	DnseBoards []string

	// Market index DNSE sẽ subscribe
	DnseMarketIndexes []string

	// Danh sách symbol ticker cụ thể muốn nhận tick realtime từ DNSE.
	// Để trống = không subscribe tick per-symbol (chỉ subscribe market_index).
	DnseTickSymbols []string

	// Vietcap
	// Các nhóm cổ phiếu cần lấy snapshot (VN30, HOSE, HNX, UPCOM, ...)
	VietcapGroups []string

	// KB Securities - danh sách symbol cho intraday chart
	KbSymbols []string

	// Khoảng thời gian (giây) polling Vietcap snapshot
	VietcapSnapshotIntervalSec int

	// Khoảng thời gian (giây) polling Vietcap market index
	VietcapMarketIndexIntervalSec int

	// Khoảng thời gian (giây) polling KB intraday
	KbIntradayIntervalSec int
}

// Load đọc cấu hình từ environment variables.
func Load() *Config {
	return &Config{
		RedisAddr:     getEnv("REDIS_ADDR", "localhost:6379"),
		RedisPassword: getEnv("REDIS_PASSWORD", ""),
		RedisDB:       getEnvInt("REDIS_DB", 0),

		DnseAPIKey:    getEnv("DNSE_API_KEY", ""),
		DnseAPISecret: getEnv("DNSE_API_SECRET", ""),
		DnseWsURL:     getEnv("DNSE_WS_URL", "wss://ws-openapi.dnse.com.vn/v1/stream?encoding=json"),

		DnseBoards:        splitComma(getEnv("DNSE_BOARDS", "G1,G2,G3")),
		DnseMarketIndexes: splitComma(getEnv("DNSE_MARKET_INDEXES", "VNINDEX,VN30,HNXIndex,HNX30,HNXUpcomIndex")),
		DnseTickSymbols:   splitComma(getEnv("DNSE_TICK_SYMBOLS", "")),

		VietcapGroups: splitComma(getEnv("VIETCAP_GROUPS", "VN30,HOSE,HNX,UPCOM")),

		KbSymbols: splitComma(getEnv("KB_SYMBOLS", "VNINDEX,VN30,HNX30,HNXINDEX,UPCOM")),

		VietcapSnapshotIntervalSec:    getEnvInt("VIETCAP_SNAPSHOT_INTERVAL_SEC", 30),
		VietcapMarketIndexIntervalSec: getEnvInt("VIETCAP_MARKET_INDEX_INTERVAL_SEC", 30),
		KbIntradayIntervalSec:         getEnvInt("KB_INTRADAY_INTERVAL_SEC", 60),
	}
}

func getEnv(key, fallback string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return fallback
}

func getEnvInt(key string, fallback int) int {
	v := getEnv(key, "")
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return fallback
	}
	return n
}

func splitComma(s string) []string {
	var result []string
	for _, part := range strings.Split(s, ",") {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}
