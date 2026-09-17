# TripWeave 部署手册

**正式公网（免信用卡，推荐）**：Render + Neon + Upstash
**自托管 VPS**：Oracle Always Free（`docker-compose.prod.yml`）

---

## 方案 A：Render（正式公网，¥0，免信用卡）

`render.yaml` 是蓝图（Infrastructure as Code），Render 读它一键建服务。

### 1. 注册三个服务（全部邮箱/GitHub 登录，不要卡）

| 服务 | 用途 | 注册 |
|---|---|---|
| **Render** | API（Docker）+ 前端静态站 | render.com（GitHub 登录） |
| **Neon** | PostgreSQL 500MB | neon.tech（GitHub 登录） |
| **Upstash** | Redis | upstash.com（GitHub 登录） |

### 2. Neon 建库（拿 DATABASE_URL）

Neon → New Project → 区域选 **Singapore** → 拿到连接串，形如：
```
postgresql://user:pass@ep-xxx.ap-southeast-1.aws.neon.tech/tripweave?sslmode=require
```
（**必须带 `sslmode=require`**）

### 3. Upstash 建 Redis（拿地址+密码）

Upstash → Create Database → 区域 Singapore → 拿到：
- Endpoint（host，如 `xxx.ap-southeast-1.upstash.io`）
- Port（通常 `6379`）→ `REDIS_ADDR = host:port`
- Password

### 4. Render 一键部署

Render Dashboard → **New → Blueprint** → 选你的仓库 → Render 读 `render.yaml` 建出 `tripweave-api` + `tripweave-web`。

然后在 dashboard 给两个服务补 `sync:false` 的机密：

**tripweave-api**：DATABASE_URL / REDIS_ADDR / REDIS_PASSWORD / DEEPSEEK_API_KEY / AMAP_SERVICE_KEY / SMTP_USER / SMTP_PASS / SMTP_FROM / APP_BASE_URL(=前端地址) / CORS_ORIGINS(=前端地址)

**tripweave-web**：VITE_API_BASE(=`https://tripweave-api.onrender.com/api/v1`) / VITE_AMAP_KEY / VITE_AMAP_SECURITY_CODE

### 注意
- **免费档闲置 15 分钟休眠**，首次访问冷启动约 30 秒
- API 启动自动跑迁移（`RUN_MIGRATE=true`，镜像内含 migrations）
- 之后 push 到 main 即自动重新部署

---

## 方案 B：自托管 VPS（Oracle Always Free，¥0）

### 架构
```
公网 :80
  │
nginx 容器（前端静态 + /api 反代，SSE 关缓冲）
  │
api 容器（Go，启动时自动跑迁移）
  │
postgres + redis 容器（仅内网）
```

### 首次上线
```bash
sudo bash deploy/scripts/setup.sh          # 装 Docker + 放通 80/443
git clone <你的仓库> && cd tripweave
cp deploy/.env.example .env.prod && nano .env.prod   # 填机密
bash deploy/scripts/deploy.sh               # 构建 + 上线
```

### 日常更新
```bash
bash deploy/scripts/deploy.sh
```

### 域名 + HTTPS（可选）
域名接 Cloudflare 免费计划（改 NS）→ 开代理 → SSL/TLS 选 Flexible → `.env.prod` 改 https 地址重新 deploy。

### 高德控制台（拿到 IP/域名后）
- Web 服务 key：白名单加服务器公网 IP（VPS）—— Render 出站 IP 不固定，Render 路线下高德 Web 服务 key 的 IP 白名单要设为「不限制」或用 Render 的静态出站 IP（需付费档）。**免费档建议高德控制台不设 IP 白名单**（key 已足够）。
- JS API key：域名白名单加 Render 域名（`tripweave-web.onrender.com`）或你的域名

## 机密清单
| 文件 | 位置 | 进 git？ |
|---|---|---|
| `.env.prod` | VPS 仓库根 | 否（.gitignore 的 `.env.*`） |
| Render/Neon/Upstash 机密 | 各自 dashboard | 否 |
| SSH 私钥 | 你本机 | 否 |
