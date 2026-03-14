package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/pelletier/go-toml/v2"
	"github.com/spf13/viper"

	"sem/internal/errs"
)

func Load(configPath string) (Config, error) {
	v := viper.New()
	v.SetConfigFile(configPath)
	v.SetConfigType("toml")
	v.SetEnvPrefix("SEM")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	if err := v.ReadInConfig(); err != nil {
		var notFound viper.ConfigFileNotFoundError
		if errors.As(err, &notFound) || errors.Is(err, os.ErrNotExist) {
			return Config{}, errs.ErrNotInitialized
		}
		return Config{}, fmt.Errorf("read config: %w", err)
	}

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return Config{}, fmt.Errorf("unmarshal config: %w", err)
	}

	if err := cfg.Validate(); err != nil {
		return Config{}, fmt.Errorf("validate config: %w", err)
	}

	return cfg, nil
}

func Save(configPath string, cfg Config) error {
	if err := cfg.Validate(); err != nil {
		return fmt.Errorf("validate config: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		return fmt.Errorf("create config directory: %w", err)
	}

	data, err := toml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}

	if err := os.WriteFile(configPath, data, 0o644); err != nil {
		return fmt.Errorf("write config: %w", err)
	}

	return nil
}

func WriteDefault(configPath, baseDir string, force bool) (Config, error) {
	if _, err := os.Stat(configPath); err == nil && !force {
		return Config{}, errs.ErrAlreadyInitialized
	}

	cfg := Default(baseDir)
	if err := Save(configPath, cfg); err != nil {
		return Config{}, err
	}

	return cfg, nil
}
