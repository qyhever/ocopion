package txtvideo

import (
	"context"
	"crypto/rand"
	"fmt"
	"io"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"ocopion/internal/config"
)

type CommandRunner interface {
	LookPath(string) (string, error)
	Run(context.Context, string, ...string) ([]byte, error)
}

type execRunner struct{}

func (execRunner) LookPath(name string) (string, error) { return exec.LookPath(name) }

// Bound output retained from tools: ffmpeg progress must not grow with video duration.
type limitedBuffer struct{ data []byte }

func (b *limitedBuffer) Write(p []byte) (int, error) {
	n := len(p)
	if n >= 8192 {
		b.data = append(b.data[:0], p[n-8192:]...)
	} else {
		if len(b.data)+n > 8192 {
			b.data = b.data[len(b.data)+n-8192:]
		}
		b.data = append(b.data, p...)
	}
	return n, nil
}
func (execRunner) Run(ctx context.Context, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	var out limitedBuffer
	cmd.Stdout, cmd.Stderr = &out, &out
	err := cmd.Run()
	return out.data, err
}

type FileRepository struct {
	uploadDir, videoDir string
	runner              CommandRunner
}

func NewFileRepository(cfg config.AttachConfig) *FileRepository {
	return &FileRepository{uploadDir: cfg.UploadDirPath, videoDir: cfg.TxtVideoDirPath, runner: execRunner{}}
}
func newID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(err)
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[:4], b[4:6], b[6:8], b[8:10], b[10:])
}

var videoName = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}\.mp4$`)

func (r *FileRepository) CheckDependencies() error {
	if strings.TrimSpace(r.uploadDir) == "" || strings.TrimSpace(r.videoDir) == "" {
		return fmt.Errorf("请配置 attach.upload_dir_path 和 attach.txt_video_dir_path")
	}
	for _, name := range []string{"edge-tts", "ffmpeg", "ffprobe"} {
		if _, err := r.runner.LookPath(name); err != nil {
			return fmt.Errorf("缺少视频生成依赖 %s，请安装并加入服务 PATH", name)
		}
	}
	return nil
}
func (r *FileRepository) Prepare(cover io.Reader) (dir string, err error) {
	if err = os.MkdirAll(r.uploadDir, 0755); err != nil {
		return "", fmt.Errorf("创建上传目录失败: %w", err)
	}
	dir, err = os.MkdirTemp(r.uploadDir, ".txt-video-")
	if err != nil {
		return "", err
	}
	defer func() {
		if err != nil {
			r.Cleanup(dir)
		}
	}()
	f, err := os.OpenFile(filepath.Join(dir, "cover"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return dir, err
	}
	_, err = io.Copy(f, cover)
	closeErr := f.Close()
	if err == nil {
		err = closeErr
	}
	return dir, err
}
func (r *FileRepository) Cleanup(dir string) { _ = os.RemoveAll(dir) }
func (r *FileRepository) command(ctx context.Context, name string, args ...string) ([]byte, error) {
	out, err := r.runner.Run(ctx, name, args...)
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if err != nil {
		return nil, fmt.Errorf("%s 执行失败: %w（%s）", name, err, strings.TrimSpace(string(out)))
	}
	return out, nil
}
func (r *FileRepository) Generate(ctx context.Context, dir, text, speed, filename string) error {
	if !videoName.MatchString(filename) {
		return fmt.Errorf("视频文件名不合法")
	}
	textPath, source, audio := filepath.Join(dir, "text.txt"), filepath.Join(dir, "speech.mp3"), filepath.Join(dir, "audio.m4a")
	if err := os.WriteFile(textPath, []byte(text), 0600); err != nil {
		return err
	}
	if _, err := r.command(ctx, "edge-tts", "--voice", "zh-CN-XiaoxiaoNeural", "--file", textPath, "--write-media", source); err != nil {
		return err
	}
	if _, err := r.command(ctx, "ffmpeg", "-nostdin", "-y", "-i", source, "-vn", "-af", "atempo="+speed, "-c:a", "aac", "-b:a", "192k", "-ar", "44100", "-ac", "2", audio); err != nil {
		return err
	}
	out, err := r.command(ctx, "ffprobe", "-v", "error", "-select_streams", "a:0", "-show_entries", "stream=duration", "-of", "default=noprint_wrappers=1:nokey=1", audio)
	if err != nil {
		return err
	}
	duration, err := strconv.ParseFloat(strings.TrimSpace(string(out)), 64)
	if err != nil || duration <= 0 || math.IsNaN(duration) || math.IsInf(duration, 0) {
		return fmt.Errorf("无法读取有效音轨时长")
	}
	if err := os.MkdirAll(r.videoDir, 0755); err != nil {
		return err
	}
	staging, err := os.MkdirTemp(r.videoDir, ".txt-video-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(staging)
	output := filepath.Join(staging, "video.mp4")
	// Extract and loop a single cover frame.
	frame := filepath.Join(dir, "frame.png")
	if _, err := r.command(ctx, "ffmpeg", "-nostdin", "-y", "-i", filepath.Join(dir, "cover"), "-frames:v", "1", frame); err != nil {
		return err
	}
	_, err = r.command(ctx, "ffmpeg", "-nostdin", "-y", "-loop", "1", "-framerate", "30", "-i", frame, "-i", audio,
		"-map", "0:v:0", "-map", "1:a:0", "-vf", "scale=1920:1080:force_original_aspect_ratio=decrease,pad=1920:1080:(ow-iw)/2:(oh-ih)/2,setsar=1,format=yuv420p",
		"-c:v", "libx264", "-preset", "medium", "-tune", "stillimage", "-crf", "18", "-r", "30", "-pix_fmt", "yuv420p",
		"-color_range", "tv", "-colorspace", "bt709", "-color_primaries", "bt709", "-color_trc", "bt709", "-c:a", "copy", "-movflags", "+faststart", "-t", strconv.FormatFloat(duration, 'f', 6, 64), output)
	if err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	info, err := os.Stat(output)
	if err != nil {
		return err
	}
	if info.Size() == 0 {
		return fmt.Errorf("视频成品为空")
	}
	return os.Rename(output, filepath.Join(r.videoDir, filename))
}
func (r *FileRepository) Open(filename string) (*os.File, error) {
	if !videoName.MatchString(filename) {
		return nil, os.ErrNotExist
	}
	path := filepath.Join(r.videoDir, filename)
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, os.ErrNotExist
	}
	return os.Open(path)
}
