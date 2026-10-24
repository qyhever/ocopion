package config

import (
	"github.com/spf13/viper"
	"strings"
	"testing"
)

func TestCloudflareEnvOverridesYAML(t *testing.T) {
	t.Setenv("OCOPION_CFST_BIN_PATH", "./env-cfst")
	t.Setenv("OCOPION_OUTPUT_FAST_IP_PATH", "./env-fast-ip.txt")
	t.Setenv("OCOPION_CFST_URL", "https://example.com/env")
	t.Setenv("OCOPION_CLASH_TEMPLATE_PATH", "./env-template.yaml")
	t.Setenv("OCOPION_OUTPUT_CLASH_PATH", "./env-clash.yaml")
	t.Setenv("OCOPION_UPLOAD_CONFIG_FILE_URL", "https://example.com/upload")
	t.Setenv("OCOPION_UPLOAD_CONFIG_FILE_SECRET", "test-secret")
	loader := viper.New()
	loader.SetConfigFile("app.yml")
	bindEnvVars(loader)
	if err := loader.ReadInConfig(); err != nil {
		t.Fatal(err)
	}
	var cfg Config
	if err := loader.Unmarshal(&cfg); err != nil {
		t.Fatal(err)
	}
	if cfg.UploadConfigFileURL != "https://example.com/upload" || cfg.UploadConfigFileSecret != "test-secret" || cfg.ClashTemplatePath != "./env-template.yaml" || cfg.OutputClashPath != "./env-clash.yaml" || cfg.CFSTBinPath != "./env-cfst" || cfg.OutputFastIPPath != "./env-fast-ip.txt" || cfg.CFSTURL != "https://example.com/env" {
		t.Fatalf("env override failed")
	}
}
func TestCloudflareDefaultOutput(t *testing.T) {
	t.Setenv("OCOPION_OUTPUT_FAST_IP_PATH", "")
	t.Setenv("OCOPION_CLASH_TEMPLATE_PATH", "")
	t.Setenv("OCOPION_OUTPUT_CLASH_PATH", "")
	loader := viper.New()
	bindEnvVars(loader)
	var cfg Config
	if err := loader.Unmarshal(&cfg); err != nil {
		t.Fatal(err)
	}
	if cfg.ClashTemplatePath != "./internal/data/clash-template.yaml" || cfg.OutputClashPath != "./public/clash-kdd.yaml" || cfg.OutputFastIPPath != "./fast-ip.txt" {
		t.Fatalf("bad default %q", cfg.OutputFastIPPath)
	}
}

func TestUploadConfigFromYAML(t *testing.T) {
	t.Setenv("OCOPION_UPLOAD_CONFIG_FILE_URL", "")
	t.Setenv("OCOPION_UPLOAD_CONFIG_FILE_SECRET", "")
	loader := viper.New()
	bindEnvVars(loader)
	loader.SetConfigType("yaml")
	if err := loader.ReadConfig(strings.NewReader("upload_config_file_url: https://example.com/config\nupload_config_file_secret: yaml-secret\n")); err != nil {
		t.Fatal(err)
	}
	var cfg Config
	if err := loader.Unmarshal(&cfg); err != nil {
		t.Fatal(err)
	}
	if cfg.UploadConfigFileURL != "https://example.com/config" || cfg.UploadConfigFileSecret != "yaml-secret" {
		t.Fatal("upload YAML config not loaded")
	}
}

func TestJWTEnvOverridesYAML(t *testing.T) {
	t.Setenv("OCOPION_JWT_SECRET", "env-secret")
	t.Setenv("OCOPION_JWT_ACCESS_EXPIRES_IN", "15m")
	t.Setenv("OCOPION_JWT_REFRESH_EXPIRES_IN", "168h")
	loader := viper.New()
	bindEnvVars(loader)
	loader.SetConfigType("yaml")
	if err := loader.ReadConfig(strings.NewReader("jwt:\n  secret: yaml-secret\n  access_expires_in: 2h\n  refresh_expires_in: 72h\n")); err != nil {
		t.Fatal(err)
	}
	var cfg Config
	if err := loader.Unmarshal(&cfg); err != nil {
		t.Fatal(err)
	}
	if cfg.JWT.Secret != "env-secret" || cfg.JWT.AccessExpiresIn != "15m" || cfg.JWT.RefreshExpiresIn != "168h" {
		t.Fatalf("JWT env override failed: %#v", cfg.JWT)
	}
}
