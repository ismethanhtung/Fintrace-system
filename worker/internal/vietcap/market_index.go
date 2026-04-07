package vietcap

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"

	"github.com/redis/go-redis/v9"

	"fintrace-worker/internal/rediskeys"
)

const (
	vietcapMarketIndexURL = "https://trading.vietcap.com.vn/api/price/marketIndex/getList"
	marketIndexTimeoutSec = 15
)

// defaultMarketIndexSymbols là danh sách mặc định nếu không cấu hình riêng.
var defaultMarketIndexSymbols = []string{
	"VNINDEX", "VN30", "HNXIndex", "HNX30", "HNXUpcomIndex", "VNXALL",
}

// MarketIndexResult là kết quả lấy danh sách market index.
type MarketIndexResult struct {
	FetchedAt string                   `json:"fetchedAt"`
	Count     int                      `json:"count"`
	Rows      []map[string]interface{} `json:"rows"`
}

// FetchAndSaveMarketIndex lấy danh sách market index từ Vietcap và lưu vào Redis.
func FetchAndSaveMarketIndex(ctx context.Context, rdb *redis.Client) error {
	result, err := fetchMarketIndex(ctx, defaultMarketIndexSymbols)
	if err != nil {
		return fmt.Errorf("fetch market index: %w", err)
	}

	jsonBytes, err := json.Marshal(result)
	if err != nil {
		return fmt.Errorf("marshal market index: %w", err)
	}

	key := rediskeys.VietcapMarketIndex()
	ttl := time.Duration(rediskeys.TTLMarketIndexSec) * time.Second
	if err := rdb.Set(ctx, key, jsonBytes, ttl).Err(); err != nil {
		return fmt.Errorf("redis set %s: %w", key, err)
	}

	// Cập nhật timestamp riêng để client có thể check freshness
	updatedAt := time.Now().UTC().Format(time.RFC3339)
	_ = rdb.Set(ctx, rediskeys.VietcapMarketIndexUpdatedAt(), updatedAt, ttl)

	log.Printf("[vietcap] market_index saved: count=%d", result.Count)
	return nil
}

// fetchMarketIndex gọi Vietcap API và trả về danh sách market index.
func fetchMarketIndex(ctx context.Context, symbols []string) (*MarketIndexResult, error) {
	body, err := json.Marshal(map[string][]string{"symbols": symbols})
	if err != nil {
		return nil, err
	}

	reqCtx, cancel := context.WithTimeout(ctx, marketIndexTimeoutSec*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, vietcapMarketIndexURL, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	req.Header.Set("Referer", "https://trading.vietcap.com.vn/")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d từ Vietcap market index API", resp.StatusCode)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var rows []map[string]interface{}
	if err := json.Unmarshal(data, &rows); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}

	return &MarketIndexResult{
		FetchedAt: time.Now().UTC().Format(time.RFC3339),
		Count:     len(rows),
		Rows:      rows,
	}, nil
}
