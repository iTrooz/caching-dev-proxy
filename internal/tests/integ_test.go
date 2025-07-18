package tests

import (
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"caching-dev-proxy/internal/config"
)

func TestProxyIntegration(t *testing.T) {
	// Create a test upstream server
	upstream := fixture_upstream()
	defer upstream.Close()

	// Create test config
	tempDir := t.TempDir()
	cfg := fixture_config(tempDir, nil)

	// Create proxy server
	_, proxyTestServer, client, err := fixture_proxy(cfg)
	require.NoError(t, err, "Failed to create proxy server")
	defer proxyTestServer.Close()

	// Test first request (should hit upstream and cache)
	t.Run("first request - cache miss", func(t *testing.T) {
		resp, err := client.Get(upstream.URL + "/test")
		require.NoError(t, err, "Request failed")
		defer func() { _ = resp.Body.Close() }()

		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.Equal(t, "MISS", resp.Header.Get("X-Cache"))

		body, _ := io.ReadAll(resp.Body)
		assert.Contains(t, string(body), "Hello from upstream")
	})

	// Test second request (should hit cache)
	t.Run("second request - cache hit", func(t *testing.T) {
		resp, err := client.Get(upstream.URL + "/test")
		require.NoError(t, err, "Request failed")
		defer func() { _ = resp.Body.Close() }()

		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.Equal(t, "HIT", resp.Header.Get("X-Cache"))

		body, _ := io.ReadAll(resp.Body)
		assert.Contains(t, string(body), "Hello from upstream")
	})

	// Verify cache file was created
	t.Run("verify cache file exists", func(t *testing.T) {
		upstreamURL, _ := url.Parse(upstream.URL)
		expectedCachePath := filepath.Join(tempDir, upstreamURL.Host, "test", "GET.bin")

		_, err := os.Stat(expectedCachePath)
		assert.NoError(t, err, "Cache file should exist at %s", expectedCachePath)
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
	cfg := fixture_config(tempDir, customRules)

	// Create proxy server
	_, proxyTestServer, client, err := fixture_proxy(cfg)
	require.NoError(t, err, "Failed to create proxy server")
	defer proxyTestServer.Close()

	// Test that requests are cached (since we're using blacklist mode and the upstream URL is not in the blacklist)
	t.Run("request should be cached with blacklist rules", func(t *testing.T) {
		resp, err := client.Get(upstream.URL + "/test")
		require.NoError(t, err, "Request failed")
		defer func() { _ = resp.Body.Close() }()

		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.Equal(t, "MISS", resp.Header.Get("X-Cache"))

		// Second request should hit cache
		resp2, err := client.Get(upstream.URL + "/test")
		require.NoError(t, err, "Request failed")
		defer func() { _ = resp2.Body.Close() }()

		assert.Equal(t, "HIT", resp2.Header.Get("X-Cache"))
	})
}
