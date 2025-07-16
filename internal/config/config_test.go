package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoad(t *testing.T) {
	// Create a temporary config file
	tempDir := t.TempDir()
	configFile := filepath.Join(tempDir, "test_config.yaml")

	configContent := `
server:
  port: 9999
cache:
  ttl: "30m"
  folder: "./test_cache"
rules:
  mode: "whitelist"
  rules:
    - base_uri: "https://example.com"
      methods: ["GET"]
`

	err := os.WriteFile(configFile, []byte(configContent), 0644)
	if err != nil {
		t.Fatalf("Failed to create test config file: %v", err)
	}

	// Test loading the config
	config, err := Load(configFile)
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	// Verify values
	if config.Server.Port != 9999 {
		t.Errorf("Expected port 9999, got %d", config.Server.Port)
	}

	if config.Cache.TTL != "30m" {
		t.Errorf("Expected TTL '30m', got '%s'", config.Cache.TTL)
	}

	if config.Rules.Mode != "whitelist" {
		t.Errorf("Expected mode 'whitelist', got '%s'", config.Rules.Mode)
	}

	if len(config.Rules.Rules) != 1 {
		t.Errorf("Expected 1 rule, got %d", len(config.Rules.Rules))
	}
}

func TestValidate(t *testing.T) {
	tests := []struct {
		name    string
		config  Config
		wantErr bool
	}{
		{
			name: "valid config",
			config: Config{
				Server: ServerConfig{Port: 8080},
				Cache:  CacheConfig{TTL: "1h", Folder: "/tmp/cache"},
				Rules:  RulesConfig{Mode: "whitelist"},
			},
			wantErr: false,
		},
		{
			name: "invalid port",
			config: Config{
				Server: ServerConfig{Port: -1},
				Cache:  CacheConfig{TTL: "1h", Folder: "/tmp/cache"},
				Rules:  RulesConfig{Mode: "whitelist"},
			},
			wantErr: true,
		},
		{
			name: "invalid TTL",
			config: Config{
				Server: ServerConfig{Port: 8080},
				Cache:  CacheConfig{TTL: "invalid", Folder: "/tmp/cache"},
				Rules:  RulesConfig{Mode: "whitelist"},
			},
			wantErr: true,
		},
		{
			name: "invalid mode",
			config: Config{
				Server: ServerConfig{Port: 8080},
				Cache:  CacheConfig{TTL: "1h", Folder: "/tmp/cache"},
				Rules:  RulesConfig{Mode: "invalid"},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestGetCacheTTL(t *testing.T) {
	config := Config{
		Cache: CacheConfig{TTL: "1h30m"},
	}

	ttl, err := config.GetCacheTTL()
	if err != nil {
		t.Fatalf("GetCacheTTL() error = %v", err)
	}

	expected := time.Hour + 30*time.Minute
	if ttl != expected {
		t.Errorf("GetCacheTTL() = %v, want %v", ttl, expected)
	}
}
