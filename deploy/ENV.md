# 环境变量与机密说明

TripWeave 全部运行配置按「用在哪个服务 / 干什么 / 从哪获取」归纳。
本地产开发用 `apps/api/.env` + `apps/web/.env`；生产（Render）在 dashboard 的 Environment 页填同样的键。

## 速查表

### 数据库与缓存（API）

| 变量 | 干什么 | 从哪获取 |
|---|---|---|
| `DATABASE_URL` | PostgreSQL 连接串，API 的所有业务数据（用户/行程/活动/地点/AI 对话）都存这里。格式 `postgresql://用户:密码@主机/库?sslmode=require` | **Neon** 控制台项目首页的 Connection string（新加坡区，必须带 `sslmode=require`） |
| `REDIS_ADDR` | Redis 地址 `host:port`。用于：登录态的 refresh token 轮换存储、注册邮箱验证码、AI 生成的单行程互斥锁 | **Upstash** 数据库详情页的 Endpoint |
| `REDIS_PASSWORD` | Redis 密码 | 同上页的 Password |
| `REDIS_TLS` | 是否用 TLS 连 Redis。Upstash 强制 TLS，必须 `true`；本地自建 Redis 留空即可 | 固定值 `true`（Render 蓝图里已内置，不用手填） |

### 登录态（API）

| 变量 | 干什么 | 从哪获取 |
|---|---|---|
| `JWT_SECRET` | 给 JWT 登录令牌签名的 HMAC 密钥，泄露等于任何人都能伪造登录态 | **Render 自动生成**（蓝图中 `generateValue:true`）；本地/自托管自己生成一个 64 位随机串 |
| `ACCESS_TOKEN_TTL` | 访问令牌有效期，到期靠 refresh token 静默续期 | 默认 `30m`，不用改 |
| `REFRESH_TOKEN_TTL` | 刷新令牌有效期（存 Redis 里轮换） | 默认 `720h`（30 天），不用改 |

### AI（API）

| 变量 | 干什么 | 从哪获取 |
|---|---|---|
| `DEEPSEEK_API_KEY` | DeepSeek 大模型 API key，AI 行程生成/对话/工具调用的核心 | **platform.deepseek.com** → API Keys → 创建 |
| `DEEPSEEK_BASE_URL` | API 地址 | 默认 `https://api.deepseek.com`，不用改 |
| `DEEPSEEK_MODEL` | 模型名 | 默认 `deepseek-chat`，不用改 |
| `AI_MAX_TOOL_ROUNDS` | 一轮对话里 AI 最多调用几轮工具（防失控） | 默认 `5`，不用改 |

### 地图（API 用 Web 服务 key / 前端用 JS key）

| 变量 | 干什么 | 从哪获取 |
|---|---|---|
| `AMAP_SERVICE_KEY` | 高德 **Web 服务** key（服务端）：POI 搜索、地理编码、路线规划、天气。只出现在后端，不进前端 | **lbs.amap.com** 控制台 → 应用管理 → 创建应用 → 添加「Web服务」key |
| `VITE_AMAP_KEY` | 高德 **Web 端（JS API）** key（前端）：浏览器里渲染地图底图、marker、路线。构建时打进静态文件，是公开的（所以要在高德控制台配域名白名单） | 同控制台添加「Web端(JS API)」key |
| `VITE_AMAP_SECURITY_CODE` | 与 JS API key 配套的**安全密钥**（securityJsCode），2021 年底起强制 | 同上，JS API key 页面的「安全密钥」 |

### 邮件（API）

| 变量 | 干什么 | 从哪获取 |
|---|---|---|
| `SMTP_HOST` / `SMTP_PORT` | 发信服务器 | QQ 邮箱固定 `smtp.qq.com` / `465` |
| `SMTP_USER` | 发信邮箱账号 | 你的 QQ 邮箱地址 |
| `SMTP_PASS` | ⚠️ **不是 QQ 密码**，是 QQ 邮箱的**授权码** | QQ 邮箱 → 设置 → 账户 → 开启「POP3/SMTP服务」→ 生成的 16 位授权码 |
| `SMTP_FROM` | 发件人显示名 | 如 `TripWeave <you@qq.com>` |

用途：注册时发 6 位邮箱验证码。不配时降级为把验证码打进服务端日志（开发用）。

### 站点（API + 前端）

| 变量 | 干什么 | 从哪获取 |
|---|---|---|
| `APP_BASE_URL` | 前端的公网地址。用于：注册验证邮件里的回跳链接、分享链接等需要拼绝对 URL 的地方 | Render 上 web 服务的地址，如 `https://tripweave-web.onrender.com` |
| `CORS_ORIGINS` | 允许跨域调 API 的浏览器来源。前端和 API 分属两个域名（`tripweave-web` vs `tripweave-api`），所以要把前端域名加进来 | 同 `APP_BASE_URL` |
| `VITE_API_BASE` | 前端调后端 API 的基地址，构建时打进静态文件 | API 服务地址 + `/api/v1`，如 `https://tripweave-api.onrender.com/api/v1` |

### 其他（一般不用动）

| 变量 | 干什么 | 默认 |
|---|---|---|
| `APP_ENV` | `production` 时强制校验 JWT_SECRET 等生产不变量 | Render/compose 已设为 `production` |
| `ADDR` | API 监听端口 | `:8080` |
| `RUN_MIGRATE` | 启动时自动跑数据库迁移（建/升表结构） | 生产设 `true` |
| `MIGRATE_DIR` | 迁移 SQL 文件目录 | `file:///app/migrations`（镜像内） |

## 本地开发 vs 生产对照

| 场景 | 数据库 | Redis | 配置文件 |
|---|---|---|---|
| 本地开发 | 本机 docker（`docker-compose.yml`，127.0.0.1:5433） | 本机 docker（127.0.0.1:6380） | `apps/api/.env`、`apps/web/.env` |
| Render 生产 | Neon（新加坡） | Upstash（新加坡，TLS） | Render dashboard Environment 页 |
| VPS 自托管 | 容器内 postgres | 容器内 redis | `.env.prod`（git 已忽略） |

## 安全须知

1. **所有密码类值都不进 git**：`.env*` 已被 `.gitignore` 忽略（`.env.example` 模板除外）。
2. 在聊天/截图里贴过明文的（如本次 Neon/Redis/DeepSeek 的串），建议用完后在对应控制台 **Rotate/重置** 一次。
3. `VITE_AMAP_KEY` 注定公开（打进前端），所以**必须**在高德控制台配域名白名单，防别人盗用额度。
4. `JWT_SECRET` 泄露 = 任何人可伪造任意用户登录，生产环境用 Render 自动生成的强随机值，不要自己编一个简单串。
