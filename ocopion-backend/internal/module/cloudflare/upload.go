package cloudflare

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

// UploadClash 在本地保存完成后上传实际落盘的配置；未配置上传地址和密钥时跳过。
func (r *FileRepository) UploadClash(ctx context.Context) error {
	if r.uploadURL == "" && r.uploadSecret == "" {
		return nil
	}
	if r.uploadURL == "" {
		return fmt.Errorf("未配置 upload_config_file_url（OCOPION_UPLOAD_CONFIG_FILE_URL）")
	}
	if r.uploadSecret == "" {
		return fmt.Errorf("未配置 upload_config_file_secret（OCOPION_UPLOAD_CONFIG_FILE_SECRET）")
	}
	if err := r.validatePaths(); err != nil {
		return err
	}
	file, err := os.Open(r.clashPath)
	if err != nil {
		return fmt.Errorf("读取待上传 Clash 配置失败: %w", err)
	}
	defer file.Close()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", filepath.Base(r.clashPath))
	if err != nil {
		return fmt.Errorf("创建配置上传表单失败: %w", err)
	}
	if _, err := io.Copy(part, file); err != nil {
		return fmt.Errorf("读取待上传 Clash 配置失败: %w", err)
	}
	if err := writer.WriteField("secret", r.uploadSecret); err != nil {
		return fmt.Errorf("写入配置上传表单失败: %w", err)
	}
	if err := writer.Close(); err != nil {
		return fmt.Errorf("完成配置上传表单失败: %w", err)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, r.uploadURL, &body)
	if err != nil {
		return fmt.Errorf("创建配置上传请求失败: %w", err)
	}
	request.Header.Set("Content-Type", writer.FormDataContentType())
	client := &http.Client{
		Timeout: time.Minute,
		// 防止重定向将包含 secret 的表单转发到其他地址。
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse },
	}
	response, err := client.Do(request)
	if err != nil {
		return fmt.Errorf("上传 Clash 配置失败: %w", err)
	}
	defer response.Body.Close()
	// 不将响应正文放入任务错误，避免远端回显密钥。当前以 HTTP 2xx 判断上传成功。
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("上传 Clash 配置失败: HTTP %d", response.StatusCode)
	}
	if _, err := io.Copy(io.Discard, io.LimitReader(response.Body, outputLimit)); err != nil {
		return fmt.Errorf("读取配置上传响应失败: %w", err)
	}
	return nil
}
