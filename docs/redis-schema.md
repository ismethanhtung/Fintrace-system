# Redis Schema — Fintrace System

Tài liệu này mô tả tất cả Redis keys được dùng trong Fintrace System, bao gồm format, TTL, và cách Next.js API route đọc data.

## Quy ước đặt tên

```
board:<nguồn>:<loại_data>:<định_danh>
```

| Segment | Ý nghĩa |
|---|---|
| `board` | Namespace cho trang board |
| `nguồn` | `dnse`, `vietcap`, `kb` |
| `loại_data` | `tick`, `market_index`, `snapshot`, `intraday` |
| `định_danh` | Symbol, group, index name, date |

---

## 1. DNSE Tick Data

**Key:** `board:dnse:tick:{SYMBOL}`

**Ví dụ:** `board:dnse:tick:HPG`

**TTL:** 300 giây (5 phút)

**Ghi bởi:** DNSE WebSocket goroutine

**Đọc bởi:** `/api/dnse/realtime/stream` (thay thế SSE stream)

**Format (JSON string):**
```json
{
  "symbol": "HPG",
  "basicPrice": 27500,
  "ceilingPrice": 29450,
  "floorPrice": 25550,
  "matchingPrice": 27600,
  "matchingVolume": 1000,
  "totalMatchingVolume": 5430200,
  "highestPrice": 27800,
  "lowestPrice": 27300,
  "bid": [
    { "price": 27550, "quantity": 500 },
    { "price": 27500, "quantity": 1200 }
  ],
  "offer": [
    { "price": 27650, "quantity": 800 }
  ],
  "_savedAt": "2026-04-06T09:15:00Z"
}
```

> **Lưu ý:** Field names giữ nguyên như DNSE gửi về (snake_case hoặc camelCase tùy channel). `_savedAt` được thêm bởi worker.

---

## 2. DNSE Market Index

**Key:** `board:dnse:market_index:{INDEX_NAME}`

**Ví dụ:** `board:dnse:market_index:VNINDEX`

**TTL:** 120 giây (2 phút)

**Ghi bởi:** DNSE WebSocket goroutine (channel `market_index.{INDEX}.json`)

**Đọc bởi:** `/api/board/vietcap-market-index` (fallback khi Vietcap không có)

**Format (JSON string):**
```json
{
  "indexName": "VNINDEX",
  "value": 1285.34,
  "changedValue": 12.45,
  "changedRatio": 0.9788,
  "totalVolumeTraded": 892340000,
  "grossTradeAmount": 24567890000000,
  "upCount": 187,
  "refCount": 45,
  "downCount": 98,
  "highestValue": 1288.00,
  "lowestValue": 1278.50,
  "priorValue": 1272.89,
  "_savedAt": "2026-04-06T09:15:30Z"
}
```

---

## 3. Vietcap Priceboard Snapshot

**Key:** `board:vietcap:snapshot:{GROUP}`

**Ví dụ:** `board:vietcap:snapshot:VN30`, `board:vietcap:snapshot:HOSE`

**TTL:** 120 giây (2 phút)

**Ghi bởi:** Asynq task `board:vietcap:snapshot` (mỗi 30 giây)

**Đọc bởi:** `/api/board/vietcap-snapshot?group={GROUP}`

**Format (JSON string):**
```json
{
  "group": "VN30",
  "fetchedAt": "2026-04-06T09:15:00Z",
  "count": 30,
  "rows": [
    {
      "s":  "VCB",
      "c":  88500,
      "o":  88000,
      "h":  89000,
      "l":  87500,
      "v":  3240000,
      "f":  19234560,
      "tc": 88200,
      "cl": 83790,
      "ch": 92610
    }
  ]
}
```

> **Lưu ý:** Field trong `rows` là format của Vietcap API (không transform). Next.js app đã biết cách đọc format này qua `vietcapBoardSnapshotService`.

---

## 4. Vietcap Market Index

**Key:** `board:vietcap:market_index`

**TTL:** 120 giây (2 phút)

**Ghi bởi:** Asynq task `board:vietcap:market_index` (mỗi 30 giây)

**Đọc bởi:** `/api/board/vietcap-market-index`

**Key phụ — timestamp:** `board:vietcap:market_index:updated_at`  
Lưu string ISO 8601, dùng để client check data freshness.

**Format (JSON string):**
```json
{
  "fetchedAt": "2026-04-06T09:15:00Z",
  "count": 6,
  "rows": [
    {
      "indexId": "VNINDEX",
      "indexValue": 1285.34,
      "indexChange": 12.45,
      "percentChange": 0.9788,
      "totalTrade": 892340000,
      "totalValue": 24567890000000,
      "advances": 187,
      "noChanges": 45,
      "declines": 98
    }
  ]
}
```

---

## 5. KB Intraday Chart

### Key theo ngày

**Key:** `board:kb:intraday:{SYMBOL}:{DATE}`

**Ví dụ:** `board:kb:intraday:VNINDEX:06-04-2026`

**TTL:** 86400 giây (24 giờ)

**Ghi bởi:** Asynq task `board:kb:intraday` (mỗi 60 giây)

### Key "latest" (shortcut)

**Key:** `board:kb:intraday:{SYMBOL}:latest`

**Ví dụ:** `board:kb:intraday:VNINDEX:latest`

**TTL:** 86400 giây (24 giờ)

**Đọc bởi:** `/api/board/kb-index-intraday?symbol={SYMBOL}`

**Format (JSON string):**
```json
{
  "fetchedAt": "2026-04-06T09:15:00Z",
  "uiSymbol": "VNINDEX",
  "sourceSymbol": "VNINDEX",
  "date": "06-04-2026",
  "rows": [
    {
      "time": 900,
      "open": 1272.89,
      "high": 1278.50,
      "low": 1272.00,
      "close": 1275.30,
      "volume": 12340000
    }
  ]
}
```

---

## Cách Next.js API route đọc Redis

### Pattern cơ bản

```typescript
import { createClient } from 'redis';

const redis = createClient({ url: process.env.REDIS_URL });
await redis.connect();

// Đọc key
const raw = await redis.get('board:vietcap:snapshot:VN30');
if (!raw) {
  // Fallback: gọi upstream API nếu Redis không có data
  return fetchFromUpstream();
}

return JSON.parse(raw);
```

### Thứ tự ưu tiên (Redis-first pattern)

```
1. GET từ Redis
2. Nếu Redis có data → trả về ngay (fast path)
3. Nếu Redis không có data (TTL hết hoặc worker chưa chạy):
   a. Gọi upstream API như cũ (fallback)
   b. Optionally: SET vào Redis để cache
4. Trả về kết quả
```

---

## Monitoring Redis

```bash
# Xem tất cả keys board
redis-cli keys "board:*"

# Xem TTL còn lại của một key
redis-cli ttl "board:vietcap:snapshot:VN30"

# Xem nội dung key
redis-cli get "board:vietcap:market_index" | jq .

# Memory usage
redis-cli info memory

# Monitor real-time
redis-cli monitor
```

---

## Giới hạn và lưu ý

| Vấn đề | Giải pháp |
|---|---|
| Key expire khi worker down | Next.js fallback về upstream API |
| Data stale (> TTL) | Tăng TTL hoặc giảm polling interval |
| Redis OOM | Đặt `maxmemory` + `allkeys-lru` (đã cấu hình trong docker-compose) |
| Nhiều Next.js instance | Không vấn đề — tất cả đọc cùng Redis |
| DNSE tick symbols giới hạn 300 | Dùng `DNSE_TICK_SYMBOLS` để chọn symbols quan trọng nhất |
