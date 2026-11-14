package txtvideo

import (
	"bytes"
	"context"
	"encoding/json"
	"math"
	"os"
	"strconv"
	"testing"
	"time"
)

// Explicit opt-in: contacts Edge TTS over the network and runs real video encoders.
func TestRealVideoGeneration(t *testing.T) {
	if os.Getenv("OCOPION_TEST_TXT_VIDEO") != "1" {
		t.Skip("set OCOPION_TEST_TXT_VIDEO=1 to run real tools")
	}
	r := testRepo(t, execRunner{})
	if err := r.CheckDependencies(); err != nil {
		t.Fatal(err)
	}
	durations := map[string]float64{}
	for _, speed := range []string{"1", "0.5"} {
		dir, err := r.Prepare(bytes.NewReader(pngCover(t)))
		if err != nil {
			t.Fatal(err)
		}
		ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
		name := newID() + ".mp4"
		err = r.Generate(ctx, dir, "你好，欢迎使用文案转视频。", speed, name)
		cancel()
		r.Cleanup(dir)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = os.Stat(dir); !os.IsNotExist(err) {
			t.Fatal("cover retained")
		}
		out, err := r.command(context.Background(), "ffprobe", "-v", "error", "-show_entries", "stream=codec_type,duration,width,height,r_frame_rate", "-of", "json", r.videoDir+"/"+name)
		if err != nil {
			t.Fatal(err)
		}
		var result struct {
			Streams []struct {
				CodecType     string `json:"codec_type"`
				Duration      string `json:"duration"`
				Width, Height int
				Rate          string `json:"r_frame_rate"`
			}
		}
		if err = json.Unmarshal(out, &result); err != nil {
			t.Fatal(err)
		}
		var audio, video float64
		for _, stream := range result.Streams {
			d, _ := strconv.ParseFloat(stream.Duration, 64)
			if stream.CodecType == "audio" {
				audio = d
			} else if stream.CodecType == "video" {
				video = d
				if stream.Width != 1920 || stream.Height != 1080 || stream.Rate != "30/1" {
					t.Fatalf("bad video: %+v", stream)
				}
			}
		}
		if audio <= 0 || video <= 0 || math.Abs(video-audio) > 1.0/30 {
			t.Fatalf("duration mismatch: audio=%f video=%f", audio, video)
		}
		durations[speed] = audio
		t.Logf("speed=%s audio=%.6fs video=%.6fs", speed, audio, video)
	}
	ratio := durations["0.5"] / durations["1"]
	if ratio < 1.9 || ratio > 2.1 {
		t.Fatalf("half speed ratio=%f", ratio)
	}
}
