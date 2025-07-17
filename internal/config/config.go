package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Config represents the application configuration
type Config struct {
	Server ServerConfig `yaml:"server"`
	Cache  CacheConfig  `yaml:"cache"`
	Rules  RulesConfig  `yaml:"rules"`
	Log    LogConfig    `yaml:"log"`
}

// ServerConfig contains server-related configuration
type ServerConfig struct {
	Port      int       `yaml:"port"`
	SSLBumping SSLConfig `yaml:"ssl_bumping"`
}

// SSLConfig contains SSL bumping configuration
type SSLConfig struct {
	Enabled  bool   `yaml:"enabled"`
	CertFile string `yaml:"cert_file"`
	KeyFile  string `yaml:"key_file"`
	CAFile   string `yaml:"ca_file"`
}

// CacheConfig contains cache-related configuration
type CacheConfig struct {
	TTL    string `yaml:"ttl"`
	Folder string `yaml:"folder"`
}

// RulesConfig contains caching rules configuration
type RulesMode string

const (
	RulesModeWhitelist RulesMode = "whitelist"
	RulesModeBlacklist RulesMode = "blacklist"
)

type RulesConfig struct {
	Mode  RulesMode   `yaml:"mode"` // "whitelist" or "blacklist"
	Rules []CacheRule `yaml:"rules"`
}

// LogConfig contains logging configuration
type LogConfig struct {
	Level string `yaml:"level"` // "debug", "info", "warn", "error"
}

// CacheRule defines a caching rule
type CacheRule struct {
	BaseURI     string   `yaml:"base_uri"`
	Methods     []string `yaml:"methods"`
	StatusCodes []string `yaml:"status_codes,omitempty"` // e.g., ["200", "404", "4xx", "5xx"]
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
	if config.Log.Level == "" {
		config.Log.Level = "info"
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

	validLogLevels := map[string]bool{
		"debug": true, "info": true, "warn": true, "error": true,
	}
	if !validLogLevels[c.Log.Level] {
		return fmt.Errorf("log level must be one of 'debug', 'info', 'warn', 'error', got: %s", c.Log.Level)
	}

	// Validate SSL bumping configuration
	if c.Server.SSLBumping.Enabled {
		if c.Server.SSLBumping.CAFile == "" {
			return fmt.Errorf("ssl_bumping.ca_file is required when SSL bumping is enabled")
		}
		if c.Server.SSLBumping.KeyFile == "" {
			return fmt.Errorf("ssl_bumping.key_file is required when SSL bumping is enabled")
		}
	}

	return nil
}

// MatchesStatusCode checks if a status code matches a pattern
func MatchesStatusCode(statusCode int, pattern string) bool {
	statusStr := strconv.Itoa(statusCode)

	// Exact match
	if pattern == statusStr {
		return true
	}

	// Pattern matching (e.g., "4xx", "5xx")
	if strings.HasSuffix(pattern, "xx") && len(pattern) == 3 {
		prefix := pattern[:1]
		return strings.HasPrefix(statusStr, prefix)
	}

	return false
}
