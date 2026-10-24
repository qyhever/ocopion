# ocopion

Go + Gin 后端服务。

## Cloudflare IP 最低延迟优选

部署前安装 [CloudflareSpeedTest](https://github.com/XIU2/CloudflareSpeedTest)，赋予 `cfst` 执行权限，并将官方 IPv4 网段文件 `ip.txt` 放在同一目录。服务启动时不检查 cfst，配置缺失不会影响其他接口。

在 `internal/config/app.yml` 或环境对应的本地配置文件中设置：

```yaml
cfst_bin_path: "/opt/cfst/cfst"
output_fast_ip_path: "./fast-ip.txt"
clash_template_path: "./internal/data/clash-template.yaml"
output_clash_path: "./public/clash-kdd.yaml"
cfst_url: ""
upload_config_file_url: "https://bluespot.6688988.xyz/bluespot/api/attach/uploadConfigFile"
upload_config_file_secret: "" # 使用环境变量或本地配置提供实际密钥
```

环境变量优先于 YAML：

| 配置项 | 环境变量 | 默认值 |
| --- | --- | --- |
| `cfst_bin_path` | `OCOPION_CFST_BIN_PATH` | 空，任务调用时报告未配置 |
| `output_fast_ip_path` | `OCOPION_OUTPUT_FAST_IP_PATH` | `./fast-ip.txt` |
| `clash_template_path` | `OCOPION_CLASH_TEMPLATE_PATH` | `./internal/data/clash-template.yaml` |
| `output_clash_path` | `OCOPION_OUTPUT_CLASH_PATH` | `./public/clash-kdd.yaml` |
| `cfst_url` | `OCOPION_CFST_URL` | 空，沿用 cfst 默认值 |
| `upload_config_file_url` | `OCOPION_UPLOAD_CONFIG_FILE_URL` | 无内置默认值，读取 YAML 或环境变量配置 |
| `upload_config_file_secret` | `OCOPION_UPLOAD_CONFIG_FILE_SECRET` | 空，上传时必须提供 |

相对路径以**后端进程工作目录**为基准解析为绝对路径。服务账户须能执行 cfst、读取 `ip.txt` 和 Clash 模板、写入系统临时目录和结果目录。配置 `cfst_url` 时会传递 `-url`；当前使用 TCP 延迟测速，该地址不影响优选结果。

启动任务并查询进度：

```sh
# 无请求体，立即返回 running；运行期间再次调用返回同一任务。
curl -X POST http://localhost:8301/api/cloudflare/fast-ip

# 查询最近一次任务，任务失败仍返回 code=200。
curl http://localhost:8301/api/cloudflare/fast-ip

# 任务成功后下载 Clash 配置（默认输出路径）。
curl -o clash-kdd.yaml http://localhost:8301/public/clash-kdd.yaml
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
-dd -p 16 -f <cfst目录>/ip.txt -o <任务临时CSV>
```

后端读取官方 CSV，过滤非法 IPv4、无有效响应、无效延迟和丢包率，按平均延迟升序排列；延迟相同则按丢包率、IP 数值升序排列。去重后取前 16 个，排序依据见 [官方 CSV 实现](https://github.com/XIU2/CloudflareSpeedTest/blob/master/utils/csv.go)。不足 16 个结果、进程失败、超时、CSV 异常、模板异常或保存失败均标记任务失败，具体原因通过 `error` 返回。

选出 16 个 IP 后，每次重新读取模板并生成 Clash 配置，保留全部原节点。模板必须包含 `🇺🇸 US-CF-Vless-WS`、`🇰🇷 KR-CF-Vless-WS`，分别完整复制为 `🇺🇸 US-CFIP-Vless-WS1`～`🇺🇸 US-CFIP-Vless-WS16` 和 `🇰🇷 KR-CFIP-Vless-WS1`～`🇰🇷 KR-CFIP-Vless-WS16`。仅修改 `name` 和 `server`，同编号使用同一个优选 IP，编号顺序与任务 `ips` 一致；UUID、TLS servername、WebSocket Host 等字段保持原值。

`🚀 节点选择`、`♻️ 自动选择` 加入全部 32 个新节点；`♻️ 自动选择-US`、`♻️ 自动选择-KR` 分别加入对应的 16 个节点，各组原成员保留。DNS、规则、其他配置和注释通过 YAML 节点树保留。模板缺失、格式错误、原节点或目标策略组缺失、名称冲突时任务失败，两个输出文件均不更新。重复调用从模板重新生成，不会累计追加节点。

输出路径不得与模板、IP 文本输出或 cfst 输入 `ip.txt` 冲突。完成模板读取和配置生成后，依次保存 IP 文本、Clash 配置；两个文件全部保存成功后，再执行配置上传；上传成功才标记 `succeeded`。输出自动创建目录，分别在目标目录写临时文件，再通过重命名替换；保存失败保留对应文件的旧内容，临时文件自动清理。两个文件独立替换，不提供跨文件事务：**Clash 保存失败时，IP 文本可能已经更新**。IP 文本严格包含 16 行 IPv4 地址，无表头，末尾有换行。

上传使用 `POST multipart/form-data`，`file` 为实际保存的 Clash 文件，文件名取 `output_clash_path` 的文件名，`secret` 为配置的上传密钥。请求超时为 1 分钟，同时受后台任务整体 10 分钟超时限制。当前按 HTTP 2xx 判断上传成功，不解析未约定的业务响应格式；非 2xx、网络错误或超时会使任务变为 `failed`，本地已保存的两个文件保留。上传不自动重试，不跟随重定向，任务错误不包含远端响应正文。上传地址和密钥都为空时跳过上传，仅配置其中一项时任务失败。

通过环境变量设置密钥后启动服务，例如：

```sh
export OCOPION_UPLOAD_CONFIG_FILE_URL="https://bluespot.6688988.xyz/bluespot/api/attach/uploadConfigFile"
export OCOPION_UPLOAD_CONFIG_FILE_SECRET="<实际密钥>"
```

默认配置可通过 `/public/clash-kdd.yaml` 下载。修改输出位置后，只有位于现有静态目录内的文件可通过 `/public` 下载。部署脚本会把模板上传至 `internal/data/`，部署后须确保服务工作目录对应后端目录。

默认单实例部署，任务状态保存在内存，重启恢复 `idle`，结果文件保留。结果代表 cfst 本次 IPv4 抽样中延迟最低的 16 个地址，不穷举全部 Cloudflare IP。本功能不自动下载 cfst，不添加定时任务或鉴权体系。

接口定义见 `docs/swagger.yaml`，请求示例见 `rest/*.http`。

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
