package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/mitchellh/go-homedir"
)

// Config holds the application's persistent configuration data.
// The fields are tagged with `json` to enable reading/writing to a JSON file.
type Config struct {
	UserID string `json:"user_id"`
	// You might add an API key or other configuration here later.
	APIKey string `json:"api_key"`
}

// Read loads the configuration from a file or initializes a new one.
// It tries to load ~/.gatorcli.json. If the file doesn't exist, it returns a default Config.
func Read() (Config, error) {
	home, err := homedir.Dir()
	if err != nil {
		return Config{}, fmt.Errorf("failed to find home directory: %w", err)
	}

	configPath := filepath.Join(home, ".gatorcli.json")

	// Try to read the file content
	data, err := os.ReadFile(configPath)
	if errors.Is(err, os.ErrNotExist) {
		// File does not exist, return a default/empty configuration
		return Config{}, nil
	}
	if err != nil {
		return Config{}, fmt.Errorf("failed to read config file: %w", err)
	}

	var cfg Config
	// Unmarshal the JSON data into the Config struct
	if err := json.Unmarshal(data, &cfg); err != nil {
		return Config{}, fmt.Errorf("failed to unmarshal config JSON: %w", err)
	}

	return cfg, nil
}

// Save writes the current configuration back to the file (~/.gatorcli.json).
// This is a helper function used by SetUser.
func (c *Config) Save() error {
	home, err := homedir.Dir()
	if err != nil {
		return fmt.Errorf("failed to find home directory: %w", err)
	}

	configPath := filepath.Join(home, ".gatorcli.json")

	// Marshal the struct into JSON data with a neat indentation
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal config JSON: %w", err)
	}

	// Write the JSON data to the file
	if err := os.WriteFile(configPath, data, 0600); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	return nil
}

// SetUser updates the UserID field in the Config struct and saves the changes to disk.
// This is the method that resolves the 's.Config.SetUser undefined' error.
func (c *Config) SetUser(username string) error {
	c.UserID = username
	return c.Save()
}
