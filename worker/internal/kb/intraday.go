// Package kb xử lý việc lấy dữ liệu intraday chart từ KB Securities và lưu vào Redis.
// API KB không cần authentication.
//
// Mapping symbol (UI name → KB API source symbol):
//   VNINDEX  → VNINDEX
//   VN30     → VN30
//   HNX30    → HNX30
//   HNXINDEX → HNXINDEX, HNX (fallback)
//   UPCOM    → UPCOMINDEX, HNXUPCOMINDEX, UPCOM (fallback)
package kb

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"

	"fintrace-worker/internal/rediskeys"
)

const (
	kbBaseURL     = "https://kbbuddywts.kbsec.com.vn/iis-server/investment/index"
	kbTimeoutSec  = 15
)

// symbolCandidates maps UI symbol name → danh sách source symbol thử theo thứ tự.
var symbolCandidates = map[string][]string{
	"VNINDEX":  {"VNINDEX"},
	"VN30":     {"VN30"},
	"HNX30":    {"HNX30"},
	"HNXINDEX": {"HNXINDEX", "HNX"},
	"UPCOM":    {"UPCOMINDEX", "HNXUPCOMINDEX", "UPCOM"},
}

// IntradayResult là kết quả intraday lưu vào Redis.
type IntradayResult struct {
	FetchedAt    string      `json:"fetchedAt"`
	UISymbol     string      `json:"uiSymbol"`
	SourceSymbol string      `json:"sourceSymbol"`
	Date         string      `json:"date"`
	Rows         interface{} `json:"rows"` // Giữ nguyên raw data từ KB
}

// FetchAndSaveAllSymbols lấy intraday cho tất cả symbol được cấu hình và lưu vào Redis.
// Symbols nhận vào là UI symbol names (VNINDEX, VN30, ...).
func FetchAndSaveAllSymbols(ctx context.Context, rdb *redis.Client, symbols []string) {
	date := todayVnDateString()

	type result struct {
		symbol string
		err    error
	}

	ch := make(chan result, len(symbols))

	for _, sym := range symbols {
		go func(symbol string) {
			err := fetchAndSaveSymbol(ctx, rdb, symbol, date)
			ch <- result{symbol: symbol, err: err}
		}(sym)
	}

	for range symbols {
		r := <-ch
		if r.err != nil {
			log.Printf("[kb] intraday error symbol=%s: %v", r.symbol, r.err)
		}
	}
}

// fetchAndSaveSymbol lấy intraday cho một symbol và lưu vào Redis.
func fetchAndSaveSymbol(ctx context.Context, rdb *redis.Client, uiSymbol, date string) error {
	uiSymbol = strings.ToUpper(strings.TrimSpace(uiSymbol))
	candidates, ok := symbolCandidates[uiSymbol]
	if !ok {
		// Symbol không có trong mapping cố định, thử dùng trực tiếp
		candidates = []string{uiSymbol}
	}

	var lastErr error
	for _, sourceSymbol := range candidates {
		result, err := fetchIntraday(ctx, sourceSymbol, date)
		if err != nil {
			lastErr = err
			continue
		}

		out := &IntradayResult{
			FetchedAt:    time.Now().UTC().Format(time.RFC3339),
			UISymbol:     uiSymbol,
			SourceSymbol: sourceSymbol,
			Date:         date,
			Rows:         result,
		}

		jsonBytes, err := json.Marshal(out)
		if err != nil {
			return fmt.Errorf("marshal: %w", err)
		}

		// Lưu theo ngày (TTL 24h)
		keyByDate := rediskeys.KbIntraday(uiSymbol, date)
		if err := rdb.Set(ctx, keyByDate, jsonBytes, rediskeys.TTLKbIntradaySec*time.Second).Err(); err != nil {
			return fmt.Errorf("redis set %s: %w", keyByDate, err)
		}

		// Lưu key "latest" để Next.js có thể đọc ngay
		keyLatest := rediskeys.KbIntradayLatest(uiSymbol)
		_ = rdb.Set(ctx, keyLatest, jsonBytes, rediskeys.TTLKbIntradaySec*time.Second)

		log.Printf("[kb] intraday saved: symbol=%s source=%s date=%s", uiSymbol, sourceSymbol, date)
		return nil
	}

	if lastErr != nil {
		return fmt.Errorf("tất cả candidates thất bại cho %s: %w", uiSymbol, lastErr)
	}
	return fmt.Errorf("không có data cho %s (ngày %s)", uiSymbol, date)
}

// fetchIntraday gọi KB API cho một source symbol.
func fetchIntraday(ctx context.Context, sourceSymbol, date string) (interface{}, error) {
	url := fmt.Sprintf("%s/%s/data_1P?sdate=%s&edate=%s",
		kbBaseURL, sourceSymbol, date, date,
	)

	reqCtx, cancel := context.WithTimeout(ctx, kbTimeoutSec*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d từ KB API", resp.StatusCode)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var payload map[string]interface{}
	if err := json.Unmarshal(data, &payload); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}

	rows, ok := payload["data_1P"]
	if !ok {
		return nil, fmt.Errorf("field data_1P không tồn tại trong response")
	}

	rowsArr, ok := rows.([]interface{})
	if !ok || len(rowsArr) == 0 {
		return nil, fmt.Errorf("data_1P rỗng hoặc không hợp lệ")
	}

	return rowsArr, nil
}

// todayVnDateString trả về ngày hôm nay theo timezone Việt Nam, định dạng DD-MM-YYYY.
// KB API yêu cầu ngày theo múi giờ VN (UTC+7).
func todayVnDateString() string {
	loc, err := time.LoadLocation("Asia/Ho_Chi_Minh")
	if err != nil {
		// Fallback nếu timezone không load được
		loc = time.FixedZone("ICT", 7*3600)
	}
	now := time.Now().In(loc)
	return fmt.Sprintf("%02d-%02d-%04d", now.Day(), now.Month(), now.Year())
}
