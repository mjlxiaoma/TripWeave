# TripWeave API 生产镜像
# 构建上下文 = apps/api（compose 里指定）。migrations 运行时挂卷注入（更新迁移无需重建镜像）。

# --- 构建阶段 ---
FROM golang:1.26-alpine AS build
WORKDIR /src

# 先拷依赖清单利用层缓存
COPY go.mod go.sum ./
RUN go mod download

COPY . .
# CGO 关闭：纯静态二进制，alpine 即可运行
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/server ./cmd/server

# --- 运行阶段 ---
FROM alpine:3.21
RUN apk add --no-cache ca-certificates tzdata wget \
    && adduser -D -u 10001 appuser

WORKDIR /app
COPY --from=build /out/server /app/server

USER appuser
EXPOSE 8080

# 迁移目录由 compose 挂载到 /app/migrations（MIGRATE_DIR=file:///app/migrations）
ENTRYPOINT ["/app/server"]
