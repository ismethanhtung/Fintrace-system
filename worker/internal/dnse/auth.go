// Package dnse xử lý kết nối WebSocket với DNSE OpenAPI.
// Tài liệu: https://openapi.dnse.com.vn
package dnse

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"
)

// AuthMessage là message xác thực gửi lên DNSE sau khi WebSocket kết nối.
type AuthMessage struct {
	Action    string `json:"action"`
	APIKey    string `json:"api_key"`
	Signature string `json:"signature"`
	Timestamp int64  `json:"timestamp"`
	Nonce     string `json:"nonce"`
}

// SubscribeMessage là message subscribe một channel trên DNSE.
type SubscribeMessage struct {
	Action   string             `json:"action"`
	Channels []SubscribeChannel `json:"channels"`
}

// SubscribeChannel mô tả một channel cần subscribe.
// Nếu channel không cần symbols (vd: market_index), để Symbols = [].
type SubscribeChannel struct {
	Name    string   `json:"name"`
	Symbols []string `json:"symbols"`
}

// PongMessage dùng để trả lời PING từ server.
type PongMessage struct {
	Action string `json:"action"`
	TS     int64  `json:"ts"`
}

// BuildAuthMessage tạo message xác thực với HMAC-SHA256 signature.
// Thuật toán: HMAC-SHA256(apiSecret, "{apiKey}:{timestamp}:{nonce}")
func BuildAuthMessage(apiKey, apiSecret string) ([]byte, error) {
	ts := time.Now().Unix()
	nonce := fmt.Sprintf("%d", time.Now().UnixNano()/1000)
	message := fmt.Sprintf("%s:%d:%s", apiKey, ts, nonce)

	mac := hmac.New(sha256.New, []byte(apiSecret))
	if _, err := mac.Write([]byte(message)); err != nil {
		return nil, fmt.Errorf("hmac write: %w", err)
	}
	sig := hex.EncodeToString(mac.Sum(nil))

	msg := AuthMessage{
		Action:    "auth",
		APIKey:    apiKey,
		Signature: sig,
		Timestamp: ts,
		Nonce:     nonce,
	}
	return json.Marshal(msg)
}

// BuildSubscribeMessage tạo message subscribe cho một channel + danh sách symbols.
func BuildSubscribeMessage(channelName string, symbols []string) ([]byte, error) {
	msg := SubscribeMessage{
		Action: "subscribe",
		Channels: []SubscribeChannel{
			{
				Name:    channelName,
				Symbols: symbols,
			},
		},
	}
	return json.Marshal(msg)
}

// BuildPongMessage tạo message pong phản hồi PING.
func BuildPongMessage() ([]byte, error) {
	msg := PongMessage{
		Action: "pong",
		TS:     time.Now().Unix(),
	}
	return json.Marshal(msg)
}
