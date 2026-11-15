# qyhever 线上 Nginx 配置调整

2026-11-14 通过 `ssh qyhever` 检查。线上使用 Docker 容器 `app` 内的 OpenResty，容器使用 host 网络；Go 服务监听 `8301`，网站为 `https://ocopion.qyhever.com`。宿主机没有 `nginx` 命令，检查和重载均在容器内执行。

## 配置对应关系

| 本目录文件 | 宿主机目标文件 |
| --- | --- |
| `ocopion-api.conf` | `/opt/pros/nginx/conf.d/locations/ocopion-api.conf` |
| `ocopion-speedtest-proxy.conf` | `/opt/pros/nginx/conf.d/snippets/ocopion-speedtest-proxy.conf` |

原有 `location ^~ /ocopion/api/` 会跳过同级正则匹配，因此使用四条精确匹配规则覆盖测速接口，其他 API 沿用原配置。WebSocket 显式转发 Upgrade 和 Connection；测速接口关闭请求和响应缓冲、压缩、缓存及上游重试，上传最大 8 MiB。

线上全局 `http/core.conf` 目前配置 `set_real_ip_from 0.0.0.0/0`，会信任任何来源的转发头。这两份候选配置使用 `$realip_remote_addr` 将原始 TCP 对端地址传给测速后端，避免测速出口 IP 被自带的 `X-Forwarded-For` 伪造。这适用于当前直接连接此 Nginx 的部署；若接入 CDN，需重新指定可信代理链，不能直接使用此地址作为终端用户 IP。

候选文件已放入容器内完整配置的临时副本，执行 `openresty -t -c <副本>/nginx.conf` 检查通过，临时副本已删除。未替换线上配置，也未重载服务。

## 应用命令

以下命令从本仓库根目录执行。它们会修改并重载线上配置，检查内容后再运行。

```sh
# 备份现有业务代理文件。
ssh qyhever 'cp -p /opt/pros/nginx/conf.d/locations/ocopion-api.conf /opt/pros/nginx/conf.d/locations/ocopion-api.conf.bak-$(date +%Y%m%d-%H%M%S)'

# 先安装共享片段，再安装引用它的业务配置。
scp ocopion-backend/docs/nginx/ocopion-speedtest-proxy.conf qyhever:/opt/pros/nginx/conf.d/snippets/ocopion-speedtest-proxy.conf
scp ocopion-backend/docs/nginx/ocopion-api.conf qyhever:/opt/pros/nginx/conf.d/locations/ocopion-api.conf

# 只有语法检查通过，才执行平滑重载。
ssh qyhever 'docker exec app openresty -t && docker exec app openresty -s reload'
```

不要把备份文件命名为另一个 `.conf` 文件后放进 `servers` 等通配符加载目录。

## 后端配置

线上后端工作目录为 `/opt/apps/ocopion-backend`，配置文件在该目录根部。部署新版后，在 `/opt/apps/ocopion-backend/prod.local.yml` 合并以下字段，然后按原有方式重启后端：

```yaml
server:
  trusted_proxies:
    - "127.0.0.1"
    - "::1"
speedtest:
  node_name: "qyhever VPS"
  allowed_origins:
    - "https://ocopion.qyhever.com"
```

后端收到的是 Nginx 转发的 HTTP 请求，显式配置 HTTPS Origin 可以避免 WebSocket 因协议不一致被拒绝。可信代理配置保证页面取得实际客户端地址，而不是 `127.0.0.1`。

前端测速页面和后端测速模块都需要部署新版，单独修改 Nginx 不会新增 API。部署后，未登录访问 `/ocopion/api/info` 应返回 HTTP 401，登录后返回节点信息；浏览器回显连接应返回 101，下载时应边接收边更新速率。还应验证上传确认字节数、停止与重测。

参考：[Nginx location 优先级](https://nginx.org/en/docs/http/ngx_http_core_module.html#location)、[WebSocket 代理](https://nginx.org/en/docs/http/websocket.html)、[原始客户端地址变量](https://nginx.org/en/docs/http/ngx_http_realip_module.html#embedded_variables)。
