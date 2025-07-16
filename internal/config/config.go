package config

import (
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

// Config represents the application configuration
type Config struct {
	Server ServerConfig `yaml:"server"`
	Cache  CacheConfig  `yaml:"cache"`
	Rules  RulesConfig  `yaml:"rules"`
}

// ServerConfig contains server-related configuration
type ServerConfig struct {
	Port int `yaml:"port"`
}

// CacheConfig contains cache-related configuration
type CacheConfig struct {
	TTL    string `yaml:"ttl"`
	Folder string `yaml:"folder"`
}

// RulesConfig contains caching rules configuration
type RulesConfig struct {
	Mode  string      `yaml:"mode"` // "whitelist" or "blacklist"
	Rules []CacheRule `yaml:"rules"`
}

// CacheRule defines a caching rule
type CacheRule struct {
	BaseURI string   `yaml:"base_uri"`
	Methods []string `yaml:"methods"`
}

// Load loads configuration from a YAML file
func Load(path string) (*Config, error) {
	var config Config

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading config file: %w", err)
	}

	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("parsing config YAML: %w", err)
	}

	// Set defaults
	if config.Server.Port == 0 {
		config.Server.Port = 8080
	}

	return &config, nil
}

// GetCacheTTL parses and returns the cache TTL duration
func (c *Config) GetCacheTTL() (time.Duration, error) {
	return time.ParseDuration(c.Cache.TTL)
}

// Validate validates the configuration
func (c *Config) Validate() error {
	if c.Server.Port <= 0 || c.Server.Port > 65535 {
		return fmt.Errorf("invalid port: %d", c.Server.Port)
	}

	if c.Cache.TTL == "" {
		return fmt.Errorf("cache TTL is required")
	}

	if _, err := c.GetCacheTTL(); err != nil {
		return fmt.Errorf("invalid cache TTL format: %w", err)
	}

	if c.Cache.Folder == "" {
		return fmt.Errorf("cache folder is required")
	}

	if c.Rules.Mode != "whitelist" && c.Rules.Mode != "blacklist" {
		return fmt.Errorf("rules mode must be 'whitelist' or 'blacklist', got: %s", c.Rules.Mode)
	}

	return nil
}
