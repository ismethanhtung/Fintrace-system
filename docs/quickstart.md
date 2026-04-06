# Quickstart Guide

## Bước 1 — Cài đặt Go (nếu chưa có)

```bash
# macOS (dùng Homebrew)
brew install go

# Hoặc tải từ https://go.dev/dl/
```

Yêu cầu: **Go 1.22+**

## Bước 2 — Tạo .env

```bash
cd Fintrace-system
cp .env.example .env
```

Mở `.env` và điền:
- `DNSE_API_KEY` và `DNSE_API_SECRET` — lấy từ [DNSE OpenAPI Portal](https://openapi.dnse.com.vn)
- `REDIS_PASSWORD` — mật khẩu Redis (khuyến nghị đặt cho production)

## Bước 3 — Resolve Go dependencies

```bash
cd Fintrace-system/worker
go mod tidy
```

Lệnh này tự tải dependencies và tạo file `go.sum`.

## Bước 4a — Chạy với Docker Compose (khuyến nghị)

```bash
cd Fintrace-system
docker compose up -d --build
```

Kiểm tra logs:
```bash
docker compose logs -f worker
```

Kết quả mong đợi:
```
[main] Redis connected OK
[main] warm-up: fetching initial data...
[vietcap] snapshot saved: group=VN30 rows=30 key=board:vietcap:snapshot:VN30
[vietcap] market_index saved: count=6
[kb] intraday saved: symbol=VNINDEX source=VNINDEX date=06-04-2026
[main] warm-up: done
[dnse] connected to wss://ws-openapi.dnse.com.vn/v1/stream?encoding=json
[dnse] auth success, subscribing to channels...
```

## Bước 4b — Chạy local (không Docker)

Cần Redis đang chạy trên `localhost:6379`.

```bash
# Terminal 1: Redis
redis-server

# Terminal 2: Worker
cd Fintrace-system/worker
go run main.go
```

## Bước 5 — Kiểm tra data trong Redis

```bash
# Nếu dùng Docker
docker compose exec redis redis-cli keys "board:*"

# Nếu local
redis-cli keys "board:*"
```

Output mong đợi:
```
board:vietcap:snapshot:VN30
board:vietcap:snapshot:HOSE
board:vietcap:snapshot:HNX
board:vietcap:snapshot:UPCOM
board:vietcap:market_index
board:vietcap:market_index:updated_at
board:kb:intraday:VNINDEX:latest
board:kb:intraday:VNINDEX:06-04-2026
... (các symbols KB khác)
```

Xem nội dung:
```bash
redis-cli get "board:vietcap:snapshot:VN30" | jq '.count'
# → 30

redis-cli get "board:vietcap:market_index" | jq '.rows[0].indexId'
# → "VNINDEX"
```

## Dừng hệ thống

```bash
# Docker Compose
docker compose down

# Local: Ctrl+C trong terminal chạy worker
```

## Troubleshooting

| Lỗi | Nguyên nhân | Giải pháp |
|---|---|---|
| `Redis không phản hồi` | Redis chưa chạy | `docker compose up redis -d` hoặc `redis-server` |
| `DNSE_API_KEY chưa cấu hình` | Thiếu env | Điền vào `.env` |
| `HTTP 403 từ Vietcap` | Rate limit | Tăng interval trong `.env` |
| `data_1P rỗng` | Ngoài giờ giao dịch | KB intraday chỉ có data trong giờ trading |
