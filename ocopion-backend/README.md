# ocopion

Go + Gin 后端服务。

## Cloudflare IP 最低延迟优选

部署前安装 [CloudflareSpeedTest](https://github.com/XIU2/CloudflareSpeedTest)，赋予 `cfst` 执行权限，并将官方 IPv4 网段文件 `ip.txt` 放在同一目录。服务启动时不检查 cfst，配置缺失不会影响其他接口。

在 `internal/config/app.yml` 或环境对应的本地配置文件中设置：

```yaml
cfst_bin_path: "/opt/cfst/cfst"
output_fast_ip_path: "./fast-ip.txt"
cfst_url: ""
```

环境变量优先于 YAML：

| 配置项 | 环境变量 | 默认值 |
| --- | --- | --- |
| `cfst_bin_path` | `OCOPION_CFST_BIN_PATH` | 空，任务调用时报告未配置 |
| `output_fast_ip_path` | `OCOPION_OUTPUT_FAST_IP_PATH` | `./fast-ip.txt` |
| `cfst_url` | `OCOPION_CFST_URL` | 空，沿用 cfst 默认值 |

相对路径以**后端进程工作目录**为基准解析为绝对路径。服务账户须能执行 cfst、读取 `ip.txt`、写入系统临时目录和结果目录。配置 `cfst_url` 时会传递 `-url`；当前使用 TCP 延迟测速，该地址不影响优选结果。

启动任务并查询进度：

```sh
# 无请求体，立即返回 running；运行期间再次调用返回同一任务。
curl -X POST http://localhost:8301/api/cloudflare/fast-ip

# 查询最近一次任务，任务失败仍返回 code=200。
curl http://localhost:8301/api/cloudflare/fast-ip
```

首次查询返回：

```json
{
  "code": 200,
  "message": "操作成功",
  "data": {
    "status": "idle",
    "started_at": null,
    "finished_at": null,
    "ips": [],
    "error": ""
  }
}
```

`status` 依次为 `idle`、`running`、`succeeded` 或 `failed`。时间采用 RFC3339 格式；尚未开始或完成时对应字段为 `null`。成功后 `ips` 包含 16 个 IP；失败时 `ips` 为空，`error` 说明原因，进程输出最多保留 4096 字节。终态之后再次 POST 会启动新任务。

后台执行不受 HTTP 请求结束影响，整体超时为 10 分钟，使用以下参数，工作目录为 cfst 所在目录：

```text
-dd -p 0 -f <cfst目录>/ip.txt -o <任务临时CSV>
```

后端读取官方 CSV，过滤非法 IPv4、无有效响应、无效延迟和丢包率，按平均延迟升序排列；延迟相同则按丢包率、IP 数值升序排列。去重后取前 16 个，排序依据见 [官方 CSV 实现](https://github.com/XIU2/CloudflareSpeedTest/blob/master/utils/csv.go)。不足 16 个结果、进程失败、超时、CSV 异常或保存失败均标记任务失败，并保留旧结果文件。

成功时自动创建目标目录，在该目录写临时文件并通过重命名替换目标文件。文件严格包含 16 行 IPv4 地址，无表头，末尾有换行；任务临时文件会清理。

默认单实例部署，任务状态保存在内存，重启恢复 `idle`，结果文件保留。结果代表 cfst 本次 IPv4 抽样中延迟最低的 16 个地址，不穷举全部 Cloudflare IP。本功能不自动下载 cfst，不添加定时任务或鉴权体系。

接口定义见 `docs/swagger.yaml`，请求示例见 `rest/index.http`。

## 验证

在后端目录执行：

```sh
# 使用模拟可执行文件运行测试，无需真实外网测速。
go test ./...

# 检查后台任务的并发访问。
go test -race ./internal/module/cloudflare

# 构建后端。
go build -o /tmp/ocopion ./cmd
```
