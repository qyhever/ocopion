# 文案转视频

登录 PC 端后打开 `/txt-video`，选择封面、填写文案并提交。服务使用晓晓中文女声合成音频，以封面生成 1920×1080、30fps 的 MP4。`0.5` 是半速，时长约为 `1` 的两倍。

## 运行依赖

服务进程的 `PATH` 中必须包含 `edge-tts`、`ffmpeg` 和 `ffprobe`。FFmpeg 需要支持 `libx264`、AAC、PNG/JPEG/WebP 解码。安装示例：

```sh
# macOS：安装 FFmpeg 与 pipx。
brew install ffmpeg pipx
pipx install edge-tts
pipx ensurepath

# Debian / Ubuntu：安装 FFmpeg 与 pipx。
sudo apt-get update
sudo apt-get install -y ffmpeg pipx
pipx install edge-tts
pipx ensurepath
```

以上命令将语音工具装到当前用户的环境中。通过 systemd 部署时，需要为实际运行服务的用户安装，并在服务单元中配置包含该工具目录的 `PATH`。重启服务后生效。

Edge TTS 需要出站网络访问微软语音服务（HTTPS / WebSocket）。安装成功不代表网络可用；网络或上游失败会将任务标记为失败，并在页面显示原因。

## 存储配置

```yaml
attach:
  upload_dir_path: "./public/uploads"
  txt_video_dir_path: "./public/txt-videos"
```

可以通过环境变量覆盖：

```sh
export OCOPION_ATTACH_UPLOAD_DIR_PATH=/var/lib/ocopion/uploads
export OCOPION_ATTACH_TXT_VIDEO_DIR_PATH=/var/lib/ocopion/txt-videos
```

服务用户需要对两个目录具备读、写及创建子目录的权限。目录会自动创建。成品目录可以位于 `/public` 之外；视频接口直接读取配置目录，支持 HTTP Range 与附件下载。生产代理需允许至少 11 MiB 的请求体（例如 Nginx `client_max_body_size 12m`），并将 `/ocopion/api/` 转发到服务 `/api/`。

每个任务在上传目录中创建独立的隐藏临时目录，保存封面、文案和中间音频。成品先在视频目录内的隐藏临时目录生成，再原子重命名为 UUID v4 文件名。正常完成、失败及超时均清理临时文件，失败不发布半成品。进程被强制终止时可能留下隐藏临时目录，可在停止服务后手动清理。

## API

- `POST /api/txt-video/tasks`：需要 JWT，multipart 字段 `text`、`cover`、`speed`。文案去除首尾空白后为 1～10,000 字，封面为 JPEG/PNG/WebP，最大 10 MiB、4000 万像素，`speed` 为 `1` 或 `0.5`，默认 `1`。
- `GET /api/txt-video/tasks/:id`：需要 JWT，仅提交者可查询。返回 `id`、`status`、`error`；成功后附带 `videoUrl`、`downloadUrl`。状态为 `running`、`succeeded` 或 `failed`。不存在、已过期或不属于当前用户的任务统一返回业务码 `404`。
- `GET /api/txt-video/files/:filename`：公开访问，合法文件名为模块生成的 UUID v4 加 `.mp4`。`?download=1` 返回附件，默认用于在线播放。地址通过不可预测的文件名共享，拿到链接的人可以访问。

接口复用统一响应 `{ "code": 200, "message": "操作成功", "data": ... }`，业务失败通过 `code` 判断。返回的视频地址使用前端现有 `/ocopion/api` 代理；直接对接后端时，将该前缀替换为 `/api`。

每个用户最多同时执行一个任务，单个服务进程最多两个，超额直接拒绝。任务独立于提交请求，最长执行 30 分钟；完成记录保留 24 小时。任务状态在内存中，服务重启后丢失。已生成的视频永久保留，不自动删除。多实例部署需让任务提交和查询固定到同一实例，并自行安排视频共享存储。

前端按用户 ID 保存最近任务 ID，刷新或返回页面会恢复查询。暂时网络失败保留记录，可手动恢复查询；任务不存在时清除记录并提示重新生成。

## 编码与验证

音频使用 `zh-CN-XiaoxiaoNeural`，经 `atempo=1` 或 `atempo=0.5` 处理，编码为 AAC 192k、44.1kHz、双声道。读取最终音轨时长后，以封面第一帧循环生成视频，等比缩放、居中补黑边；视频使用 H.264 `libx264`、`medium`、`stillimage`、CRF 18、`yuv420p`、BT.709 色彩标签和 `+faststart`，以实际音轨时长限制成品。仓库中没有原始转换脚本，以上为本模块明确采用的编码参数。

```sh
# 后端目录：运行模块、接口和配置测试。
go test ./internal/module/txtvideo -race
go test ./internal/api -run TestTxtVideoRoutesRequireLogin
go test ./internal/config

# 后端目录：使用真实工具和出站网络生成两个倍速的视频，并验证时长。
OCOPION_TEST_TXT_VIDEO=1 go test ./internal/module/txtvideo -run TestRealVideoGeneration -v -count=1

# 前端目录：验证恢复查询、重复提交限制、资源释放及生产构建。
pnpm test:txt-video
pnpm build
```

真实工具测试的临时文件与视频由测试结束时清理，生产生成的视频不会自动清理。HTTP 调用示例见 `rest/txt-video.http`。
