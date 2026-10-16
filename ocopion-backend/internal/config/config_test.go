package config

import (
	"github.com/spf13/viper"
	"testing"
)

func TestCloudflareEnvOverridesYAML(t *testing.T) {
	t.Setenv("OCOPION_CFST_BIN_PATH", "./env-cfst")
	t.Setenv("OCOPION_OUTPUT_FAST_IP_PATH", "./env-fast-ip.txt")
	t.Setenv("OCOPION_CFST_URL", "https://example.com/env")
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
	if cfg.CFSTBinPath != "./env-cfst" || cfg.OutputFastIPPath != "./env-fast-ip.txt" || cfg.CFSTURL != "https://example.com/env" {
		t.Fatalf("env override failed")
	}
}
func TestCloudflareDefaultOutput(t *testing.T) {
	t.Setenv("OCOPION_OUTPUT_FAST_IP_PATH", "")
	loader := viper.New()
	bindEnvVars(loader)
	var cfg Config
	if err := loader.Unmarshal(&cfg); err != nil {
		t.Fatal(err)
	}
	if cfg.OutputFastIPPath != "./fast-ip.txt" {
		t.Fatalf("bad default %q", cfg.OutputFastIPPath)
	}
}
