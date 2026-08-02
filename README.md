# 一键分站总控系统

一个独立的一键开站与分站运维系统，使用 Go 1.22、PostgreSQL、Vue 2.7、Docker Compose 和 Traefik 构建。

本仓库只包含总控系统，不包含任何被部署站点的业务源码或镜像。当前部署执行器仍遵循既有站点应用的环境变量、初始化接口和 Worker 约定；未来重新启用时，需要提供兼容镜像，或同步调整 `deploy/templates` 与 `backend/internal/control/executor.go` 中的适配逻辑。

## 功能

- HttpOnly 管理员会话、CSRF、登录失败锁定、公开接口限流和操作审计。
- 卡密批量创建、一次性明文显示、状态管理、过期、撤销和开站会话。
- 域名前缀原子保留、唯一约束和重复提交幂等处理。
- 客户开站向导、公开资料填写、Logo 上传、SSE 进度、断线轮询和失败重试。
- Agent 任务租约、幂等建库、独立密钥、Docker Compose 启停和健康检查。
- 初始化完成后再开放 Traefik 路由，并验证站点身份和公网 HTTPS。
- Reporter 上报心跳、用户与调用统计及脱敏渠道快照。
- 单站启停、重启、冻结、备份、升级、失败回滚和彻底删除。
- 概览、卡密、站点、任务、版本、节点和审计日志后台。
- 管理后台浅色与深色主题。

## 仓库结构

```text
backend/             Go API、Agent、Reporter、初始化工具和数据库迁移
frontend/            Vue 管理后台与公开开站页面
deploy/              Docker Compose、Traefik 和分站模板
docs/                OpenAPI 契约与部署说明
.env.example         本地开发配置模板
.env.production.example  生产配置模板
```

## 本地开发

1. 将 `.env.example` 复制为 `.env`，并生成四个互不相同的随机密钥。
2. 启动 PostgreSQL、迁移和 MinIO：

```bash
docker compose -f deploy/docker-compose.dev.yml up -d postgres migrate minio
```

3. 启动 API、Agent 和前端：

```bash
cd backend
go run ./cmd/api

# 新终端
cd backend
go run ./cmd/agent

# 新终端
cd frontend
npm ci
npm run dev
```

4. 创建管理员：

```bash
cd backend
ADMIN_USERNAME=admin ADMIN_PASSWORD='请替换为强密码' go run ./cmd/init-admin
```

默认前端地址为 `http://127.0.0.1:8081`，API 地址为 `http://127.0.0.1:8080`。

## 生产部署

生产示例位于 `deploy/docker-compose.production.yml`。先由 `.env.production.example` 创建 `.env.production`，填写域名、数据库、独立密钥和兼容站点镜像，再执行：

```bash
docker network create platform-proxy
docker compose --env-file .env.production -f deploy/docker-compose.production.yml build
docker compose --env-file .env.production -f deploy/docker-compose.production.yml up -d
docker compose --env-file .env.production -f deploy/docker-compose.production.yml run --rm \
  -e ADMIN_USERNAME=admin -e ADMIN_PASSWORD='请替换为强密码' \
  --entrypoint /app/init-admin control-api
```

示例编排将总控 Web 映射到 `19540`，将分站路由入口映射到 `19541`。端口可按部署环境修改。

## 站点镜像适配

总控不会构建被部署站点。版本管理中的镜像必须满足当前执行器约定：

- 提供 Web App 和 Worker 启动方式。
- 接受模板中定义的数据库、内部地址、维护令牌等环境变量。
- 提供安装、管理员初始化、公开资料写入、Ready 和 Reporter 所需接口。
- 响应包含用于路由身份验证的站点标识。

如果目标应用不符合这些约定，应先修改站点模板与执行器，再进行真实开站测试。

## 安全边界

- 不提交 `.env`、`.env.production`、上传文件、数据库数据、日志、构建产物或打包文件。
- `control-api` 不挂载 Docker Socket；只有 Agent 和 Traefik 接触 Docker。
- 卡密只保存 HMAC；管理员密码使用安全哈希；站点密钥使用主密钥加密。
- Reporter 使用每站独立令牌签名并防止重放。
- 总控不展示或回传站点上游 API Key。
- 上线前必须替换全部示例域名、镜像、密码和密钥。

## 测试

```bash
cd backend
go test ./...

cd ../frontend
npm ci
npm run build
```

PostgreSQL 集成测试默认跳过。设置指向专用测试数据库的 `CONTROL_TEST_DATABASE_URL` 后才会运行，禁止使用生产数据库。

API 契约见 `docs/openapi.yaml`，域名和部署方式见 `docs/deployment.md`。

