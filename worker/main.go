// Fintrace Worker — Go service thu thập dữ liệu thị trường và lưu vào Redis.
//
// Worker làm hai việc song song:
//  1. Kết nối WebSocket DNSE để nhận realtime tick + market index → ghi vào Redis ngay lập tức.
//  2. Chạy Asynq scheduler để định kỳ:
//     - Lấy Vietcap priceboard snapshot mỗi 30 giây
//     - Lấy Vietcap market index mỗi 30 giây
//     - Lấy KB intraday chart mỗi 60 giây
//
// Next.js app sau đó đọc từ Redis thay vì gọi thẳng upstream API.
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/hibiken/asynq"
	"github.com/joho/godotenv"
	"github.com/redis/go-redis/v9"

	"fintrace-worker/internal/config"
	"fintrace-worker/internal/dnse"
	"fintrace-worker/internal/tasks"
	"fintrace-worker/internal/vietcap"
	"fintrace-worker/internal/kb"
)

func main() {
	// Load .env nếu có (dev mode)
	if err := godotenv.Load(); err != nil {
		log.Println("[main] .env không tìm thấy, dùng environment variables hiện tại")
	}

	cfg := config.Load()

	log.Printf("[main] redis addr=%s db=%d", cfg.RedisAddr, cfg.RedisDB)
	log.Printf("[main] dnse boards=%v market_indexes=%v tick_symbols=%d debug=%v",
		cfg.DnseBoards, cfg.DnseMarketIndexes, len(cfg.DnseTickSymbols), cfg.DnseDebug)
	log.Printf("[main] vietcap groups=%v", cfg.VietcapGroups)
	log.Printf("[main] kb symbols=%v", cfg.KbSymbols)

	// Khởi tạo Redis client
	rdb := redis.NewClient(&redis.Options{
		Addr:     cfg.RedisAddr,
		Password: cfg.RedisPassword,
		DB:       cfg.RedisDB,
	})

	// Kiểm tra kết nối Redis
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	if err := pingRedis(ctx, rdb); err != nil {
		log.Fatalf("[main] Không thể kết nối Redis: %v", err)
	}
	log.Println("[main] Redis connected OK")

	// ─── Asynq setup ────────────────────────────────────────────────────────────

	redisOpt := asynq.RedisClientOpt{
		Addr:     cfg.RedisAddr,
		Password: cfg.RedisPassword,
		DB:       cfg.RedisDB,
	}

	// Server xử lý tasks
	asynqServer := asynq.NewServer(
		redisOpt,
		asynq.Config{
			Concurrency: 4,
			Queues: map[string]int{
				"critical": 6,
				"default":  3,
				"low":      1,
			},
			ErrorHandler: asynq.ErrorHandlerFunc(func(ctx context.Context, task *asynq.Task, err error) {
				log.Printf("[asynq] task error type=%s err=%v", task.Type(), err)
			}),
		},
	)

	// Đăng ký handlers
	handler := tasks.NewHandler(cfg, rdb)
	mux := asynq.NewServeMux()
	mux.HandleFunc(tasks.TypeVietcapSnapshot, handler.HandleVietcapSnapshot)
	mux.HandleFunc(tasks.TypeVietcapMarketIndex, handler.HandleVietcapMarketIndex)
	mux.HandleFunc(tasks.TypeKbIntraday, handler.HandleKbIntraday)

	// Scheduler chạy tasks định kỳ
	scheduler := asynq.NewScheduler(redisOpt, &asynq.SchedulerOpts{
		LogLevel: asynq.WarnLevel,
	})

	snapshotCron := fmt.Sprintf("@every %ds", cfg.VietcapSnapshotIntervalSec)
	marketIndexCron := fmt.Sprintf("@every %ds", cfg.VietcapMarketIndexIntervalSec)
	kbIntradayCron := fmt.Sprintf("@every %ds", cfg.KbIntradayIntervalSec)

	if _, err := scheduler.Register(snapshotCron,
		asynq.NewTask(tasks.TypeVietcapSnapshot, nil),
		asynq.Queue("default"),
	); err != nil {
		log.Fatalf("[main] register vietcap_snapshot: %v", err)
	}

	if _, err := scheduler.Register(marketIndexCron,
		asynq.NewTask(tasks.TypeVietcapMarketIndex, nil),
		asynq.Queue("default"),
	); err != nil {
		log.Fatalf("[main] register vietcap_market_index: %v", err)
	}

	if _, err := scheduler.Register(kbIntradayCron,
		asynq.NewTask(tasks.TypeKbIntraday, nil),
		asynq.Queue("low"),
	); err != nil {
		log.Fatalf("[main] register kb_intraday: %v", err)
	}

	// ─── Chạy warm-up ngay khi khởi động ────────────────────────────────────────
	// Không đợi scheduler mới chạy lần đầu; lấy data ngay khi start.

	go func() {
		log.Println("[main] warm-up: fetching initial data...")
		warmCtx, warmCancel := context.WithTimeout(ctx, 30*time.Second)
		defer warmCancel()

		vietcap.FetchAndSaveAllGroups(warmCtx, rdb, cfg.VietcapGroups)
		if err := vietcap.FetchAndSaveMarketIndex(warmCtx, rdb); err != nil {
			log.Printf("[main] warm-up market_index error: %v", err)
		}
		kb.FetchAndSaveAllSymbols(warmCtx, rdb, cfg.KbSymbols)

		log.Println("[main] warm-up: done")
	}()

	// ─── Start goroutines ────────────────────────────────────────────────────────

	// DNSE WebSocket goroutine (chạy vô hạn, tự reconnect)
	go func() {
		if cfg.DnseAPIKey == "" || cfg.DnseAPISecret == "" {
			log.Println("[main] DNSE_API_KEY/SECRET chưa cấu hình, bỏ qua DNSE stream")
			return
		}
		dnse.RunStream(ctx, cfg, rdb)
	}()

	// Asynq scheduler
	if err := scheduler.Start(); err != nil {
		log.Fatalf("[main] scheduler start: %v", err)
	}

	// Asynq server (blocking trong goroutine)
	serverErrCh := make(chan error, 1)
	go func() {
		if err := asynqServer.Start(mux); err != nil {
			serverErrCh <- err
		}
	}()

	log.Println("[main] worker is running. Ctrl+C để dừng.")

	// ─── Graceful shutdown ───────────────────────────────────────────────────────

	select {
	case <-ctx.Done():
		log.Println("[main] shutdown signal received")
	case err := <-serverErrCh:
		log.Printf("[main] asynq server error: %v", err)
	}

	log.Println("[main] shutting down scheduler...")
	scheduler.Shutdown()

	log.Println("[main] shutting down asynq server...")
	asynqServer.Shutdown()

	log.Println("[main] closing redis...")
	_ = rdb.Close()

	log.Println("[main] worker stopped cleanly")
}

// pingRedis thử kết nối Redis với retry.
func pingRedis(ctx context.Context, rdb *redis.Client) error {
	const maxRetry = 10
	for i := 0; i < maxRetry; i++ {
		if err := rdb.Ping(ctx).Err(); err == nil {
			return nil
		}
		log.Printf("[main] redis not ready, retry %d/%d...", i+1, maxRetry)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(2 * time.Second):
		}
	}
	return fmt.Errorf("redis không phản hồi sau %d lần thử", maxRetry)
}
