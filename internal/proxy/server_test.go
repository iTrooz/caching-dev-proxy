package proxy

import (
	"net/http"
	"net/http/httptest"
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

func TestConfigRuleMatch(t *testing.T) {
	rule := &ConfigRule{
		CacheRule: config.CacheRule{
			BaseURI: "https://api.example.com",
			Methods: []string{"GET", "POST"},
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
			name:       "matching URL and method",
			targetURL:  "https://api.example.com/users",
			method:     "GET",
			statusCode: 200,
			want:       true,
		},
		{
			name:       "matching URL different method",
			targetURL:  "https://api.example.com/users",
			method:     "POST",
			statusCode: 200,
			want:       true,
		},
		{
			name:       "matching URL non-matching method",
			targetURL:  "https://api.example.com/users",
			method:     "DELETE",
			statusCode: 200,
			want:       false,
		},
		{
			name:       "non-matching URL",
			targetURL:  "https://other.example.com/users",
			method:     "GET",
			statusCode: 200,
			want:       false,
		},
		{
			name:       "case insensitive method",
			targetURL:  "https://api.example.com/users",
			method:     "get",
			statusCode: 200,
			want:       true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := rule.Match(tt.targetURL, tt.method, tt.statusCode)
			if got != tt.want {
				t.Errorf("ConfigRule.Match() = %v, want %v", got, tt.want)
			}
		})
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
			got := rule.Match(tt.targetURL, tt.method, tt.statusCode)
			if got != tt.want {
				t.Errorf("ConfigRule.Match() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGetTargetURL(t *testing.T) {
	tests := []struct {
		name string
		req  *http.Request
		want string
	}{
		{
			name: "absolute URL",
			req:  httptest.NewRequest("GET", "https://api.example.com/users", nil),
			want: "https://api.example.com/users",
		},
		{
			name: "relative URL with host header",
			req: func() *http.Request {
				req := httptest.NewRequest("GET", "/users", nil)
				req.Host = "api.example.com"
				return req
			}(),
			want: "http://api.example.com/users",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := getTargetURL(tt.req)
			if got != tt.want {
				t.Errorf("getTargetURL() = %v, want %v", got, tt.want)
			}
		})
	}
}
