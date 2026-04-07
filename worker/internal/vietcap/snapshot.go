// Package vietcap xử lý việc lấy dữ liệu từ Vietcap API và lưu vào Redis.
// Các API của Vietcap là public, không cần authentication.
package vietcap

import (
	"bytes"
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
	vietcapSnapshotURL = "https://trading.vietcap.com.vn/api/price/v1/w/priceboard/tickers/price/group"
	snapshotTimeoutSec = 15
)

// SnapshotRow là một dòng dữ liệu giá trong priceboard Vietcap.
// Giữ nguyên structure từ API để dễ forward cho Next.js đọc.
type SnapshotRow = map[string]interface{}

// SnapshotResult là kết quả lấy snapshot cho một group.
type SnapshotResult struct {
	Group     string        `json:"group"`
	FetchedAt string        `json:"fetchedAt"`
	Count     int           `json:"count"`
	Rows      []SnapshotRow `json:"rows"`
}

// FetchAndSaveSnapshot lấy snapshot cho một group từ Vietcap và lưu vào Redis.
// group: VN30, HOSE, HNX, UPCOM, ...
func FetchAndSaveSnapshot(ctx context.Context, rdb *redis.Client, group string) error {
	group = strings.ToUpper(strings.TrimSpace(group))
	if group == "" {
		return fmt.Errorf("group không được để trống")
	}

	result, err := fetchSnapshot(ctx, group)
	if err != nil {
		return fmt.Errorf("fetch snapshot %s: %w", group, err)
	}

	jsonBytes, err := json.Marshal(result)
	if err != nil {
		return fmt.Errorf("marshal snapshot %s: %w", group, err)
	}

	key := rediskeys.VietcapSnapshot(group)
	ttl := time.Duration(rediskeys.TTLSnapshotSec) * time.Second
	if err := rdb.Set(ctx, key, jsonBytes, ttl).Err(); err != nil {
		return fmt.Errorf("redis set %s: %w", key, err)
	}

	log.Printf("[vietcap] snapshot saved: group=%s rows=%d key=%s", group, result.Count, key)
	return nil
}

// FetchAndSaveAllGroups lấy snapshot cho nhiều group song song và lưu vào Redis.
func FetchAndSaveAllGroups(ctx context.Context, rdb *redis.Client, groups []string) {
	type result struct {
		group string
		err   error
	}

	ch := make(chan result, len(groups))

	for _, g := range groups {
		go func(group string) {
			err := FetchAndSaveSnapshot(ctx, rdb, group)
			ch <- result{group: group, err: err}
		}(g)
	}

	for range groups {
		r := <-ch
		if r.err != nil {
			log.Printf("[vietcap] snapshot error group=%s: %v", r.group, r.err)
		}
	}
}

// fetchSnapshot gọi Vietcap API và trả về SnapshotResult.
func fetchSnapshot(ctx context.Context, group string) (*SnapshotResult, error) {
	body, err := json.Marshal(map[string]string{"group": group})
	if err != nil {
		return nil, err
	}

	reqCtx, cancel := context.WithTimeout(ctx, snapshotTimeoutSec*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, vietcapSnapshotURL, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	req.Header.Set("Referer", "https://trading.vietcap.com.vn/")
	req.Header.Set("Origin", "https://trading.vietcap.com.vn")
	req.Header.Set("Sec-Fetch-Dest", "empty")
	req.Header.Set("Sec-Fetch-Mode", "cors")
	req.Header.Set("Sec-Fetch-Site", "same-site")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d từ Vietcap snapshot API", resp.StatusCode)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var rows []SnapshotRow
	if err := json.Unmarshal(data, &rows); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}

	return &SnapshotResult{
		Group:     group,
		FetchedAt: time.Now().UTC().Format(time.RFC3339),
		Count:     len(rows),
		Rows:      rows,
	}, nil
}
