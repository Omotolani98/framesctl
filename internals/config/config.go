// Package config controls configurations
package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/joho/godotenv"
	"github.com/spf13/viper"
)

const (
	appDirectoryName = ".framesctl"
	configFileName   = "config.yaml"
	databaseFileName = "framesctl.db"
)

type Config struct {
	AppPath       string
	ConfigPath    string
	DatabasePath  string
	APIBaseURL    string
	HTTPAddr      string
	DatabaseURL   string
	PublicBaseURL string

	AWSAccessKeyID  string
	AWSSecretKey    string
	AWSSessionToken string
	AWSRegion       string
	AWSBucket       string

	MaxUploadBytes int64
}

const bytesPerGiB int64 = 1 << 30

func LoadConfig() (Config, error) {
	if err := godotenv.Load(); err != nil &&
		!errors.Is(err, os.ErrNotExist) {
		return Config{}, fmt.Errorf("load .env: %w", err)
	}

	maxUploadGB, err := getPositiveInt64("MAX_UPLOAD_GB", 10)
	if err != nil {
		return Config{}, err
	}

	cfg := Config{
		HTTPAddr:      getEnv("HTTP_ADDR", ":8080"),
		DatabaseURL:   strings.TrimSpace(os.Getenv("DATABASE_URL")),
		PublicBaseURL: getEnv("PUBLIC_BASE_URL", "http://localhost:8080"),

		AWSAccessKeyID:  strings.TrimSpace(os.Getenv("AWS_ACCESS_KEY_ID")),
		AWSSecretKey:    strings.TrimSpace(os.Getenv("AWS_SECRET_KEY")),
		AWSSessionToken: strings.TrimSpace(os.Getenv("AWS_SESSION_TOKEN")),
		AWSRegion:       strings.TrimSpace(os.Getenv("AWS_REGION")),
		AWSBucket:       strings.TrimSpace(os.Getenv("AWS_BUCKET")),

		MaxUploadBytes: maxUploadGB * bytesPerGiB,
	}

	required := map[string]string{
		"AWS_ACCESS_KEY_ID": cfg.AWSAccessKeyID,
		"AWS_SECRET_KEY":    cfg.AWSSecretKey,
		"AWS_REGION":        cfg.AWSRegion,
		"AWS_BUCKET":        cfg.AWSBucket,
	}

	for name, value := range required {
		if value == "" {
			return Config{}, fmt.Errorf(
				"required environment variable %s is empty",
				name,
			)
		}
	}

	return cfg, nil
}

func getEnv(name, fallback string) string {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback
	}

	return value
}

func getPositiveInt64(
	name string,
	fallback int64,
) (int64, error) {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return fallback, nil
	}

	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return 0, fmt.Errorf(
			"%s must be a positive integer: %w",
			name,
			err,
		)
	}

	if value <= 0 {
		return 0, fmt.Errorf("%s must be greater than zero", name)
	}

	return value, nil
}

func Load() (*Config, error) {
	defaultPath, err := defaultAppPath()
	if err != nil {
		return nil, err
	}

	v := viper.New()

	v.SetDefault("path", defaultPath)

	if err := v.BindEnv("path", "FRAMESCTL_PATH"); err != nil {
		return nil, fmt.Errorf("bind FRAMESCTL_PATH: %w", err)
	}

	appPath := filepath.Clean(v.GetString("path"))
	if appPath == "." || appPath == "" {
		return nil, errors.New("FRAMESCTL_PATH cannot be empty")
	}

	if err := os.MkdirAll(appPath, 0o700); err != nil {
		return nil, fmt.Errorf("create application directory: %w", err)
	}

	configPath := filepath.Join(appPath, configFileName)

	fileConfig, err := loadConfigFile(configPath)
	if err != nil {
		return nil, err
	}

	return &Config{
		AppPath:      appPath,
		ConfigPath:   configPath,
		DatabasePath: filepath.Join(appPath, databaseFileName),
		APIBaseURL:   fileConfig.GetString("api.base_url"),
	}, nil
}

func defaultAppPath() (string, error) {
	homeDirectory, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("get home directory: %w", err)
	}

	return filepath.Join(homeDirectory, appDirectoryName), nil
}

func loadConfigFile(path string) (*viper.Viper, error) {
	v := viper.New()

	v.SetConfigFile(path)
	v.SetConfigType("yaml")

	v.SetDefault("api.base_url", "http://localhost:8080")

	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		if err := v.SafeWriteConfigAs(path); err != nil {
			return nil, fmt.Errorf("create config file: %w", err)
		}

		if err := os.Chmod(path, 0o600); err != nil {
			return nil, fmt.Errorf("set config permissions: %w", err)
		}
	} else if err != nil {
		return nil, fmt.Errorf("inspect config file: %w", err)
	}

	if err := v.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("read config file: %w", err)
	}

	return v, nil
}
