package main

import (
	"net/http"
	"net/http/httptest"

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
func fixture_config(upstreamURL, tempDir string, rules *config.RulesConfig) *config.Config {
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

// fixture_proxy creates a proxy server with the given config and returns the server and test server
func fixture_proxy(cfg *config.Config) (*proxy.Server, *httptest.Server, error) {
	proxyServer, err := proxy.New(cfg)
	if err != nil {
		return nil, nil, err
	}

	// Create test proxy HTTP server using goproxy
	proxyTestServer := httptest.NewServer(proxyServer.GetProxy())

	return proxyServer, proxyTestServer, nil
}
