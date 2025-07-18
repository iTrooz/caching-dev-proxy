package proxy

import (
	"net/http"
	"net/url"
	"testing"

	"caching-dev-proxy/internal/config"
)

func TestNew(t *testing.T) {
	cfg := &config.Config{
		Server: config.ServerConfig{Port: 8080},
		Cache:  config.CacheConfig{TTL: "1h", Folder: "/tmp/test"},
		Rules:  config.RulesConfig{Mode: "whitelist"},
	}

	server, err := New(cfg)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	if server.config != cfg {
		t.Errorf("Server config not set correctly")
	}
}

func TestConfigRuleMatchWithStatusCodes(t *testing.T) {
	rule := &ConfigRule{
		CacheRule: config.CacheRule{
			BaseURI:     "https://api.example.com",
			Methods:     []string{"GET", "POST"},
			StatusCodes: []string{"200", "4xx"},
		},
	}

	tests := []struct {
		name       string
		targetURL  string
		method     string
		statusCode int
		want       bool
	}{
		{
			name:       "matching URL, method, and status code",
			targetURL:  "https://api.example.com/users",
			method:     "GET",
			statusCode: 200,
			want:       true,
		},
		{
			name:       "matching URL, method, and status pattern",
			targetURL:  "https://api.example.com/users",
			method:     "GET",
			statusCode: 404,
			want:       true,
		},
		{
			name:       "matching URL and method, non-matching status",
			targetURL:  "https://api.example.com/users",
			method:     "GET",
			statusCode: 500,
			want:       false,
		},
		{
			name:       "non-matching method",
			targetURL:  "https://api.example.com/users",
			method:     "DELETE",
			statusCode: 200,
			want:       false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			u, err := url.Parse(tt.targetURL)
			if err != nil {
				t.Fatalf("Failed to parse URL %s: %v", tt.targetURL, err)
			}

			requ := &http.Request{
				URL:    u,
				Method: tt.method,
			}
			resp := &http.Response{
				StatusCode: tt.statusCode,
			}

			got := rule.Match(requ, resp)
			if got != tt.want {
				t.Errorf("ConfigRule.Match() = %v, want %v", got, tt.want)
			}
		})
	}
}
