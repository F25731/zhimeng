# 部署与域名接入

## 域名拓扑

```text
客户浏览器
  -> control.example.com / *.example.com
  -> TLS 入口或边缘反向代理
  -> control 子域：主服务器 19540（总控 Web）
  -> 其他子域：主服务器 19541（Traefik 站点路由）
  -> site-<uuid>-app:3000
```

通配子域必须使用独立反向代理配置并保留原始 `Host`。不要把通配子域转发到总控端口，否则客户只会看到开站页面。

```nginx
server {
    listen 443 ssl http2;
    server_name *.example.com;

    location / {
        proxy_pass http://203.0.113.10:19541;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto https;
        proxy_http_version 1.1;
        proxy_read_timeout 3600s;
    }
}
```

`control.example.com` 单独反代总控端口。通配证书必须覆盖 `*.example.com`，DNS 中总控记录和通配记录均指向 TLS 入口。

示例 IP `203.0.113.10` 属于文档保留网段，部署时必须替换为真实地址。

## 路由激活顺序

新站第一次启动时使用 `traefik.enable=false`，只允许 Agent 通过 Docker 网络访问。系统完成数据库初始化、管理员创建、公开资料写入、App Ready 和 Worker 检查后，才打开精确域名路由。

路由打开后 Agent 依次检查：

1. 通过 `SITE_ROUTER_URL` 携带原始 Host 请求 Traefik。
2. 响应头 `X-Control-Site-ID` 必须等于当前站点 ID。
3. 启用 `VERIFY_PUBLIC_HTTPS` 时，从公网 HTTPS 地址再次验证相同响应头。
4. 任一检查失败都会重新关闭路由，且不会把站点标记为 `active`。

生产环境建议：

```dotenv
SITE_ROUTER_URL=http://site-router
VERIFY_PUBLIC_HTTPS=true
ROUTE_PROBE_TIMEOUT_SECONDS=90
WORKER_READY_TIMEOUT_SECONDS=180
```

## Agent 文件和网络

每个站点目录为 `/opt/platform/sites/<site_uuid>`，包含 `.env`、`compose.yml` 和 `backups/`。`.env` 权限为 `0600`。站点容器同时加入路由网络和数据库私有网络。

## 首次上线检查

1. 所有迁移成功，且未执行任何 `*.down.sql`。
2. `control-api` 没有 Docker Socket，Agent 挂载了 Socket 和站点目录。
3. `POSTGRES_ADMIN_URL` 使用具备 `CREATE ROLE` 和 `CREATE DATABASE` 权限的控制账号。
4. `SITE_IMAGE_DEFAULT` 指向经过验证的兼容站点镜像。
5. Traefik 的 `forwardedHeaders.trustedIPs` 只包含真实 TLS 入口地址。
6. 创建测试卡密并开一个灰度站，确认初始化期间域名不可访问。
7. 完成后验证站点身份头、独立数据库、管理员登录、Worker 心跳和 Reporter 快照。

