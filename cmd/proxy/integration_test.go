package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"caching-dev-proxy/internal/config"
	"caching-dev-proxy/internal/proxy"
)

func TestProxyIntegration(t *testing.T) {
	// Create a test upstream server
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"message": "Hello from upstream", "path": "` + r.URL.Path + `"}`))
	}))
	defer upstream.Close()

	// Create temporary directory for cache
	tempDir := t.TempDir()

	// Create test config
	cfg := &config.Config{
		Server: config.ServerConfig{Port: 0}, // Will be set by test server
		Cache: config.CacheConfig{
			TTL:    "1h",
			Folder: tempDir,
		},
		Rules: config.RulesConfig{
			Mode: "whitelist",
			Rules: []config.CacheRule{
				{
					BaseURI: upstream.URL,
					Methods: []string{"GET"},
				},
			},
		},
	}

	// Create proxy server
	proxyServer, err := proxy.New(cfg)
	if err != nil {
		t.Fatalf("Failed to create proxy server: %v", err)
	}

	// Create test proxy HTTP server using goproxy
	proxyTestServer := httptest.NewServer(proxyServer.GetProxy())
	defer proxyTestServer.Close()

	// Create HTTP client that uses our proxy
	proxyURL, _ := url.Parse(proxyTestServer.URL)
	client := &http.Client{
		Transport: &http.Transport{
			Proxy: http.ProxyURL(proxyURL),
		},
		Timeout: 10 * time.Second,
	}

	// Test first request (should hit upstream and cache)
	t.Run("first request - cache miss", func(t *testing.T) {
		resp, err := client.Get(upstream.URL + "/test")
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Errorf("Expected status 200, got %d", resp.StatusCode)
		}

		if resp.Header.Get("X-Cache") != "MISS" {
			t.Errorf("Expected X-Cache: MISS, got %s", resp.Header.Get("X-Cache"))
		}

		body, _ := io.ReadAll(resp.Body)
		if !strings.Contains(string(body), "Hello from upstream") {
			t.Errorf("Unexpected response body: %s", string(body))
		}
	})

	// Test second request (should hit cache)
	t.Run("second request - cache hit", func(t *testing.T) {
		resp, err := client.Get(upstream.URL + "/test")
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Errorf("Expected status 200, got %d", resp.StatusCode)
		}

		if resp.Header.Get("X-Cache") != "HIT" {
			t.Errorf("Expected X-Cache: HIT, got %s", resp.Header.Get("X-Cache"))
		}

		body, _ := io.ReadAll(resp.Body)
		if !strings.Contains(string(body), "Hello from upstream") {
			t.Errorf("Unexpected response body: %s", string(body))
		}
	})

	// Verify cache file was created
	t.Run("verify cache file exists", func(t *testing.T) {
		upstreamURL, _ := url.Parse(upstream.URL)
		expectedCachePath := filepath.Join(tempDir, upstreamURL.Host, "test", "GET.bin")

		if _, err := os.Stat(expectedCachePath); os.IsNotExist(err) {
			t.Errorf("Cache file should exist at %s", expectedCachePath)
		}
	})
}
