package config

import (
	"fmt"
	"os"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/knadh/koanf/parsers/yaml"
	"github.com/knadh/koanf/providers/file"
	"github.com/knadh/koanf/providers/structs"
	"github.com/knadh/koanf/v2"
	"github.com/sirupsen/logrus"
)

// Config represents the application configuration
type Config struct {
	Server ServerConfig `koanf:"server"`
	Cache  CacheConfig  `koanf:"cache"`
	Rules  RulesConfig  `koanf:"rules"`
	Log    LogConfig    `koanf:"log"`
}

// ServerConfig contains server-related configuration
// Updated for nested http/https config
type ServerConfig struct {
	HTTP  HTTPConfig  `koanf:"http"`
	HTTPS HTTPSConfig `koanf:"https"`
}

type HTTPConfig struct {
	Address string `koanf:"address"`
}

type HTTPSConfig struct {
	Enabled     bool              `koanf:"enabled"`
	CAKeyFile   string            `koanf:"ca_key_file"`
	CACertFile  string            `koanf:"ca_cert_file"`
	Transparent TransparentConfig `koanf:"transparent"`
}

type TransparentConfig struct {
	Address string `koanf:"address"`
}

// CacheConfig contains cache-related configuration
type CacheConfig struct {
	TTL    string `koanf:"ttl"`
	Folder string `koanf:"folder"`
}

// RulesMode represents the mode of rule evaluation (whitelist or blacklist)
type RulesMode string

const (
	RulesModeWhitelist RulesMode = "whitelist"
	RulesModeBlacklist RulesMode = "blacklist"
)

type LogConfig struct {
	Level      string `koanf:"level"`
	ThirdParty bool   `koanf:"third_party"`
}

type RulesConfig struct {
	Mode  RulesMode   `koanf:"mode"` // "whitelist" or "blacklist"
	Rules []CacheRule `koanf:"rules"`
}

// CacheRule defines a caching rule
type CacheRule struct {
	BaseURI     string   `koanf:"base_uri"`
	Methods     []string `koanf:"methods"`
	StatusCodes []string `koanf:"status_codes,omitempty"` // e.g., ["200", "404", "4xx", "5xx"]
}

// DefaultConfig holds the default configuration values
var DefaultConfig = Config{
	Server: ServerConfig{
		HTTP: HTTPConfig{
			Address: ":8080",
		},
		HTTPS: HTTPSConfig{
			Enabled:    true,
			CAKeyFile:  "",
			CACertFile: "",
			Transparent: TransparentConfig{
				Address: ":8443",
			},
		},
	},
	Cache: CacheConfig{
		TTL:    "",
		Folder: "./cache",
	},
	Rules: RulesConfig{
		Mode:  RulesModeBlacklist,
		Rules: []CacheRule{},
	},
	Log: LogConfig{
		Level:      "info",
		ThirdParty: false,
	},
}

// Load loads configuration from a YAML file using koanf
func Load(path string) (*Config, error) {
	k := koanf.New(":")

	// Load defaults from DefaultConfig
	if err := k.Load(structs.Provider(DefaultConfig, "koanf"), nil); err != nil {
		return nil, fmt.Errorf("loading defaults: %w", err)
	}

	// Load YAML file if present
	logrus.Debugf("Loading config from %s", path)
	if _, err := os.Stat(path); err == nil {
		if err := k.Load(file.Provider(path), yaml.Parser()); err != nil {
			return nil, fmt.Errorf("loading config file: %w", err)
		}
	}

	var config Config
	if err := k.Unmarshal("", &config); err != nil {
		return nil, fmt.Errorf("unmarshalling config: %w", err)
	}

	return &config, nil
}

// GetCacheTTL parses and returns the cache TTL duration
func (c *Config) GetCacheTTL() (time.Duration, error) {
	if c.Cache.TTL == "" {
		return 0, nil // infinite
	} else {
		return time.ParseDuration(c.Cache.TTL)
	}
}

// Validate validates the configuration
func (c *Config) Validate() error {
	if _, err := c.GetCacheTTL(); err != nil {
		return fmt.Errorf("invalid cache TTL format: %w", err)
	}

	if c.Rules.Mode != "whitelist" && c.Rules.Mode != "blacklist" {
		return fmt.Errorf("rules mode must be 'whitelist' or 'blacklist', got: %s", c.Rules.Mode)
	}

	validLogLevels := []string{"debug", "info", "warn", "error"}
	if !slices.Contains(validLogLevels, c.Log.Level) {
		return fmt.Errorf("log level must be one of 'debug', 'info', 'warn', 'error', got: %s", c.Log.Level)
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

func NewRulesConfig(mode RulesMode, rules ...CacheRule) *RulesConfig {
	return &RulesConfig{
		Mode:  mode,
		Rules: rules,
	}
}

func NewCacheRule(baseURI string, methods ...string) CacheRule {
	return CacheRule{
		BaseURI: baseURI,
		Methods: methods,
	}
}
