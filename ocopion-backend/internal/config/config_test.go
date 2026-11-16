package config

import (
	"github.com/spf13/viper"
	"strings"
	"testing"
)

func TestMockUserConfig(t *testing.T) {
	for _, tc := range []struct {
		name, yaml string
		want       MockUserConfig
	}{
		{"absent", "auth:\n  whitelist: []\n", MockUserConfig{}},
		{"disabled", "auth:\n  mockuser:\n    email: mock@example.com\n", MockUserConfig{Email: "mock@example.com"}},
		{"enabled", "auth:\n  mockuser:\n    enabled: true\n    email: mock@example.com\n    username: mock-admin\n    password: mock-secret\n",
			MockUserConfig{Enabled: true, Email: "mock@example.com", Username: "mock-admin", Password: "mock-secret"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			loader := viper.New()
			bindEnvVars(loader)
			loader.SetConfigType("yaml")
			if err := loader.ReadConfig(strings.NewReader(tc.yaml)); err != nil {
				t.Fatal(err)
			}
			var cfg Config
			if err := loader.Unmarshal(&cfg); err != nil {
				t.Fatal(err)
			}
			if cfg.Auth.MockUser != tc.want {
				t.Fatal("mock user config did not match YAML")
			}
		})
	}
}

func TestTelegramEnvOverridesYAML(t *testing.T) {
	for _, env := range []bool{false, true} {
		t.Run(map[bool]string{false: "yaml", true: "env"}[env], func(t *testing.T) {
			for _, key := range []string{"BOT_TOKEN", "CHAT_ID", "MIRROR_BASE_URL"} {
				value := ""
				if env {
					value = "env-" + key
				}
				t.Setenv("OCOPION_TG_"+key, value)
			}
			loader := viper.New()
			bindEnvVars(loader)
			loader.SetConfigType("yaml")
			if err := loader.ReadConfig(strings.NewReader("tg:\n  bot_token: yaml-token\n  chat_id: yaml-chat\n  mirror_base_url: https://yaml.example\n")); err != nil {
				t.Fatal(err)
			}
			var cfg Config
			if err := loader.Unmarshal(&cfg); err != nil {
				t.Fatal(err)
			}
			want := TGConfig{BotToken: "yaml-token", ChatID: "yaml-chat", MirrorBaseURL: "https://yaml.example"}
			if env {
				want = TGConfig{BotToken: "env-BOT_TOKEN", ChatID: "env-CHAT_ID", MirrorBaseURL: "env-MIRROR_BASE_URL"}
			}
			if cfg.TG != want {
				t.Fatalf("TG=%+v", cfg.TG)
			}
		})
	}
}

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

func TestSpeedtestConfigEnv(t *testing.T) {
	t.Setenv("OCOPION_SPEEDTEST_NODE_NAME", "env-node")
	t.Setenv("OCOPION_SPEEDTEST_ALLOWED_ORIGINS", "https://one.test,https://two.test")
	t.Setenv("OCOPION_SERVER_TRUSTED_PROXIES", "127.0.0.1,::1")
	loader := viper.New()
	bindEnvVars(loader)
	var cfg Config
	if err := loader.Unmarshal(&cfg); err != nil {
		t.Fatal(err)
	}
	if cfg.Speedtest.NodeName != "env-node" || len(cfg.Speedtest.AllowedOrigins) != 2 || len(cfg.Server.TrustedProxies) != 2 {
		t.Fatalf("bad speedtest config: %+v, %+v", cfg.Speedtest, cfg.Server)
	}
}
