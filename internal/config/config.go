package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/mitchellh/go-homedir"
)

type Config struct {
	UserID string `json:"user_id"`
	APIKey string `json:"api_key"`
}

func Read() (Config, error) {
	home, err := homedir.Dir()
	if err != nil {
		return Config{}, fmt.Errorf("failed to find home directory: %w", err)
	}

	configPath := filepath.Join(home, ".gatorcli.json")

	data, err := os.ReadFile(configPath)
	if errors.Is(err, os.ErrNotExist) {
		return Config{}, nil
	}
	if err != nil {
		return Config{}, fmt.Errorf("failed to read config file: %w", err)
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return Config{}, fmt.Errorf("failed to unmarshal config JSON: %w", err)
	}

	return cfg, nil
}

func (c *Config) Save() error {
	home, err := homedir.Dir()
	if err != nil {
		return fmt.Errorf("failed to find home directory: %w", err)
	}

	configPath := filepath.Join(home, ".gatorcli.json")

	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal config JSON: %w", err)
	}

	if err := os.WriteFile(configPath, data, 0600); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	return nil
}

func (c *Config) SetUser(placeholderUserID string) error {
	c.UserID = placeholderUserID
	return c.Save()
}
