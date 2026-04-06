# Fintrace System

Hệ thống hỗ trợ **tự chủ dữ liệu** cho Fintrace — thay vì Next.js app gọi trực tiếp các API bên ngoài, một **Go Worker** sẽ thu thập dữ liệu liên tục và lưu vào **Redis**. Next.js chỉ cần đọc từ Redis.

## Mục tiêu

| Trước | Sau |
|---|---|
| Next.js → DNSE WebSocket trực tiếp | Worker → DNSE WebSocket → Redis ← Next.js |
| Next.js → Vietcap API mỗi 45s/request | Worker → Vietcap API mỗi 30s → Redis ← Next.js |
| Next.js → KB API mỗi request | Worker → KB API mỗi 60s → Redis ← Next.js |

**Lợi ích:**
- Giảm tải cho external API (chỉ 1 connection duy nhất từ worker)
- Data luôn fresh trong Redis, Next.js đọc gần như instant
- Dễ scale: nhiều instance Next.js cùng đọc Redis
- Có thể lưu lịch sử snapshot để phân tích sau

## Cấu trúc

```
Fintrace-system/
├── worker/                    # Go service
│   ├── main.go                # Entry point, setup Asynq + DNSE stream
│   ├── go.mod
│   ├── Dockerfile
│   └── internal/
│       ├── config/            # Load cấu hình từ env vars
│       ├── rediskeys/         # Tập trung tên Redis keys
│       ├── dnse/
│       │   ├── auth.go        # HMAC-SHA256 authentication với DNSE
│       │   └── stream.go      # WebSocket stream, auto-reconnect
│       ├── vietcap/
│       │   ├── snapshot.go    # Priceboard snapshot (polling)
│       │   └── market_index.go # Market index (polling)
│       ├── kb/
│       │   └── intraday.go    # Intraday chart (polling)
│       └── tasks/
│           └── handler.go     # Asynq task handlers
├── docker-compose.yml         # Redis + Worker
├── .env.example               # Mẫu cấu hình
└── docs/
    ├── README.md              # File này
    ├── architecture.md        # Kiến trúc chi tiết
    └── redis-schema.md        # Schema Redis keys
```

## Khởi động nhanh

### 1. Tạo file .env

```bash
cd Fintrace-system
cp .env.example .env
```

Chỉnh sửa `.env`, ít nhất cần điền:
```env
DNSE_API_KEY=your_key
DNSE_API_SECRET=your_secret
REDIS_PASSWORD=your_redis_password  # tùy chọn
```

### 2. Build và chạy với Docker Compose

```bash
docker compose up -d --build
```

### 3. Kiểm tra logs

```bash
# Xem log worker
docker compose logs -f worker

# Kiểm tra data trong Redis
docker compose exec redis redis-cli keys "board:*"
```

### 4. Chạy local (không Docker)

Cần cài [Go 1.22+](https://go.dev/dl/) và Redis đang chạy.

```bash
cd worker

# Tải dependencies
go mod tidy

# Chạy
go run main.go
```

## Cấu hình

Xem [.env.example](../.env.example) để biết tất cả biến môi trường.

| Biến | Mặc định | Mô tả |
|---|---|---|
| `REDIS_ADDR` | `localhost:6379` | Địa chỉ Redis |
| `REDIS_PASSWORD` | _(trống)_ | Mật khẩu Redis |
| `DNSE_API_KEY` | _(bắt buộc)_ | API key DNSE |
| `DNSE_API_SECRET` | _(bắt buộc)_ | API secret DNSE |
| `DNSE_BOARDS` | `G1,G2,G3` | Board DNSE cần subscribe |
| `DNSE_TICK_SYMBOLS` | _(trống)_ | Symbols cụ thể nhận tick realtime |
| `VIETCAP_GROUPS` | `VN30,HOSE,HNX,UPCOM` | Nhóm cổ phiếu Vietcap |
| `VIETCAP_SNAPSHOT_INTERVAL_SEC` | `30` | Chu kỳ lấy snapshot (giây) |
| `KB_SYMBOLS` | `VNINDEX,VN30,...` | Symbols KB intraday |
| `KB_INTRADAY_INTERVAL_SEC` | `60` | Chu kỳ lấy KB intraday (giây) |

## Tích hợp với Next.js (bước tiếp theo)

Sau khi worker chạy và Redis có data, cần cập nhật các API routes trong Next.js để đọc từ Redis:

1. **`/api/board/vietcap-snapshot`** → đọc `board:vietcap:snapshot:{GROUP}` từ Redis
2. **`/api/board/vietcap-market-index`** → đọc `board:vietcap:market_index` từ Redis
3. **`/api/board/kb-index-intraday`** → đọc `board:kb:intraday:{SYMBOL}:latest` từ Redis
4. **`/api/dnse/realtime/stream`** → stream từ `board:dnse:tick:{SYMBOL}` + `board:dnse:market_index:{INDEX}`

Chi tiết xem [redis-schema.md](./redis-schema.md).
