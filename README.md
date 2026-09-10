# TripWeave

AI 个性化旅行规划平台 —— 将碎片化的旅行需求，编织成真正可执行的旅行计划。

## 技术栈

- 前端：React + TypeScript + Vite + Tailwind CSS（`apps/web`）
- 后端：Go 标准库 net/http，模块化单体（`apps/api`）
- 数据库：PostgreSQL ｜ 缓存：Redis
- 部署：Docker + Nginx + Linux

## 目录结构

```text
apps/
  web/        # React 前端
  api/        # Go 后端（cmd/server 入口，internal 业务分层，pkg 基础设施）
packages/
  types/      # 前后端共享 TypeScript 类型
  config/     # 共享配置
  eslint-config/
deploy/       # docker / nginx / 部署脚本
migrations/   # 数据库迁移 SQL
docs/         # 设计文档（仅本地，不上传）
```

## 本地开发

```bash
pnpm install          # 安装前端依赖
pnpm dev:web          # 启动前端 http://localhost:5173
pnpm dev:api          # 启动后端 http://localhost:8080（需本机 Go 环境）

docker compose up -d  # 启动 PostgreSQL + Redis（可选，本步暂未使用）
```

环境变量参考 `.env.example`。