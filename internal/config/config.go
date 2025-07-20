package config

import (
	"fmt"
	"os"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/knadh/koanf/maps"
	"github.com/knadh/koanf/parsers/yaml"
	"github.com/knadh/koanf/providers/file"
	"github.com/knadh/koanf/providers/structs"
	"github.com/knadh/koanf/v2"
	"github.com/sirupsen/logrus"
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
	Port       int       `yaml:"port"`
	SSLBumping SSLConfig `yaml:"ssl_bumping"`
}

// SSLConfig contains SSL bumping configuration
type SSLConfig struct {
	Enabled    bool   `yaml:"enabled"`
	CAKeyFile  string `yaml:"ca_key_file"`
	CACertFile string `yaml:"ca_cert_file"`
}

// CacheConfig contains cache-related configuration
type CacheConfig struct {
	TTL    string `yaml:"ttl"`
	Folder string `yaml:"folder"`
}

// RulesMode represents the mode of rule evaluation (whitelist or blacklist)
type RulesMode string

const (
	RulesModeWhitelist RulesMode = "whitelist"
	RulesModeBlacklist RulesMode = "blacklist"
)

type LogConfig struct {
	Level      string `yaml:"level"`
	ThirdParty bool   `yaml:"third_party"`
}

type RulesConfig struct {
	Mode  RulesMode   `yaml:"mode"` // "whitelist" or "blacklist"
	Rules []CacheRule `yaml:"rules"`
}

// CacheRule defines a caching rule
type CacheRule struct {
	BaseURI     string   `yaml:"base_uri"`
	Methods     []string `yaml:"methods"`
	StatusCodes []string `yaml:"status_codes,omitempty"` // e.g., ["200", "404", "4xx", "5xx"]
}

// DefaultConfig holds the default configuration values
var DefaultConfig = Config{
	Server: ServerConfig{
		Port: 8080,
		SSLBumping: SSLConfig{
			Enabled:    true,
			CAKeyFile:  "",
			CACertFile: "",
		},
	},
	Cache: CacheConfig{
		TTL:    "1h",
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

type TransformFunc func(string, any) (*string, *any)

func transformMap(transformFunc TransformFunc, m map[string]any) {
	for k, v := range m {
		nk, nv := transformFunc(k, v)
		if nk != nil && nv != nil {
			delete(m, k)
			m[*nk] = nv
		} else if nk != nil && nv == nil {
			delete(m, k)
			m[*nk] = v
		} else if nk == nil && nv != nil {
			m[k] = nv
		}

		if subMap, ok := v.(map[string]any); ok {
			transformMap(transformFunc, subMap)
		}
	}
}

func WithTransformFunc(transformFunc TransformFunc) koanf.Option {
	return koanf.WithMergeFunc(func(src, dest map[string]interface{}) error {
		transformMap(transformFunc, src)

		return maps.MergeStrict(src, dest)
	})
}

func ToLowercase() koanf.Option {
	return WithTransformFunc(func(s string, a any) (*string, *any) {
		lowered := strings.ToLower(s)
		return &lowered, nil
	})
}

// Load loads configuration from a YAML file using koanf
func Load(path string) (*Config, error) {
	k := koanf.New(":")

	// Load defaults from DefaultConfig
	if err := k.Load(structs.Provider(DefaultConfig, "."), nil, ToLowercase()); err != nil {
		return nil, fmt.Errorf("loading defaults: %w", err)
	}

	// Load YAML file if present
	logrus.Infof("Loading config from %s", path)
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
	return time.ParseDuration(c.Cache.TTL)
}

// Validate validates the configuration
func (c *Config) Validate() error {
	if c.Server.Port <= 0 || c.Server.Port > 65535 {
		return fmt.Errorf("invalid port: %d", c.Server.Port)
	}

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
