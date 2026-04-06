// Package rediskeys định nghĩa tất cả Redis key được dùng trong hệ thống.
// Tập trung key ở đây để tránh lỗi typo và dễ dàng tìm kiếm khi cần thay đổi schema.
//
// Quy ước đặt tên key:
//   board:<nguồn>:<loại_data>:<định_danh>
//
// Ví dụ:
//   board:dnse:tick:HPG
//   board:vietcap:snapshot:VN30
//   board:kb:intraday:VNINDEX:06-04-2026
package rediskeys

import "fmt"

const (
	// TTL mặc định cho dữ liệu realtime/near-realtime
	TTLTickSec         = 300 // 5 phút - dữ liệu tick DNSE
	TTLSnapshotSec     = 120 // 2 phút - snapshot Vietcap
	TTLMarketIndexSec  = 120 // 2 phút - market index
	TTLKbIntradaySec   = 86400 // 24 giờ - intraday chart (theo ngày)

	// Prefix
	prefixDnseTick        = "board:dnse:tick:"
	prefixDnseMarketIndex = "board:dnse:market_index:"

	prefixVietcapSnapshot    = "board:vietcap:snapshot:"
	keyVietcapMarketIndex    = "board:vietcap:market_index"
	keyVietcapMarketIndexAt  = "board:vietcap:market_index:updated_at"

	prefixKbIntraday = "board:kb:intraday:"
)

// DnseTick trả về key lưu dữ liệu tick realtime từ DNSE cho một symbol.
// Ví dụ: board:dnse:tick:HPG
func DnseTick(symbol string) string {
	return prefixDnseTick + symbol
}

// DnseMarketIndex trả về key lưu giá trị market index realtime từ DNSE.
// Ví dụ: board:dnse:market_index:VNINDEX
func DnseMarketIndex(indexName string) string {
	return prefixDnseMarketIndex + indexName
}

// VietcapSnapshot trả về key lưu snapshot board từ Vietcap cho một group.
// Ví dụ: board:vietcap:snapshot:VN30
func VietcapSnapshot(group string) string {
	return prefixVietcapSnapshot + group
}

// VietcapMarketIndex trả về key lưu danh sách market index từ Vietcap.
// Đây là JSON array chứa tất cả index (VNINDEX, VN30, HNXIndex, ...).
func VietcapMarketIndex() string {
	return keyVietcapMarketIndex
}

// VietcapMarketIndexUpdatedAt trả về key lưu thời điểm cập nhật market index.
func VietcapMarketIndexUpdatedAt() string {
	return keyVietcapMarketIndexAt
}

// KbIntraday trả về key lưu dữ liệu intraday chart từ KB Securities.
// date có dạng "DD-MM-YYYY". Ví dụ: board:kb:intraday:VNINDEX:06-04-2026
func KbIntraday(symbol, date string) string {
	return fmt.Sprintf("%s%s:%s", prefixKbIntraday, symbol, date)
}

// KbIntradayLatest trả về key lưu kết quả intraday mới nhất (bất kể ngày).
// Key này luôn được overwrite khi có dữ liệu mới, dùng để fallback nhanh.
// Ví dụ: board:kb:intraday:VNINDEX:latest
func KbIntradayLatest(symbol string) string {
	return fmt.Sprintf("%s%s:latest", prefixKbIntraday, symbol)
}
