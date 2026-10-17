package cloudflare

import (
	"context"
	"encoding/csv"
	"fmt"
	"io"
	"math"
	"net/netip"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"ocopion/internal/config"
)

const outputLimit = 4096

type FileRepository struct {
	uploadURL    string
	uploadSecret string
	templatePath string
	clashPath    string
	binPath      string
	outputPath   string
	url          string
	pathError    error
}

func NewRepository(cfg *config.Config) *FileRepository {
	r := &FileRepository{outputPath: "./fast-ip.txt", templatePath: "./internal/data/clash-template.yaml", clashPath: "./public/clash-kdd.yaml"}
	if cfg != nil {
		r.uploadURL = strings.TrimSpace(cfg.UploadConfigFileURL)
		r.uploadSecret = cfg.UploadConfigFileSecret
		if cfg.ClashTemplatePath != "" {
			r.templatePath = cfg.ClashTemplatePath
		}
		if cfg.OutputClashPath != "" {
			r.clashPath = cfg.OutputClashPath
		}
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
	for _, path := range []*string{&r.templatePath, &r.clashPath} {
		if r.pathError == nil {
			*path, r.pathError = filepath.Abs(*path)
		}
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
	args := []string{"-dd", "-p", strconv.Itoa(fastIPCount), "-f", input, "-o", csvPath}
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
	if err := r.validatePaths(); err != nil {
		return err
	}
	return atomicWrite(r.outputPath, []byte(strings.Join(ips, "\n")+"\n"))
}

func (r *FileRepository) ReadClashTemplate() ([]byte, error) {
	if err := r.validatePaths(); err != nil {
		return nil, err
	}
	data, err := os.ReadFile(r.templatePath)
	if err != nil {
		return nil, fmt.Errorf("读取 Clash 模板失败: %w", err)
	}
	return data, nil
}

func (r *FileRepository) SaveClash(data []byte) error {
	if err := r.validatePaths(); err != nil {
		return err
	}
	if err := atomicWrite(r.clashPath, data); err != nil {
		return fmt.Errorf("保存 Clash 配置失败: %w", err)
	}
	return nil
}

// 同时检查规范路径、符号链接和既有文件的硬链接，避免覆盖输入文件。
func (r *FileRepository) validatePaths() error {
	if r.pathError != nil {
		return r.pathError
	}
	paths := []string{r.templatePath, r.outputPath, r.clashPath}
	if r.binPath != "" {
		paths = append(paths, filepath.Join(filepath.Dir(r.binPath), "ip.txt"))
	}
	for i, a := range paths {
		for _, b := range paths[i+1:] {
			if samePath(a, b) {
				return fmt.Errorf("文件路径冲突: %s 与 %s", a, b)
			}
		}
	}
	return nil
}

func samePath(a, b string) bool {
	if filepath.Clean(a) == filepath.Clean(b) {
		return true
	}
	ai, ae := os.Stat(a)
	bi, be := os.Stat(b)
	if ae == nil && be == nil && os.SameFile(ai, bi) {
		return true
	}
	return resolvedPath(a) == resolvedPath(b)
}

// 输出尚不存在时，也解析已有父目录的符号链接。
func resolvedPath(path string) string {
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		return resolved
	}
	parent := filepath.Dir(path)
	if parent == path {
		return path
	}
	return filepath.Join(resolvedPath(parent), filepath.Base(path))
}

func atomicWrite(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("创建结果目录失败: %w", err)
	}
	file, err := os.CreateTemp(dir, ".ocopion-*")
	if err != nil {
		return fmt.Errorf("创建结果临时文件失败: %w", err)
	}
	defer os.Remove(file.Name())
	defer file.Close()
	if err = file.Chmod(0644); err != nil {
		return err
	}
	if _, err = file.Write(data); err != nil {
		return fmt.Errorf("写入结果失败: %w", err)
	}
	if err = file.Sync(); err != nil {
		return fmt.Errorf("同步结果失败: %w", err)
	}
	if err = file.Close(); err != nil {
		return fmt.Errorf("关闭结果文件失败: %w", err)
	}
	if err = os.Rename(file.Name(), path); err != nil {
		return fmt.Errorf("替换结果文件失败: %w", err)
	}
	return nil
}
