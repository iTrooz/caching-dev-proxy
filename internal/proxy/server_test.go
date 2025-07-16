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

func TestShouldCache(t *testing.T) {
	cfg := &config.Config{
		Cache: config.CacheConfig{TTL: "1h", Folder: "/tmp/test"},
		Rules: config.RulesConfig{
			Mode: "whitelist",
			Rules: []config.CacheRule{
				{
					BaseURI: "https://api.example.com",
					Methods: []string{"GET"},
				},
			},
		},
	}

	server, _ := New(cfg)

	tests := []struct {
		name   string
		method string
		url    string
		want   bool
	}{
		{
			name:   "GET request matching whitelist",
			method: "GET",
			url:    "https://api.example.com/users",
			want:   true,
		},
		{
			name:   "POST request not in whitelist methods",
			method: "POST",
			url:    "https://api.example.com/users",
			want:   false,
		},
		{
			name:   "GET request not matching whitelist URI",
			method: "GET",
			url:    "https://other.example.com/users",
			want:   false,
		},
		{
			name:   "Non-GET request",
			method: "DELETE",
			url:    "https://api.example.com/users",
			want:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.url, nil)
			req.Host = "api.example.com"

			got := server.shouldCache(req)
			if got != tt.want {
				t.Errorf("shouldCache() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestShouldCacheBlacklist(t *testing.T) {
	cfg := &config.Config{
		Cache: config.CacheConfig{TTL: "1h", Folder: "/tmp/test"},
		Rules: config.RulesConfig{
			Mode: "blacklist",
			Rules: []config.CacheRule{
				{
					BaseURI: "https://no-cache.example.com",
					Methods: []string{"GET"},
				},
			},
		},
	}

	server, _ := New(cfg)

	tests := []struct {
		name   string
		method string
		url    string
		want   bool
	}{
		{
			name:   "GET request matching blacklist",
			method: "GET",
			url:    "https://no-cache.example.com/users",
			want:   false,
		},
		{
			name:   "GET request not in blacklist",
			method: "GET",
			url:    "https://api.example.com/users",
			want:   true,
		},
		{
			name:   "POST request not matching blacklist methods",
			method: "POST",
			url:    "https://no-cache.example.com/users",
			want:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.url, nil)

			got := server.shouldCache(req)
			if got != tt.want {
				t.Errorf("shouldCache() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestMatchesRule(t *testing.T) {
	rule := config.CacheRule{
		BaseURI: "https://api.example.com",
		Methods: []string{"GET", "POST"},
	}

	tests := []struct {
		name      string
		targetURL string
		method    string
		want      bool
	}{
		{
			name:      "matching URL and method",
			targetURL: "https://api.example.com/users",
			method:    "GET",
			want:      true,
		},
		{
			name:      "matching URL different method",
			targetURL: "https://api.example.com/users",
			method:    "POST",
			want:      true,
		},
		{
			name:      "matching URL non-matching method",
			targetURL: "https://api.example.com/users",
			method:    "DELETE",
			want:      false,
		},
		{
			name:      "non-matching URL",
			targetURL: "https://other.example.com/users",
			method:    "GET",
			want:      false,
		},
		{
			name:      "case insensitive method",
			targetURL: "https://api.example.com/users",
			method:    "get",
			want:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := matchesRule(tt.targetURL, tt.method, rule)
			if got != tt.want {
				t.Errorf("matchesRule() = %v, want %v", got, tt.want)
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
