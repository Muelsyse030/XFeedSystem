# ---- 构建阶段 ----
FROM golang:1.24-alpine AS builder

RUN apk add --no-cache git ca-certificates

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o xfeed-api ./cmd/api

# ---- 运行阶段 ----
FROM alpine:3.21

RUN apk add --no-cache ca-certificates tzdata
ENV TZ=Asia/Shanghai

WORKDIR /app
COPY --from=builder /app/xfeed-api /usr/local/bin/xfeed-api
COPY --from=builder /app/configs /etc/xfeed/configs
COPY --from=builder /app/migrations /etc/xfeed/migrations

# 配置路径可通过 CONFIG_PATH 环境变量覆盖
ENV CONFIG_PATH=/etc/xfeed/configs

EXPOSE 8000

CMD ["xfeed-api"]
