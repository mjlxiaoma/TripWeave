# TripWeave 部署手册

目标环境：Oracle Cloud Always Free（Ampere A1，4C/24G，Ubuntu 22.04），全程 ¥0。

## 架构

```
公网 :80
  │
nginx 容器（前端静态 + /api 反代，SSE 关缓冲）
  │
api 容器（Go，启动时自动跑迁移 RUN_MIGRATE=true）
  │
postgres + redis 容器（仅内网，不暴露端口）
```

## 首次上线

```bash
# 1. 服务器上初始化（装 Docker + 放通 80/443）
sudo bash deploy/scripts/setup.sh

# 2. 拉代码 + 配机密
git clone <你的仓库> && cd tripweave
cp deploy/.env.example .env.prod
nano .env.prod   # 填 DB_PASSWORD / REDIS_PASSWORD / JWT_SECRET / DEEPSEEK_API_KEY / AMAP_SERVICE_KEY / SMTP_* / APP_BASE_URL

# 3. 上线
bash deploy/scripts/deploy.sh
```

## 日常更新

```bash
bash deploy/scripts/deploy.sh   # git pull → 重建 → 滚动更新（含自动迁移）
```

## 域名 + HTTPS（可选，买了域名再做）

1. 域名 DNS A 记录指向服务器公网 IP
2. 把域名接入 Cloudflare（免费计划）：改 NS 到 Cloudflare → 开代理 → SSL/TLS 模式选 Flexible 即可免费 HTTPS（源站仍是 80）
3. `.env.prod` 里 `APP_BASE_URL` / `CORS_ORIGINS` 改成 `https://你的域名`，重新 `deploy.sh`

## 高德控制台（拿到 IP/域名后）

- **Web 服务 key**（`AMAP_SERVICE_KEY`）：白名单加服务器公网 IP
- **JS API key**（`VITE_AMAP_KEY`）：域名白名单加你的域名（或 IP）

## 机密清单

| 文件 | 位置 | 进 git？ |
|---|---|---|
| `.env.prod` | 服务器仓库根 | 否（.gitignore 的 `.env.*` 覆盖） |
| SSH 私钥 | 你本机 | 否 |
