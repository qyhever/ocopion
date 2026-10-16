package cloudflare

import (
	"context"
	"encoding/csv"
	"fmt"
	"io"
	"math"
	"net/netip"
	"ocopion/internal/config"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const outputLimit = 4096

type FileRepository struct {
	binPath    string
	outputPath string
	url        string
	pathError  error
}

func NewRepository(cfg *config.Config) *FileRepository {
	r := &FileRepository{outputPath: "./fast-ip.txt"}
	if cfg != nil {
		r.binPath = cfg.CFSTBinPath
		r.url = cfg.CFSTURL
		if cfg.OutputFastIPPath != "" {
			r.outputPath = cfg.OutputFastIPPath
		}
	}
	if r.binPath != "" {
		r.binPath, r.pathError = filepath.Abs(r.binPath)
	}
	if r.pathError == nil {
		r.outputPath, r.pathError = filepath.Abs(r.outputPath)
	}
	return r
}

// limitedOutput 限制内存占用，但仍消费完整 stdout/stderr，避免阻塞子进程。
type limitedOutput struct{ data []byte }

func (b *limitedOutput) Write(p []byte) (int, error) {
	n := len(p)
	if available := outputLimit - len(b.data); available > 0 {
		if len(p) > available {
			p = p[:available]
		}
		b.data = append(b.data, p...)
	}
	return n, nil
}

func (r *FileRepository) Measure(ctx context.Context) ([]Candidate, error) {
	if r.pathError != nil {
		return nil, fmt.Errorf("解析路径失败: %w", r.pathError)
	}
	if r.binPath == "" {
		return nil, fmt.Errorf("未配置 cfst_bin_path（OCOPION_CFST_BIN_PATH）")
	}
	dir := filepath.Dir(r.binPath)
	input := filepath.Join(dir, "ip.txt")
	f, err := os.Open(input)
	if err != nil {
		return nil, fmt.Errorf("读取 cfst IPv4 网段文件失败: %w", err)
	}
	info, err := f.Stat()
	f.Close()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("cfst ip.txt 必须为普通文件")
	}
	// 每个任务使用独立目录，cfst 可以在其中创建 CSV，结束后统一清理。
	tempDir, err := os.MkdirTemp("", "ocopion-cfst-*")
	if err != nil {
		return nil, fmt.Errorf("创建测速临时目录失败: %w", err)
	}
	defer os.RemoveAll(tempDir)
	csvPath := filepath.Join(tempDir, "result.csv")
	args := []string{"-dd", "-p", "0", "-f", input, "-o", csvPath}
	if r.url != "" {
		args = append(args, "-url", r.url)
	}
	cmd := exec.CommandContext(ctx, r.binPath, args...)
	cmd.Dir = dir
	// 子进程异常继承输出管道时，也不无限等待输出关闭。
	cmd.WaitDelay = time.Second
	output := &limitedOutput{}
	cmd.Stdout = output
	cmd.Stderr = output
	err = cmd.Run()
	if ctx.Err() != nil {
		return nil, fmt.Errorf("cfst 测速超时或取消: %w；输出: %s", ctx.Err(), strings.ToValidUTF8(string(output.data), "�"))
	}
	if err != nil {
		return nil, fmt.Errorf("cfst 执行失败: %w；输出: %s", err, strings.ToValidUTF8(string(output.data), "�"))
	}
	file, err := os.Open(csvPath)
	if err != nil {
		return nil, fmt.Errorf("读取 cfst CSV 失败: %w", err)
	}
	defer file.Close()
	return parseCSV(file)
}

func parseCSV(input io.Reader) ([]Candidate, error) {
	reader := csv.NewReader(input)
	header, err := reader.Read()
	if err != nil {
		return nil, fmt.Errorf("读取 CSV 表头失败: %w", err)
	}
	expected := []string{"IP 地址", "已发送", "已接收", "丢包率", "平均延迟"}
	if len(header) < len(expected) {
		return nil, fmt.Errorf("CSV 表头缺少必要字段")
	}
	for i, name := range expected {
		if strings.TrimSpace(strings.TrimPrefix(header[i], "\ufeff")) != name {
			return nil, fmt.Errorf("CSV 表头字段 %d 应为 %s", i+1, name)
		}
	}
	var candidates []Candidate
	for {
		row, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("解析 CSV 失败: %w", err)
		}
		ip, ipErr := netip.ParseAddr(strings.TrimSpace(row[0]))
		sent, sentErr := strconv.Atoi(strings.TrimSpace(row[1]))
		received, receivedErr := strconv.Atoi(strings.TrimSpace(row[2]))
		loss, lossErr := strconv.ParseFloat(strings.TrimSpace(row[3]), 64)
		delay, delayErr := strconv.ParseFloat(strings.TrimSpace(row[4]), 64)
		if ipErr != nil || !ip.Is4() || sentErr != nil || receivedErr != nil || sent <= 0 || received <= 0 || received > sent ||
			lossErr != nil || math.IsNaN(loss) || math.IsInf(loss, 0) || loss < 0 || loss > 1 ||
			delayErr != nil || math.IsNaN(delay) || math.IsInf(delay, 0) || delay < 0 {
			continue
		}
		candidates = append(candidates, Candidate{IP: ip, Delay: delay, Loss: loss})
	}
	return candidates, nil
}

func (r *FileRepository) Save(ips []string) error {
	if r.pathError != nil {
		return r.pathError
	}
	if len(ips) != fastIPCount {
		return fmt.Errorf("保存结果必须为 %d 个 IP", fastIPCount)
	}
	dir := filepath.Dir(r.outputPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("创建结果目录失败: %w", err)
	}
	file, err := os.CreateTemp(dir, ".fast-ip-*")
	if err != nil {
		return fmt.Errorf("创建结果临时文件失败: %w", err)
	}
	defer os.Remove(file.Name())
	defer file.Close()
	if err = file.Chmod(0644); err != nil {
		return err
	}
	if _, err = file.WriteString(strings.Join(ips, "\n") + "\n"); err != nil {
		return fmt.Errorf("写入结果失败: %w", err)
	}
	if err = file.Sync(); err != nil {
		return fmt.Errorf("同步结果失败: %w", err)
	}
	if err = file.Close(); err != nil {
		return fmt.Errorf("关闭结果文件失败: %w", err)
	}
	if err = os.Rename(file.Name(), r.outputPath); err != nil {
		return fmt.Errorf("替换结果文件失败: %w", err)
	}
	return nil
}
