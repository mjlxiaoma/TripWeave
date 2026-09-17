# TripWeave API 生产镜像（构建上下文 = 仓库根目录，要访问 apps/api 与 migrations）
# 自托管（compose）时 migrations 走挂卷；Render 等无卷托管环境用镜像内副本。

# --- 构建阶段 ---
FROM golang:1.26-alpine AS build
WORKDIR /src

# 先拷依赖清单利用层缓存
COPY apps/api/go.mod apps/api/go.sum ./apps/api/
RUN cd apps/api && go mod download

COPY apps/api ./apps/api
# CGO 关闭：纯静态二进制，alpine 即可运行
RUN cd apps/api && CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/server ./cmd/server

# --- 运行阶段 ---
FROM alpine:3.21
RUN apk add --no-cache ca-certificates tzdata wget \
    && adduser -D -u 10001 appuser

WORKDIR /app
COPY --from=build /out/server /app/server
# 迁移脚本随镜像（MIGRATE_DIR=file:///app/migrations）；自托管时该路径会被卷覆盖
COPY migrations ./migrations

USER appuser
EXPOSE 8080

ENTRYPOINT ["/app/server"]
