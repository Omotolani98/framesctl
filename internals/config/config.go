// Package config controls configurations
package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/viper"
)

const (
	appDirectoryName = ".framesctl"
	configFileName   = "config.yaml"
	databaseFileName = "framesctl.db"
)

type Config struct {
	AppPath      string
	ConfigPath   string
	DatabasePath string
	APIBaseURL   string
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
