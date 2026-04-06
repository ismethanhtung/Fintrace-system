# ─── Stage 1: Build ───────────────────────────────────────────────────────────
# Dùng Go image chính thức với alpine để image nhỏ gọn.
FROM golang:1.22-alpine AS builder

WORKDIR /app

# Cài thêm ca-certificates để HTTPS requests hoạt động, và git cho go mod
RUN apk add --no-cache ca-certificates git tzdata

# Copy go.mod trước để cache layer dependency
COPY go.mod go.sum ./
RUN go mod download

# Copy toàn bộ source code
COPY . .

# Build binary với các flag tối ưu:
# - CGO_ENABLED=0: build static binary (không phụ thuộc libc)
# - -ldflags "-s -w": strip debug info để binary nhỏ hơn
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /fintrace-worker ./main.go

# ─── Stage 2: Runtime ─────────────────────────────────────────────────────────
# Dùng image tối thiểu để chạy binary
FROM alpine:3.19

# ca-certificates: cho HTTPS; tzdata: cho timezone Asia/Ho_Chi_Minh
RUN apk add --no-cache ca-certificates tzdata

WORKDIR /app

# Copy binary từ stage build
COPY --from=builder /fintrace-worker /app/fintrace-worker

# Worker chạy như non-root user để bảo mật
RUN addgroup -S worker && adduser -S worker -G worker
USER worker

ENTRYPOINT ["/app/fintrace-worker"]
