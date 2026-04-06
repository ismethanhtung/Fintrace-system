// Package tasks định nghĩa các Asynq task type và handler xử lý chúng.
// Hiện tại có 3 task được scheduler chạy định kỳ:
//   - TypeVietcapSnapshot: lấy priceboard snapshot từ Vietcap
//   - TypeVietcapMarketIndex: lấy danh sách market index từ Vietcap
//   - TypeKbIntraday: lấy intraday chart từ KB Securities
package tasks

import (
	"context"
	"encoding/json"
	"log"

	"github.com/hibiken/asynq"
	"github.com/redis/go-redis/v9"

	"fintrace-worker/internal/config"
	"fintrace-worker/internal/kb"
	"fintrace-worker/internal/vietcap"
)

// Task type constants — dùng làm task name trong Asynq.
const (
	TypeVietcapSnapshot    = "board:vietcap:snapshot"
	TypeVietcapMarketIndex = "board:vietcap:market_index"
	TypeKbIntraday         = "board:kb:intraday"
)

// VietcapSnapshotPayload là payload cho task VietcapSnapshot.
// Nếu Groups rỗng, lấy từ config.
type VietcapSnapshotPayload struct {
	Groups []string `json:"groups,omitempty"`
}

// Handler giữ dependency cần thiết để xử lý các task.
type Handler struct {
	cfg *config.Config
	rdb *redis.Client
}

// NewHandler tạo Handler mới.
func NewHandler(cfg *config.Config, rdb *redis.Client) *Handler {
	return &Handler{cfg: cfg, rdb: rdb}
}

// HandleVietcapSnapshot xử lý task lấy priceboard snapshot từ Vietcap.
func (h *Handler) HandleVietcapSnapshot(_ context.Context, t *asynq.Task) error {
	// Parse payload để lấy groups, fallback về config
	var payload VietcapSnapshotPayload
	if err := json.Unmarshal(t.Payload(), &payload); err != nil || len(payload.Groups) == 0 {
		payload.Groups = h.cfg.VietcapGroups
	}

	log.Printf("[task] vietcap_snapshot: fetching %d groups: %v", len(payload.Groups), payload.Groups)

	// Dùng background context để task không bị cancel bởi asynq context ngắn
	ctx := context.Background()
	vietcap.FetchAndSaveAllGroups(ctx, h.rdb, payload.Groups)

	log.Printf("[task] vietcap_snapshot: done")
	return nil
}

// HandleVietcapMarketIndex xử lý task lấy market index từ Vietcap.
func (h *Handler) HandleVietcapMarketIndex(_ context.Context, _ *asynq.Task) error {
	log.Printf("[task] vietcap_market_index: fetching...")
	ctx := context.Background()
	if err := vietcap.FetchAndSaveMarketIndex(ctx, h.rdb); err != nil {
		log.Printf("[task] vietcap_market_index error: %v", err)
		return err
	}
	log.Printf("[task] vietcap_market_index: done")
	return nil
}

// HandleKbIntraday xử lý task lấy intraday chart từ KB Securities.
func (h *Handler) HandleKbIntraday(_ context.Context, _ *asynq.Task) error {
	log.Printf("[task] kb_intraday: fetching symbols=%v", h.cfg.KbSymbols)
	ctx := context.Background()
	kb.FetchAndSaveAllSymbols(ctx, h.rdb, h.cfg.KbSymbols)
	log.Printf("[task] kb_intraday: done")
	return nil
}
