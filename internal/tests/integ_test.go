package main

import (
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"caching-dev-proxy/internal/config"
)

func TestProxyIntegration(t *testing.T) {
	// Create a test upstream server
	upstream := fixture_upstream()
	defer upstream.Close()

	// Create temporary directory for cache
	tempDir := t.TempDir()

	// Create test config
	cfg := fixture_config(upstream.URL, tempDir, nil)

	// Create proxy server
	_, proxyTestServer, err := fixture_proxy(cfg)
	if err != nil {
		t.Fatalf("Failed to create proxy server: %v", err)
	}
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
		defer func() { _ = resp.Body.Close() }()

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
		defer func() { _ = resp.Body.Close() }()

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

		if _, err := os.Stat(expectedCachePath); err != nil {
			t.Errorf("Cache file should exist at %s", expectedCachePath)
		}
	})
}

func TestProxyIntegrationWithCustomRules(t *testing.T) {
	// Create a test upstream server
	upstream := fixture_upstream()
	defer upstream.Close()

	// Create temporary directory for cache
	tempDir := t.TempDir()

	// Create custom rules (blacklist mode)
	customRules := &config.RulesConfig{
		Mode: "blacklist",
		Rules: []config.CacheRule{
			{
				BaseURI: "https://example.com",
				Methods: []string{"GET"},
			},
		},
	}

	// Create test config with custom rules
	cfg := fixture_config(upstream.URL, tempDir, customRules)

	// Create proxy server
	_, proxyTestServer, err := fixture_proxy(cfg)
	if err != nil {
		t.Fatalf("Failed to create proxy server: %v", err)
	}
	defer proxyTestServer.Close()

	// Create HTTP client that uses our proxy
	proxyURL, _ := url.Parse(proxyTestServer.URL)
	client := &http.Client{
		Transport: &http.Transport{
			Proxy: http.ProxyURL(proxyURL),
		},
		Timeout: 10 * time.Second,
	}

	// Test that requests are cached (since we're using blacklist mode and the upstream URL is not in the blacklist)
	t.Run("request should be cached with blacklist rules", func(t *testing.T) {
		resp, err := client.Get(upstream.URL + "/test")
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
		defer func() { _ = resp.Body.Close() }()

		if resp.StatusCode != http.StatusOK {
			t.Errorf("Expected status 200, got %d", resp.StatusCode)
		}

		if resp.Header.Get("X-Cache") != "MISS" {
			t.Errorf("Expected X-Cache: MISS, got %s", resp.Header.Get("X-Cache"))
		}

		// Second request should hit cache
		resp2, err := client.Get(upstream.URL + "/test")
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
		defer func() { _ = resp2.Body.Close() }()

		if resp2.Header.Get("X-Cache") != "HIT" {
			t.Errorf("Expected X-Cache: HIT, got %s", resp2.Header.Get("X-Cache"))
		}
	})
}
