#!/usr/bin/env bash
# TripWeave 日常部署（服务器上跑）：拉代码 → 构建 → 滚动更新。
# 用法：bash deploy/scripts/deploy.sh
set -euo pipefail
cd "$(dirname "$0")/../.."

echo "==> git pull"
git pull --ff-only

if [ ! -f .env.prod ]; then
  echo "缺少 .env.prod：先 cp deploy/.env.example .env.prod 并填入机密" >&2
  exit 1
fi

echo "==> 构建镜像"
docker compose -f docker-compose.prod.yml --env-file .env.prod build

echo "==> 滚动更新（自动迁移在 api 启动时执行，RUN_MIGRATE=true）"
docker compose -f docker-compose.prod.yml --env-file .env.prod up -d

echo "==> 清理悬空镜像"
docker image prune -f

echo "==> 状态"
docker compose -f docker-compose.prod.yml ps
