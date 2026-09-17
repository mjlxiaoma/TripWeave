# TripWeave 前端 + Nginx 反代（一体镜像）
# 构建上下文 = 仓库根目录（compose 里指定 context: .），要访问 pnpm-workspace 与 apps/web。

# --- 构建阶段 ---
FROM node:22-alpine AS build
RUN corepack enable && corepack prepare pnpm@11.4.0 --activate
WORKDIR /src

# 只拷依赖清单，最大化利用层缓存
COPY pnpm-workspace.yaml package.json pnpm-lock.yaml ./
COPY apps/web/package.json ./apps/web/
RUN pnpm install --frozen-lockfile

# 高德 JS API key（构建时注入静态文件）。Web 服务 key 不进这里（后端运行时注入）。
ARG VITE_AMAP_KEY
ARG VITE_AMAP_SECURITY_CODE
ENV VITE_AMAP_KEY=$VITE_AMAP_KEY \
    VITE_AMAP_SECURITY_CODE=$VITE_AMAP_SECURITY_CODE \
    VITE_API_BASE=/api/v1

COPY . .
RUN pnpm --filter web build

# --- 运行阶段 ---
FROM nginx:1.27-alpine
COPY --from=build /src/apps/web/dist /usr/share/nginx/html
COPY deploy/nginx/nginx.conf /etc/nginx/conf.d/default.conf

EXPOSE 80
HEALTHCHECK --interval=30s --timeout=3s --retries=3 \
  CMD wget -qO- http://127.0.0.1/ >/dev/null || exit 1
