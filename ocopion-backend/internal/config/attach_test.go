package config

import (
	"github.com/spf13/viper"
	"testing"
)

func TestAttachEnvironment(t *testing.T) {
	values := map[string]string{"VIEW_ATTACH_BASE_URL": "https://example.test/files", "UPLOAD_DIR_PATH": "./ordinary", "VIEW_LARGE_FILE_BASE_URL": "https://example.test/large", "UPLOAD_LARGE_FILE_PATH": "./large", "CHUNK_DIR_PATH": "./chunks", "CHUNK_DIR_SALT": "secret", "TXT_VIDEO_DIR_PATH": "./videos"}
	for k, v := range values {
		t.Setenv("OCOPION_ATTACH_"+k, v)
	}
	loader := viper.New()
	bindEnvVars(loader)
	var cfg Config
	if e := loader.Unmarshal(&cfg); e != nil {
		t.Fatal(e)
	}
	a := cfg.Attach
	if a.ViewAttachBaseURL != values["VIEW_ATTACH_BASE_URL"] || a.UploadDirPath != "./ordinary" || a.ViewLargeFileBaseURL != values["VIEW_LARGE_FILE_BASE_URL"] || a.UploadLargeFilePath != "./large" || a.ChunkDirPath != "./chunks" || a.ChunkDirSalt != "secret" || a.TxtVideoDirPath != "./videos" {
		t.Fatalf("%+v", a)
	}
	if cfg.File.UploadDirPath != "" {
		t.Fatal("attach leaked into file config")
	}
}
