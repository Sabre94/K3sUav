# Multi-stage build for UAV Agent
FROM golang:1.25-alpine AS builder

# 安装构建依赖
RUN apk add --no-cache git make

# 设置工作目录
WORKDIR /build

# 复制 go mod 文件
COPY go.mod go.sum ./
RUN go mod download

# 复制源代码
COPY . .

# 编译
RUN CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -a -installsuffix cgo -ldflags '-w -s' -o uav-agent ./cmd/agent/

# 最终镜像
FROM alpine:latest

# 安装 CA 证书（用于 HTTPS 请求）
RUN apk --no-cache add ca-certificates

# 创建非 root 用户
RUN addgroup -S uav && adduser -S uav -G uav

WORKDIR /app

# 从 builder 复制二进制文件
COPY --from=builder /build/uav-agent .

# 使用非 root 用户运行
USER uav

# 暴露端口（如果后续需要 metrics endpoint）
EXPOSE 8080

# 启动命令
ENTRYPOINT ["/app/uav-agent"]
