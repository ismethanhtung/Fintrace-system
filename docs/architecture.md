# Kiến trúc Fintrace System

## Tổng quan luồng dữ liệu

```
┌─────────────────────────────────────────────────────────────────┐
│                        EXTERNAL APIs                            │
│                                                                 │
│  DNSE WebSocket          Vietcap REST API      KB REST API      │
│  wss://ws-openapi        trading.vietcap.com   kbbuddywts.kbsec │
│  .dnse.com.vn            .vn/api/price/...     .com.vn/...      │
└────────┬───────────────────────┬──────────────────┬────────────┘
         │ WebSocket (realtime)  │ HTTP (30s poll)  │ HTTP (60s poll)
         ▼                       ▼                  ▼
┌─────────────────────────────────────────────────────────────────┐
│                    FINTRACE WORKER (Go)                         │
│                                                                 │
│  ┌─────────────────┐   ┌──────────────────────────────────┐    │
│  │  DNSE Stream    │   │        Asynq Scheduler           │    │
│  │  (goroutine)    │   │                                  │    │
│  │                 │   │  ┌────────────────────────────┐  │    │
│  │  - auth HMAC    │   │  │ @every 30s                 │  │    │
│  │  - subscribe    │   │  │ vietcap_snapshot task      │  │    │
│  │    channels     │   │  └─────────────┬──────────────┘  │    │
│  │  - reconnect    │   │                │                  │    │
│  │    on error     │   │  ┌────────────────────────────┐  │    │
│  └────────┬────────┘   │  │ @every 30s                 │  │    │
│           │            │  │ vietcap_market_index task   │  │    │
│           │            │  └─────────────┬──────────────┘  │    │
│           │            │                │                  │    │
│           │            │  ┌────────────────────────────┐  │    │
│           │            │  │ @every 60s                 │  │    │
│           │            │  │ kb_intraday task           │  │    │
│           │            │  └─────────────┬──────────────┘  │    │
│           │            └────────────────┼─────────────────┘    │
└───────────┼─────────────────────────────┼──────────────────────┘
            │ SET (với TTL)               │ SET (với TTL)
            ▼                             ▼
┌─────────────────────────────────────────────────────────────────┐
│                        REDIS                                    │
│                                                                 │
│  board:dnse:tick:{SYMBOL}           TTL: 5 phút                │
│  board:dnse:market_index:{INDEX}    TTL: 5 phút                │
│  board:vietcap:snapshot:{GROUP}     TTL: 2 phút                │
│  board:vietcap:market_index         TTL: 2 phút                │
│  board:kb:intraday:{SYM}:latest     TTL: 24 giờ                │
│  board:kb:intraday:{SYM}:{DATE}     TTL: 24 giờ                │
└─────────────────────────────────────────────────────────────────┘
            │ GET (gần instant)
            ▼
┌─────────────────────────────────────────────────────────────────┐
│                    NEXT.JS APP (Fintrace)                       │
│                                                                 │
│  /api/board/vietcap-snapshot     ←─ board:vietcap:snapshot:*   │
│  /api/board/vietcap-market-index ←─ board:vietcap:market_index │
│  /api/board/kb-index-intraday    ←─ board:kb:intraday:*:latest │
│  /api/dnse/realtime/stream (SSE) ←─ board:dnse:tick:*          │
│                                      board:dnse:market_index:*  │
└─────────────────────────────────────────────────────────────────┘
            │ JSON/SSE
            ▼
┌────────────────┐
│  Browser       │
│  Board Page    │
└────────────────┘
```

## Chi tiết các component

### 1. DNSE Stream (goroutine)

**File:** `worker/internal/dnse/stream.go`

```
Lifecycle:
  start → dial WebSocket → send auth → wait auth_success
        → subscribe channels → process messages (loop)
        → on error/close → exponential backoff → reconnect
```

**Channels subscribe:**
- `tick.G1.json`, `tick.G2.json`, `tick.G3.json` — tick data per symbol
- `tick_extra.G{n}.json` — thêm trường high/low/totalVol
- `top_price.G{n}.json` — 3 mức giá mua/bán tốt nhất
- `expected_price.G{n}.json` — giá kỳ vọng (ATO/ATC)
- `security_definition.G{n}.json` — định nghĩa (ref, ceiling, floor)
- `ohlc.1.json` — OHLC data theo 1-minute bar
- `market_index.VNINDEX.json`, `market_index.VN30.json`, ... — index realtime

**Lưu ý quan trọng:**  
DNSE giới hạn 300 symbols mỗi connection. Nếu cần nhiều hơn, phải tạo nhiều connection song song. Hiện tại worker chỉ subscribe symbols được khai báo trong `DNSE_TICK_SYMBOLS`.

Để nhận data cho toàn bộ HOSE/HNX/UPCOM, xem [Hướng dẫn multi-connection](#multi-connection-dnse).

### 2. Vietcap Snapshot (Asynq task)

**File:** `worker/internal/vietcap/snapshot.go`

- Chạy mỗi 30 giây (cấu hình qua `VIETCAP_SNAPSHOT_INTERVAL_SEC`)
- Lấy song song tất cả groups (`VN30`, `HOSE`, `HNX`, `UPCOM`)
- Mỗi group lưu một Redis key riêng: `board:vietcap:snapshot:{GROUP}`
- TTL: 2 phút (đủ sống qua một vài lần poll miss)

**Response format từ Vietcap:**
```json
{
  "group": "VN30",
  "fetchedAt": "2026-04-06T09:00:00Z",
  "count": 30,
  "rows": [
    { "s": "VCB", "c": 88500, "o": 88000, ... }
  ]
}
```

### 3. Vietcap Market Index (Asynq task)

**File:** `worker/internal/vietcap/market_index.go`

- Chạy mỗi 30 giây
- Lấy VNINDEX, VN30, HNXIndex, HNX30, HNXUpcomIndex, VNXALL
- Lưu tất cả vào một key duy nhất: `board:vietcap:market_index`
- TTL: 2 phút

### 4. KB Intraday (Asynq task)

**File:** `worker/internal/kb/intraday.go`

- Chạy mỗi 60 giây
- Lấy theo ngày hiện tại (múi giờ Việt Nam UTC+7)
- Thử nhiều source symbol (fallback) nếu symbol chính không có data
- Lưu 2 key: theo ngày (`board:kb:intraday:{SYM}:{DATE}`) và `latest` key
- TTL: 24 giờ

## Asynq Architecture

```
                ┌──────────────────┐
                │  Asynq Scheduler │  (enqueue tasks theo lịch)
                └────────┬─────────┘
                         │ RPUSH/LPUSH vào Redis queue
                         ▼
                ┌──────────────────┐
                │    Redis         │  (queue storage)
                │  (Asynq queues)  │
                └────────┬─────────┘
                         │ BRPOP
                         ▼
                ┌──────────────────┐
                │  Asynq Server    │  (worker pool, concurrency=4)
                │  ┌────────────┐  │
                │  │ Handler 1  │  │  vietcap_snapshot
                │  │ Handler 2  │  │  vietcap_market_index
                │  │ Handler 3  │  │  kb_intraday
                │  └────────────┘  │
                └──────────────────┘
```

**Lưu ý:** Asynq và DNSE stream **cùng dùng Redis** nhưng dùng key namespace khác nhau:
- Asynq dùng prefix `asynq:*` (internal)
- Worker dùng prefix `board:*` (data)

## Multi-connection DNSE

**(Tính năng tương lai)**

Để subscribe toàn bộ HOSE (~1600 symbols), cần:
1. Chia danh sách symbol thành batches 300
2. Tạo `n` WebSocket connections song song (mỗi connection = 1 goroutine)
3. Merge kết quả vào cùng Redis namespace

Flow:
```
goroutine 1 → DNSE WebSocket → symbols[0:300]   → Redis
goroutine 2 → DNSE WebSocket → symbols[300:600]  → Redis
goroutine n → DNSE WebSocket → symbols[(n-1)*300:] → Redis
```

Để kích hoạt, cần:
- Có danh sách đầy đủ symbols (lấy từ Vietcap snapshot hoặc DNSE catalog)
- Cấu hình `DNSE_TICK_SYMBOLS` hoặc thêm auto-discovery từ Vietcap data

## Startup Sequence

```
1. Load .env + config
2. Kết nối Redis (retry 10 lần)
3. Warm-up goroutine:
   - Fetch Vietcap snapshot (tất cả groups)
   - Fetch Vietcap market index
   - Fetch KB intraday (tất cả symbols)
4. Khởi động DNSE WebSocket goroutine
5. Khởi động Asynq scheduler (đăng ký cron jobs)
6. Khởi động Asynq server (lắng nghe tasks)
7. Chờ SIGINT/SIGTERM
8. Graceful shutdown (đóng scheduler → server → Redis)
```

## Reliability

| Tình huống | Xử lý |
|---|---|
| DNSE WebSocket drop | Tự reconnect với exponential backoff (2s → 4s → ... → 60s) |
| Vietcap API lỗi | Asynq retry tự động; data cũ trong Redis vẫn được phục vụ |
| Redis không khả dụng | Worker crash; data cũ không có; cần alert |
| Worker restart | Warm-up fetch ngay lập tức; không đợi chu kỳ scheduler |
