package tests

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"time"

	"caching-dev-proxy/internal/config"
	"caching-dev-proxy/internal/proxy"
)

// fixture_upstream creates a test upstream server
func fixture_upstream() *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, requ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"message": "Hello from upstream", "path": "` + requ.URL.Path + `"}`))
	}))
}

// fixture_config creates a test config with optional rules
func fixture_config(tempDir string, rules *config.RulesConfig) *config.Config {
	cfg := &config.Config{
		Server: config.ServerConfig{Port: 0}, // Will be set by test server
		Cache: config.CacheConfig{
			TTL:    "1h",
			Folder: tempDir,
		},
	}

	if rules != nil {
		cfg.Rules = *rules
	}

	return cfg
}

// fixture_proxy creates a proxy server with the given config and returns the server, test server, and HTTP client
func fixture_proxy(cfg *config.Config) (*proxy.Server, *httptest.Server, *http.Client, error) {
	proxyServer, err := proxy.New(cfg)
	if err != nil {
		return nil, nil, nil, err
	}

	// Create test proxy HTTP server using goproxy
	proxyTestServer := httptest.NewServer(proxyServer.GetProxy())

	// Create HTTP client that uses our proxy
	proxyURL, _ := url.Parse(proxyTestServer.URL)
	client := &http.Client{
		Transport: &http.Transport{
			Proxy: http.ProxyURL(proxyURL),
		},
		Timeout: 10 * time.Second,
	}

	return proxyServer, proxyTestServer, client, nil
}
