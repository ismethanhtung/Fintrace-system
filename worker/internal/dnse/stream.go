package dnse

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"sync/atomic"
	"time"
	"unicode/utf8"

	"github.com/gorilla/websocket"
	"github.com/redis/go-redis/v9"

	"fintrace-worker/internal/config"
	"fintrace-worker/internal/rediskeys"
)

const (
	// Thời gian chờ tối đa giữa các lần reconnect (exponential backoff)
	maxReconnectDelaySec = 60
	// Interval gửi proactive pong để giữ kết nối
	proactivePongIntervalSec = 90
	// Timeout đọc message từ WebSocket
	readTimeoutSec = 300
)

// Số log mẫu tối đa khi DNSE_DEBUG=1 và payload không map được vào tick/index.
var dnseUnhandledLogged atomic.Uint32

// channelNeedsSymbols trả về true nếu channel cần danh sách symbols khi subscribe.
// market_index channels không cần symbols.
func channelNeedsSymbols(channel string) bool {
	lower := strings.ToLower(channel)
	return !strings.HasPrefix(lower, "market_index.")
}

// buildChannels tạo danh sách tên channel đầy đủ từ config.
// Ví dụ: tick.G1.json, market_index.VNINDEX.json, ...
func buildChannels(cfg *config.Config) []string {
	seen := make(map[string]struct{})
	var channels []string

	add := func(ch string) {
		if _, ok := seen[ch]; !ok {
			seen[ch] = struct{}{}
			channels = append(channels, ch)
		}
	}

	// Channels per board (tick, top_price, ...)
	boardChannelPrefixes := []string{
		"security_definition",
		"tick",
		"tick_extra",
		"top_price",
		"expected_price",
	}
	for _, board := range cfg.DnseBoards {
		for _, prefix := range boardChannelPrefixes {
			add(fmt.Sprintf("%s.%s.json", prefix, board))
		}
	}

	// OHLC channel (không cần board code)
	add("ohlc.1.json")

	// Market index channels (không cần symbols)
	for _, idx := range cfg.DnseMarketIndexes {
		add(fmt.Sprintf("market_index.%s.json", idx))
	}

	return channels
}

// RunStream kết nối WebSocket DNSE và liên tục nhận data, ghi vào Redis.
// Hàm này chạy trong một goroutine riêng và tự động reconnect khi mất kết nối.
// Dừng khi ctx bị cancel.
func RunStream(ctx context.Context, cfg *config.Config, rdb *redis.Client) {
	log.Println("[dnse] stream goroutine started")
	backoffSec := 2

	for {
		select {
		case <-ctx.Done():
			log.Println("[dnse] stream goroutine stopped")
			return
		default:
		}

		err := runOnce(ctx, cfg, rdb)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			log.Printf("[dnse] stream error: %v — reconnecting in %ds", err, backoffSec)
			select {
			case <-ctx.Done():
				return
			case <-time.After(time.Duration(backoffSec) * time.Second):
			}
			// Exponential backoff, tối đa maxReconnectDelaySec
			backoffSec = min(backoffSec*2, maxReconnectDelaySec)
		} else {
			backoffSec = 2
		}
	}
}

// runOnce thực hiện một vòng kết nối + xử lý message DNSE.
// Trả về error nếu kết nối thất bại hoặc bị ngắt bất ngờ.
func runOnce(ctx context.Context, cfg *config.Config, rdb *redis.Client) error {
	if cfg.DnseAPIKey == "" || cfg.DnseAPISecret == "" {
		return fmt.Errorf("DNSE_API_KEY hoặc DNSE_API_SECRET chưa được cấu hình")
	}

	dialer := websocket.DefaultDialer
	conn, _, err := dialer.DialContext(ctx, cfg.DnseWsURL, nil)
	if err != nil {
		return fmt.Errorf("dial %s: %w", cfg.DnseWsURL, err)
	}
	defer conn.Close()

	log.Printf("[dnse] connected to %s", cfg.DnseWsURL)

	// Gửi auth message ngay sau khi kết nối
	authBytes, err := BuildAuthMessage(cfg.DnseAPIKey, cfg.DnseAPISecret)
	if err != nil {
		return fmt.Errorf("build auth message: %w", err)
	}
	if err := conn.WriteMessage(websocket.TextMessage, authBytes); err != nil {
		return fmt.Errorf("send auth: %w", err)
	}

	channels := buildChannels(cfg)
	tickSymbols := cfg.DnseTickSymbols

	// Proactive pong ticker để giữ kết nối sống
	pongTicker := time.NewTicker(proactivePongIntervalSec * time.Second)
	defer pongTicker.Stop()

	// Channel để signal kết thúc loop đọc
	done := make(chan error, 1)

	go func() {
		done <- readLoop(ctx, conn, cfg, rdb, channels, tickSymbols)
	}()

	for {
		select {
		case <-ctx.Done():
			_ = conn.WriteMessage(
				websocket.CloseMessage,
				websocket.FormatCloseMessage(websocket.CloseNormalClosure, "shutdown"),
			)
			return nil

		case err := <-done:
			return err

		case <-pongTicker.C:
			pongBytes, _ := BuildPongMessage()
			if err := conn.WriteMessage(websocket.TextMessage, pongBytes); err != nil {
				log.Printf("[dnse] proactive pong failed: %v", err)
			}
		}
	}
}

// readLoop đọc liên tục message từ WebSocket và xử lý.
func readLoop(
	ctx context.Context,
	conn *websocket.Conn,
	cfg *config.Config,
	rdb *redis.Client,
	channels []string,
	tickSymbols []string,
) error {
	isAuthed := false

	_ = conn.SetReadDeadline(time.Now().Add(readTimeoutSec * time.Second))
	conn.SetPongHandler(func(string) error {
		_ = conn.SetReadDeadline(time.Now().Add(readTimeoutSec * time.Second))
		return nil
	})

	for {
		if ctx.Err() != nil {
			return nil
		}

		_, rawMsg, err := conn.ReadMessage()
		if err != nil {
			return fmt.Errorf("read: %w", err)
		}

		// Reset deadline sau mỗi message
		_ = conn.SetReadDeadline(time.Now().Add(readTimeoutSec * time.Second))

		// Xử lý PING dạng text
		if strings.TrimSpace(strings.ToUpper(string(rawMsg))) == "PING" {
			pongBytes, _ := BuildPongMessage()
			_ = conn.WriteMessage(websocket.TextMessage, pongBytes)
			continue
		}

		var msg map[string]interface{}
		if err := json.Unmarshal(rawMsg, &msg); err != nil {
			continue
		}

		action := strings.ToLower(fmt.Sprintf("%v", firstOf(msg["action"], msg["a"], msg["type"], "")))

		switch action {
		case "ping":
			pongBytes, _ := BuildPongMessage()
			_ = conn.WriteMessage(websocket.TextMessage, pongBytes)

		case "auth_success":
			if !isAuthed {
				isAuthed = true
				log.Println("[dnse] auth success, subscribing to channels...")
				if err := subscribeAll(conn, channels, tickSymbols); err != nil {
					return fmt.Errorf("subscribe: %w", err)
				}
			}

		case "auth_error", "error":
			return fmt.Errorf("DNSE auth/server error: %v", msg)

		default:
			// Data message — lưu vào Redis
			if isAuthed {
				handled := handleDataMessage(ctx, rdb, msg)
				if !handled && cfg.DnseDebug {
					if n := dnseUnhandledLogged.Add(1); n <= 5 {
						log.Printf("[dnse] debug: JSON sau auth chưa map tick/index (mẫu %d/5): %s",
							n, truncateString(string(rawMsg), 480))
					}
				}
			}
		}
	}
}

// subscribeAll gửi subscribe message cho tất cả channels.
func subscribeAll(conn *websocket.Conn, channels []string, tickSymbols []string) error {
	for _, ch := range channels {
		var symbols []string
		if channelNeedsSymbols(ch) {
			symbols = tickSymbols
			if len(symbols) == 0 {
				// Không có symbols cụ thể → bỏ qua channel này
				log.Printf("[dnse] skip channel %s (no symbols configured)", ch)
				continue
			}
		}

		subBytes, err := BuildSubscribeMessage(ch, symbols)
		if err != nil {
			return fmt.Errorf("build subscribe for %s: %w", ch, err)
		}
		if err := conn.WriteMessage(websocket.TextMessage, subBytes); err != nil {
			return fmt.Errorf("send subscribe for %s: %w", ch, err)
		}
		log.Printf("[dnse] subscribed channel=%s symbols=%d", ch, len(symbols))
	}
	return nil
}

// handleDataMessage xử lý message data từ DNSE và lưu vào Redis.
// Trả về true nếu đã nhận dạng được tick hoặc market index (có ghi Redis hoặc đã parse đúng loại).
func handleDataMessage(ctx context.Context, rdb *redis.Client, msg map[string]interface{}) bool {
	// Thử parse market index (DNSE có thể gửi indexName / IndexName / ...)
	if indexName, ok := extractString(msg,
		"indexName", "index_name", "IndexName", "INDEX_NAME", "index", "Index",
	); ok && indexName != "" {
		saveMarketIndex(ctx, rdb, indexName, msg)
		return true
	}

	// Tick: symbol / Symbol / ticker ...
	if symbol, ok := extractString(msg,
		"symbol", "Symbol", "s", "ticker", "Ticker", "SYM",
	); ok && symbol != "" {
		saveTick(ctx, rdb, strings.ToUpper(symbol), msg)
		return true
	}

	// Nested data: thử unwrap field "data" hoặc "d"
	for _, key := range []string{"data", "d", "payload"} {
		if nested, ok := msg[key]; ok {
			if nestedMap, ok := nested.(map[string]interface{}); ok {
				return handleDataMessage(ctx, rdb, nestedMap)
			}
			if nestedArr, ok := nested.([]interface{}); ok {
				any := false
				for _, item := range nestedArr {
					if itemMap, ok := item.(map[string]interface{}); ok {
						if handleDataMessage(ctx, rdb, itemMap) {
							any = true
						}
					}
				}
				if any {
					return true
				}
			}
		}
	}
	return false
}

// truncateString cắt chuỗi theo số rune (an toàn với UTF-8).
func truncateString(s string, maxRunes int) string {
	if maxRunes <= 0 || utf8.RuneCountInString(s) <= maxRunes {
		return s
	}
	r := []rune(s)
	return string(r[:maxRunes]) + "…"
}

// saveTick lưu dữ liệu tick của một symbol vào Redis.
func saveTick(ctx context.Context, rdb *redis.Client, symbol string, data map[string]interface{}) {
	data["_savedAt"] = time.Now().UTC().Format(time.RFC3339)
	jsonBytes, err := json.Marshal(data)
	if err != nil {
		return
	}
	key := rediskeys.DnseTick(symbol)
	if err := rdb.Set(ctx, key, jsonBytes, rediskeys.TTLTickSec*time.Second).Err(); err != nil {
		log.Printf("[dnse] redis set tick %s: %v", symbol, err)
	}
}

// saveMarketIndex lưu dữ liệu market index vào Redis.
func saveMarketIndex(ctx context.Context, rdb *redis.Client, indexName string, data map[string]interface{}) {
	data["_savedAt"] = time.Now().UTC().Format(time.RFC3339)
	jsonBytes, err := json.Marshal(data)
	if err != nil {
		return
	}
	key := rediskeys.DnseMarketIndex(strings.ToUpper(indexName))
	if err := rdb.Set(ctx, key, jsonBytes, rediskeys.TTLMarketIndexSec*time.Second).Err(); err != nil {
		log.Printf("[dnse] redis set market_index %s: %v", indexName, err)
	}
}

// extractString thử lấy giá trị string từ map với nhiều field name khác nhau.
func extractString(m map[string]interface{}, keys ...string) (string, bool) {
	for _, k := range keys {
		if v, ok := m[k]; ok {
			if s, ok := v.(string); ok && s != "" {
				return s, true
			}
		}
	}
	return "", false
}

// firstOf trả về giá trị đầu tiên không nil/empty trong danh sách.
func firstOf(vals ...interface{}) interface{} {
	for _, v := range vals {
		if v != nil {
			if s, ok := v.(string); ok && s != "" {
				return s
			} else if !ok {
				return v
			}
		}
	}
	return ""
}

