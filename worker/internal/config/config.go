// Package config loads và cung cấp cấu hình runtime cho worker.
// Tất cả giá trị đều đọc từ biến môi trường; nếu không có thì dùng giá trị mặc định.
package config

import (
	"os"
	"strconv"
	"strings"
)

// Config giữ toàn bộ cấu hình của worker.
type Config struct {
	// Redis
	RedisAddr     string
	RedisPassword string
	RedisDB       int

	// DNSE WebSocket
	DnseAPIKey    string
	DnseAPISecret string
	DnseWsURL     string

	// Danh sách board DNSE sẽ subscribe (G1=HOSE, G2=HNX, G3=UPCOM)
	DnseBoards []string

	// Market index DNSE sẽ subscribe
	DnseMarketIndexes []string

	// Danh sách symbol ticker cụ thể muốn nhận tick realtime từ DNSE.
	// Để trống = dùng DefaultStockSymbols (giống STOCK_SYMBOLS mặc định).
	DnseTickSymbols []string

	// DNSE_DEBUG=1: log vài mẫu payload JSON đã auth nhưng chưa map được vào tick/index.
	DnseDebug bool

	// Vietcap
	// Các nhóm cổ phiếu cần lấy snapshot (VN30, HOSE, HNX, UPCOM, ...)
	VietcapGroups []string

	// KB Securities - danh sách symbol cho intraday chart
	KbSymbols []string

	// Khoảng thời gian (giây) polling Vietcap snapshot
	VietcapSnapshotIntervalSec int

	// Khoảng thời gian (giây) polling Vietcap market index
	VietcapMarketIndexIntervalSec int

	// Khoảng thời gian (giây) polling KB intraday
	KbIntradayIntervalSec int
}

// Default stock symbols for VN30, HNX30, UPCOM
var DefaultStockSymbols = []string{
	// VN30 - HOSE Top 30
	"ACB", "BID", "BVH", "CTG", "FPT", "GAS", "GVR", "HPG", "KDH", "MBB",
	"MSN", "MWG", "NVL", "PDR", "PGC", "PHG", "PLX", "PNJ", "POW", "SBT",
	"SSI", "STB", "TCB", "TPB", "VCB", "VHM", "VIC", "VNM", "VPB", "VRE",
	// HNX30 - HNX Top 30
	"ALT", "AMD", "ATS", "BAB", "BCM", "BDG", "BKC", "BLS", "BST", "BTP",
	"C69", "CAP", "DBC", "DGC", "DHT", "DIG", "DL1", "DNP", "DXP", "H9A",
	"HBC", "HUT", "KLS", "L14", "LAS", "LDG", "MBG", "MBS", "NTP", "PVB",
	"PVT", "S96", "SCR", "SHA", "SHB", "SHS", "STT", "TIG", "TMB", "VNX",
	// UPCOM - Một số cổ phiếu UPCOM phổ biến
	"B52", "BTS", "C4G", "CAG", "CNN", "CRC", "CSM", "CTF", "CTI", "D11",
	"D2D", "DAG", "DAH", "DIC", "DNE", "DRH", "DS3", "EIB", "EMC", "EVG",
	"FBA", "FCM", "FGL", "FMC", "FOT", "GEX", "GMC", "GTN", "HAG", "HAT",
	"HDB", "HDC", "HHC", "HHG", "HLA", "HLG", "HNG", "HPI", "HPM", "HQC",
	"HQT", "HRB", "HSI", "HU3", "HU6", "HVN", "IBC", "ICT", "IDJ", "IDV",
	"Inch", "ITD", "JVC", "KAC", "KBC", "KDC", "KDM", "KGC", "KHC", "KOS",
	"KPF", "KRC", "KSB", "KSH", "KTB", "KTC", "KTG", "KTT", "L10", "L18",
	"L40", "LAF", "LDP", "LEC", "LG9", "LHC", "LIX", "LM8", "LOC", "LPS",
	"LTC", "MAC", "MBW", "MCG", "MDC", "MEL", "MIC", "MIG", "MOS", "MPC",
	"NAV", "NBB", "NCT", "NDN", "NHT", "NTH", "NTK", "NTP", "NVT", "OCB",
	"OIL", "ONE", "ORS", "PAC", "PC1", "PCE", "PCG", "PIV", "PJICO", "PLA",
	"PLC", "PLE", "PMG", "PMT", "POT", "POW", "PPE", "PPY", "PTB", "PTH",
	"PTL", "PTT", "QCC", "QNC", "QNS", "QTA", "QTC", "QTD", "QTP", "RCL",
	"RDG", "RIC", "S55", "SAV", "SBT", "SCB", "SCO", "SCR", "SEB", "SFC",
	"SGB", "SGC", "SGD", "SGE", "SGR", "SHA", "SHG", "SHP", "SIM", "SJD",
	"SJE", "SJF", "SJM", "SJV", "SKG", "SMA", "SMB", "SMC", "SMP", "SMS",
	"SMT", "SNZ", "SPC", "SPF", "SPP", "SRA", "SRC", "SRF", "SSS", "STC",
	"SVC", "SVT", "SZB", "T12", "T64", "TA3", "TAG", "TAT", "TB8", "TC6",
	"TCC", "TCD", "TCM", "TCO", "TCP", "TCR", "TCT", "TDH", "TEG", "THA",
	"THB", "THD", "THG", "THI", "THJ", "THL", "TMB", "TMS", "TMT", "TMX",
	"TNC", "TNG", "TNP", "TNT", "TOP", "TOS", "TPC", "TPW", "TRA", "TRC",
	"TRI", "TS4", "TSC", "TSG", "TST", "TTB", "TTC", "TTE", "TTF", "TTH",
	"TTL", "TTT", "TV2", "TVC", "TVG", "TVM", "TVN", "TVT", "TXM", "UDC",
	"UIC", "VAB", "VAT", "VC1", "VC2", "VC3", "VC5", "VC6", "VC7", "VCA",
	"VCB", "VCC", "VCE", "VCG", "VCI", "VCW", "VDL", "VEE", "VE3", "VE4",
	"VEC", "VEH", "VEI", "VEJ", "VEL", "VEM", "VEN", "VEP", "VES", "VET",
	"VFC", "VFG", "VGC", "VGD", "VGF", "VGH", "VGI", "VGL", "VGN", "VGP",
	"VGR", "VH0", "VHC", "VHD", "VHF", "VHM", "VHN", "VHP", "VHR", "VIC",
	"VIS", "VIT", "VJV", "VKB", "VMC", "VMD", "VME", "VMG", "VMS", "VMU",
	"VNA", "VNB", "VNE", "VNG", "VNL", "VNM", "VNP", "VNR", "VNS", "VNT",
	"VNX", "VOC", "VOS", "VOT", "VOV", "VPA", "VPD", "VPE", "VPG", "VPH",
	"VPK", "VPR", "VPS", "VPT", "VPW", "VRA", "VRC", "VRG", "VRI", "VRL",
	"VRM", "VRS", "VRT", "VSA", "VSB", "VSE", "VSF", "VSG", "VSM", "VSN",
	"VSP", "VST", "VSV", "VT8", "VTC", "VTH", "VTJ", "VTL", "VTN", "VTP",
	"VTR", "VTS", "VTV", "VTX", "VVA", "VVB", "VVD", "VVI", "VVG", "VVT",
}

// Load đọc cấu hình từ environment variables.
func Load() *Config {
	// Load stock symbols from env or use default
	stockSymbolsEnv := getEnv("STOCK_SYMBOLS", "")
	stockSymbols := splitComma(stockSymbolsEnv)
	if len(stockSymbols) == 0 {
		stockSymbols = DefaultStockSymbols
	}

	// Load tick symbols from env or use default (same as stock symbols)
	tickSymbolsEnv := getEnv("DNSE_TICK_SYMBOLS", "")
	tickSymbols := splitComma(tickSymbolsEnv)
	if len(tickSymbols) == 0 {
		tickSymbols = DefaultStockSymbols
	}

	// KB intraday: ưu tiên KB_SYMBOLS (theo .env.example). Nếu không set thì dùng STOCK_SYMBOLS / default.
	kbSymbols := splitComma(getEnv("KB_SYMBOLS", ""))
	if len(kbSymbols) == 0 {
		kbSymbols = stockSymbols
	}

	return &Config{
		RedisAddr:     getEnv("REDIS_ADDR", "localhost:6379"),
		RedisPassword: getEnv("REDIS_PASSWORD", ""),
		RedisDB:       getEnvInt("REDIS_DB", 0),

		DnseAPIKey:    getEnv("DNSE_API_KEY", ""),
		DnseAPISecret: getEnv("DNSE_API_SECRET", ""),
		DnseWsURL:     getEnv("DNSE_WS_URL", "wss://ws-openapi.dnse.com.vn/v1/stream?encoding=json"),

		DnseBoards:        splitComma(getEnv("DNSE_BOARDS", "G1,G2,G3")),
		DnseMarketIndexes: splitComma(getEnv("DNSE_MARKET_INDEXES", "VNINDEX,VN30,HNXIndex,HNX30,HNXUpcomIndex")),
		DnseTickSymbols:   tickSymbols,
		DnseDebug:         getEnvBool("DNSE_DEBUG", false),

		VietcapGroups: splitComma(getEnv("VIETCAP_GROUPS", "")),

		KbSymbols: kbSymbols,

		VietcapSnapshotIntervalSec:    getEnvInt("VIETCAP_SNAPSHOT_INTERVAL_SEC", 30),
		VietcapMarketIndexIntervalSec: getEnvInt("VIETCAP_MARKET_INDEX_INTERVAL_SEC", 30),
		KbIntradayIntervalSec:         getEnvInt("KB_INTRADAY_INTERVAL_SEC", 60),
	}
}

func getEnv(key, fallback string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return fallback
}

func getEnvBool(key string, fallback bool) bool {
	v := strings.TrimSpace(strings.ToLower(getEnv(key, "")))
	if v == "" {
		return fallback
	}
	return v == "1" || v == "true" || v == "yes"
}

func getEnvInt(key string, fallback int) int {
	v := getEnv(key, "")
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return fallback
	}
	return n
}

func splitComma(s string) []string {
	var result []string
	for _, part := range strings.Split(s, ",") {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}
