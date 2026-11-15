# VPS 测速

登录后打开 `/speedtest`，测试当前浏览器与应用后端节点之间的单连接速率。页面与「Cloudflare 优选」独立，不保存历史、不写入磁盘，也不需要数据库迁移。

## 测量过程

连接与预热 → 20 次空闲回显（间隔 200 ms）→ 下载约 10 秒 → 上传约 10 秒 → 汇总。首次 WebSocket 握手单独计时，预热不计入空闲延迟。负载期间每 500 ms 发送回显，单次超时 2 秒。停止、离页和退出登录会取消请求并关闭连接；部分结果标记为未完成。

速率使用十进制 Mbps：`bytes × 8 / seconds / 1,000,000`。上传串行发送，从 64 KiB 起根据耗时调整到 4 KiB～8 MiB，每次最多增大一倍，只累计服务端完整确认的字节数。约 10 秒后停止发起新上传，当前请求最多等待 15 秒（服务端读取上限 10 秒，额外预留代理与响应时间）；结果展示实际耗时。下载按实际读取的流字节计数，测试开始后不会重传流；上传收到 401 时刷新凭据，但不重传本次上传体。

延迟展示中位数、最近秩法 P95（排序后的第 `ceil(n × 0.95)` 个样本）、相邻成功 RTT 差值绝对值的平均值（抖动）。没有成功样本时中位数显示「样本不足」，不足 2 个成功样本时 P95 和抖动显示「样本不足」。请求失败率为该阶段回显失败或超时次数除以发送数，**不是 IP 层丢包率**。IPv4/IPv6 表示此次连接的客户端地址族，不代表同时测量双栈线路。

## 接口

HTTP 接口使用 `Authorization: Bearer <access token>`，需要 access token，refresh token 无效。即使出现在全局认证白名单内，测速接口也必须登录。JSON 沿用 `{code, message, data}`，`code: 200` 才表示成功；认证失败返回 HTTP 401。

| 接口 | 成功数据或格式 | 限制 |
| --- | --- | --- |
| `GET /api/info` | `data: {nodeName, clientIP, addressFamily}` | 禁止缓存 |
| `GET /api/echo` | WebSocket 原样回显文本或二进制 | 消息 ≤ 1024 字节；寿命 ≤ 90 秒；读取空闲 10 秒；写入 2 秒 |
| `GET /api/download` | `application/octet-stream` 随机二进制流 | 服务端约 20 秒上限；写入等待 5 秒；客户端约 10 秒取消 |
| `POST /api/upload` | `data: {bytes: 65536}` | ≤ 8 MiB；读取 10 秒；不完整读取不确认字节 |

WebSocket 使用 `['ocopion-speedtest', 'access_token.' + accessToken]` 子协议列表，服务端只协商 `ocopion-speedtest`。凭据不进入 URL。压缩关闭，Origin 必须同源或精确命中配置。HTTP 请求示例见 [speedtest.http](../rest/speedtest.http)。

```json
{
  "code": 200,
  "message": "操作成功",
  "data": {
    "nodeName": "我的 VPS",
    "clientIP": "203.0.113.10",
    "addressFamily": "IPv4"
  }
}
```

## 配置

在 `prod.local.yml` 或部署配置中填写以下项目：

```yaml
server:
  trusted_proxies:
    - "127.0.0.1"
    - "::1"
speedtest:
  node_name: "我的 VPS"
  allowed_origins:
    - "https://example.com"
```

默认不信任任何代理转发头，只使用直连地址。同源 WebSocket 默认允许，非同源前端必须填写完整 Origin（协议、域名、可选端口，无路径和尾斜杠）。开发配置允许 `http://localhost:5175` 和 `http://127.0.0.1:5175`，Vite 已启用 `ws: true`，将 `/ocopion/api/*` 转发至后端 `/api/*`。

环境变量为 `OCOPION_SPEEDTEST_NODE_NAME`、`OCOPION_SPEEDTEST_ALLOWED_ORIGINS`、`OCOPION_SERVER_TRUSTED_PROXIES`；列表环境变量使用逗号分隔。代理必须覆盖来自客户端的转发头，仅将实际代理 IP/CIDR 加入可信列表。通过代理得到的客户端 IP 及地址族依赖代理提供的真实地址。

## Nginx 部署示例

当前 `qyhever` 服务器的实际配置、已验证的候选文件和应用命令见 [线上配置调整](nginx/README.md)。该服务器现有 `^~ /ocopion/api/` 规则会阻止下方同级正则匹配，应使用候选文件中的精确匹配方案。

以下配置放入现有 HTTPS `server`，替换域名。后端监听回环地址或通过防火墙限制仅允许可信代理访问。当前前端统一 API 前缀为 `/ocopion/api`。

```nginx
location ~ ^/ocopion/api/(info|echo|download|upload)$ {
    rewrite ^/ocopion/api/(.*)$ /api/$1 break;
    proxy_pass http://127.0.0.1:8301;
    proxy_http_version 1.1;
    proxy_set_header Host $host;
    proxy_set_header Upgrade $http_upgrade;
    proxy_set_header Connection "upgrade";
    # 保留真实 Origin，服务端负责精确校验；不要伪造 Origin。
    proxy_set_header Origin $http_origin;
    proxy_set_header X-Forwarded-For $remote_addr;
    proxy_set_header X-Real-IP $remote_addr;
    proxy_set_header X-Forwarded-Proto $scheme;
    proxy_set_header Accept-Encoding "";
    proxy_buffering off;
    proxy_request_buffering off;
    proxy_cache off;
    gzip off;
    client_max_body_size 8m;
    client_body_timeout 10s;
    proxy_connect_timeout 5s;
    proxy_send_timeout 15s;
    proxy_read_timeout 100s;
    send_timeout 15s;
}
```

保留现有登录和其他 API 的反向代理配置，以及 Vue Router 的 `try_files $uri $uri/ /index.html`。如果启用了额外压缩模块，也需对此位置关闭。部署链路有 CDN 时应禁用这些路径的缓存、压缩和缓冲；否则测到的可能是 CDN 到浏览器的速度。此示例只提供部署说明，不会执行部署。

实现依据：[Gorilla WebSocket 官方文档](https://pkg.go.dev/github.com/gorilla/websocket)、[Vite 代理配置](https://vite.dev/config/server-options#server-proxy)。

## 验证

```sh
# 在 ocopion-backend 中运行 Go 全量测试。
go test ./...

# 可选：使用真实 Vite 配置验证 HTTP 路径改写与 WebSocket 代理。
# 需已安装前端依赖，不依赖已有开发服务或数据库。
SPEEDTEST_VITE_INTEGRATION=1 go test ./internal/module/speedtest -run TestViteProxy -v

# 在 ocopion-frontend-pc 中运行前端测试、类型检查与生产构建。
node --test tests/*.test.mjs
pnpm build
```

上线前浏览器验收：完成一次测速；下载和上传时分别取消并重测；离页和退出登录时确认请求停止；手机宽度检查布局；经 Vite 和 Nginx 分别确认 WebSocket Upgrade 成功以及回显持续采样。
