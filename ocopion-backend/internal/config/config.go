package config

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/viper"
)

// Config 应用配置结构体
type Config struct {
	CFSTBinPath      string         `mapstructure:"cfst_bin_path"`
	OutputFastIPPath string         `mapstructure:"output_fast_ip_path"`
	CFSTURL          string         `mapstructure:"cfst_url"`
	Mode             string         `mapstructure:"mode"`
	Server           ServerConfig   `mapstructure:"server"`
	File             FileConfig     `mapstructure:"file"`
	Logger           LoggerConfig   `mapstructure:"logger"`
	Database         DatabaseConfig `mapstructure:"database"`
	JWT              JWTConfig      `mapstructure:"jwt"`
	Auth             AuthConfig     `mapstructure:"auth"`
	Postal           PostalConfig   `mapstructure:"postal"`
}

// ServerConfig 服务器配置
type ServerConfig struct {
	Port int `mapstructure:"port"`
}

type FileConfig struct {
	UploadViewBaseURL string `mapstructure:"upload_view_base_url"`
	UploadDirPath     string `mapstructure:"upload_dir_path"`
}

// LoggerConfig 日志配置
type LoggerConfig struct {
	Level      string `mapstructure:"level"`
	Filename   string `mapstructure:"filename"`
	MaxSize    int    `mapstructure:"max_size"`
	MaxAge     int    `mapstructure:"max_age"`
	MaxBackups int    `mapstructure:"max_backups"`
}

// DatabaseConfig 数据库配置
type DatabaseConfig struct {
	MySQL MySQLConfig `mapstructure:"mysql"`
}

// MySQLConfig MySQL数据库配置
type MySQLConfig struct {
	Addr     string `mapstructure:"addr"`
	User     string `mapstructure:"user"`
	Password string `mapstructure:"password"`
	DBName   string `mapstructure:"db_name"`
}

// JWTConfig JWT配置
type JWTConfig struct {
	Secret           string `mapstructure:"secret"`
	AccessExpiresIn  string `mapstructure:"access_expires_in"`
	RefreshExpiresIn string `mapstructure:"refresh_expires_in"`
}

// AuthConfig 认证相关配置
// 包括访问白名单（支持 "METHOD:/path" 或仅路径形式）。
type AuthConfig struct {
	Whitelist []string `mapstructure:"whitelist"`
}

// Postal配置
type PostalConfig struct {
	SmtpServer string `mapstructure:"smtp_server"`
	SmtpPort   string `mapstructure:"smtp_port"`
	FromEmail  string `mapstructure:"from_email"`
	FromPass   string `mapstructure:"from_pass"`
	FromName   string `mapstructure:"from_name"`
}

// GlobalConfig 全局配置实例
var GlobalConfig *Config

// Init 初始化配置
func Init() error {
	// 获取环境变量，默认为 dev
	env := os.Getenv("OCOPION_ENV")
	if env == "" {
		env = "dev"
	}

	// 验证环境变量值，只允许 dev、test、prod
	validEnvs := map[string]bool{
		"dev":  true,
		"test": true,
		"prod": true,
	}
	if !validEnvs[env] {
		return fmt.Errorf("无效的环境变量 OCOPION_ENV=%s，只允许: dev, test, prod", env)
	}

	// 获取当前工作目录
	workDir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("获取工作目录失败: %w", err)
	}

	loader := viper.New()
	addConfigPaths(loader, workDir)
	bindEnvVars(loader)

	if err := readRequiredConfig(loader, "app"); err != nil {
		return fmt.Errorf("读取配置文件 app.yml 失败: %w", err)
	}

	if err := mergeRequiredConfig(loader, env); err != nil {
		log.Printf("未找到配置文件 %s.yml，跳过: %v", env, err)
	}

	if merged, err := mergeOptionalConfig(loader, env+".local"); err != nil {
		return fmt.Errorf("读取配置文件 %s.local.yml 失败: %w", env, err)
	} else if merged {
		log.Printf("已合并本地配置文件: %s.local.yml", env)
	}

	// 将配置解析到结构体
	GlobalConfig = &Config{}
	if err := loader.Unmarshal(GlobalConfig); err != nil {
		return fmt.Errorf("解析配置文件失败: %w", err)
	}

	log.Printf("当前环境: %s", env)
	log.Printf("配置文件加载成功: app.yml -> %s.yml", env)
	return nil
}

func addConfigPaths(loader *viper.Viper, workDir string) {
	loader.AddConfigPath(filepath.Join(workDir, "internal/config"))
	loader.AddConfigPath("./internal/config")
	loader.AddConfigPath(".")
}

func bindEnvVars(loader *viper.Viper) {
	loader.SetEnvPrefix("OCOPION")
	loader.AutomaticEnv()
	loader.SetDefault("output_fast_ip_path", "./fast-ip.txt")
	loader.BindEnv("cfst_bin_path", "OCOPION_CFST_BIN_PATH")
	loader.BindEnv("output_fast_ip_path", "OCOPION_OUTPUT_FAST_IP_PATH")
	loader.BindEnv("cfst_url", "OCOPION_CFST_URL")

	loader.BindEnv("mode", "OCOPION_MODE")

	loader.BindEnv("server.port", "OCOPION_SERVER_PORT")

	loader.BindEnv("file.upload_view_base_url", "OCOPION_FILE_UPLOAD_VIEW_BASE_URL")
	loader.BindEnv("file.upload_dir_path", "OCOPION_FILE_UPLOAD_DIR_PATH")

	loader.BindEnv("logger.level", "OCOPION_LOGGER_LEVEL")
	loader.BindEnv("logger.filename", "OCOPION_LOGGER_FILENAME")
	loader.BindEnv("logger.max_size", "OCOPION_LOGGER_MAX_SIZE")
	loader.BindEnv("logger.max_age", "OCOPION_LOGGER_MAX_AGE")
	loader.BindEnv("logger.max_backups", "OCOPION_LOGGER_MAX_BACKUPS")

	loader.BindEnv("database.mysql.addr", "OCOPION_DATABASE_MYSQL_ADDR")
	loader.BindEnv("database.mysql.user", "OCOPION_DATABASE_MYSQL_USER")
	loader.BindEnv("database.mysql.password", "OCOPION_DATABASE_MYSQL_PASSWORD")
	loader.BindEnv("database.mysql.db_name", "OCOPION_DATABASE_MYSQL_DB_NAME")

	viper.BindEnv("jwt.secret", "OCOPION_JWT_SECRET")
	viper.BindEnv("jwt.access_expires_in", "OCOPION_JWT_ACCESS_EXPIRES_IN")
	viper.BindEnv("jwt.refresh_expires_in", "OCOPION_JWT_REFRESH_EXPIRES_IN")

}

func readRequiredConfig(loader *viper.Viper, name string) error {
	loader.SetConfigName(name)
	loader.SetConfigType("yml")
	return loader.ReadInConfig()
}

func mergeRequiredConfig(loader *viper.Viper, name string) error {
	loader.SetConfigName(name)
	loader.SetConfigType("yml")
	return loader.MergeInConfig()
}

func mergeOptionalConfig(loader *viper.Viper, name string) (bool, error) {
	loader.SetConfigName(name)
	loader.SetConfigType("yml")
	if err := loader.MergeInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); ok {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// GetConfig 获取全局配置
func GetConfig() *Config {
	return GlobalConfig
}

// GetServerAddr 获取服务器地址
func GetServerAddr() string {
	if GlobalConfig == nil {
		return ":8301" // 默认端口
	}
	return fmt.Sprintf(":%d", GlobalConfig.Server.Port)
}

// GetMySQLDSN 获取MySQL连接字符串
func GetMySQLDSN() string {
	if GlobalConfig == nil {
		return ""
	}
	return BuildMySQLDSN(GlobalConfig)
}

// BuildMySQLDSN 从指定配置生成 MySQL 连接字符串
func BuildMySQLDSN(cfg *Config) string {
	if cfg == nil {
		return ""
	}

	mysql := cfg.Database.MySQL
	if strings.TrimSpace(mysql.Addr) == "" ||
		strings.TrimSpace(mysql.User) == "" ||
		strings.TrimSpace(mysql.DBName) == "" {
		return ""
	}
	return fmt.Sprintf("%s:%s@tcp(%s)/%s?charset=utf8mb4&parseTime=True&loc=Local",
		mysql.User, mysql.Password, mysql.Addr, mysql.DBName)
}

// GetEnv 获取当前环境（dev/test/prod）
func GetEnv() string {
	env := os.Getenv("OCOPION_ENV")
	if env == "" {
		return "dev"
	}
	return env
}

// IsProduction 判断是否为生产环境
func IsProduction() bool {
	return GetEnv() == "prod"
}

// IsDevelopment 判断是否为开发环境
func IsDevelopment() bool {
	return GetEnv() == "dev"
}

// IsTest 判断是否为测试环境
func IsTest() bool {
	return GetEnv() == "test"
}
