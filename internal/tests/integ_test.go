package tests

import (
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"

	"caching-dev-proxy/internal/config"
)

func TestSimpleHit(t *testing.T) {
	// Create a test upstream server
	upstream := fixture_upstream()
	defer upstream.Close()

	// Create test config
	tempDir := t.TempDir()
	cfg := fixture_config(tempDir, nil)

	// Create proxy server
	_, proxyTestServer, client := fixture_proxy(cfg)
	defer proxyTestServer.Close()

	// Test first request (should hit upstream and cache)
	t.Run("first request - cache miss", func(t *testing.T) {
		resp, err := client.Get(upstream.URL + "/test")
		if err != nil {
			panic(fmt.Sprintf("Request failed: %v", err))
		}

		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.Equal(t, "MISS", resp.Header.Get("X-Cache"))

		body := readBodyAndClose(resp)
		assert.Contains(t, body, "Hello from upstream")
	})

	// Test second request (should hit cache)
	t.Run("second request - cache hit", func(t *testing.T) {
		resp, err := client.Get(upstream.URL + "/test")
		if err != nil {
			panic(fmt.Sprintf("Request failed: %v", err))
		}

		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.Equal(t, "HIT", resp.Header.Get("X-Cache"))

		body := readBodyAndClose(resp)
		assert.Contains(t, body, "Hello from upstream")
	})

	// Verify cache file was created
	t.Run("verify cache file exists", func(t *testing.T) {
		upstreamURL, err := url.Parse(upstream.URL)
		if err != nil {
			panic(fmt.Sprintf("Failed to parse upstream URL: %v", err))
		}
		expectedCachePath := filepath.Join(tempDir, upstreamURL.Host, "test", "GET.bin")

		_, err = os.Stat(expectedCachePath)
		assert.NoError(t, err, "Cache file should exist at %s", expectedCachePath)
	})
}

func TestHitWithBlacklist(t *testing.T) {
	// Create a test upstream server
	upstream := fixture_upstream()
	defer upstream.Close()

	// Create config
	customRules := config.NewRulesConfig(config.RulesModeBlacklist,
		config.NewCacheRule("https://example.com", "GET"),
	)
	cfg := fixture_config(t.TempDir(), customRules)

	// Create proxy server
	_, proxyTestServer, client := fixture_proxy(cfg)
	defer proxyTestServer.Close()

	// Test that requests are cached (since we're using blacklist mode and the upstream URL is not in the blacklist)
	t.Run("first request - cache miss", func(t *testing.T) {
		resp, err := client.Get(upstream.URL + "/test")
		if err != nil {
			panic(fmt.Sprintf("Request failed: %v", err))
		}
		defer func() {
			if err := resp.Body.Close(); err != nil {
				panic(fmt.Sprintf("Failed to close response body: %v", err))
			}
		}()

		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.Equal(t, "MISS", resp.Header.Get("X-Cache"))
	})

	t.Run("second request - cache hit", func(t *testing.T) {
		resp2, err := client.Get(upstream.URL + "/test")
		if err != nil {
			panic(fmt.Sprintf("Request failed: %v", err))
		}
		defer func() {
			if err := resp2.Body.Close(); err != nil {
				panic(fmt.Sprintf("Failed to close response body: %v", err))
			}
		}()

		assert.Equal(t, "HIT", resp2.Header.Get("X-Cache"))
	})
}

func TestHitWithWhitelist(t *testing.T) {
	// Create a test upstream server
	upstream := fixture_upstream()
	defer upstream.Close()

	// Create config with whitelist rules
	customRules := config.NewRulesConfig(config.RulesModeWhitelist,
		config.NewCacheRule("https://example.com", "GET"),
	)
	cfg := fixture_config(t.TempDir(), customRules)

	// Create proxy server
	_, proxyTestServer, client := fixture_proxy(cfg)
	defer proxyTestServer.Close()

	// Test that requests are cached (since we're using whitelist mode and the upstream URL is in the whitelist)
	t.Run("first request - cache miss", func(t *testing.T) {
		resp, err := client.Get(upstream.URL + "/test")
		if err != nil {
			panic(fmt.Sprintf("Request failed: %v", err))
		}
		defer func() {
			if err := resp.Body.Close(); err != nil {
				panic(fmt.Sprintf("Failed to close response body: %v", err))
			}
		}()

		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.Equal(t, "MISS", resp.Header.Get("X-Cache"))
	})

	t.Run("second request - cache hit", func(t *testing.T) {
		resp2, err := client.Get(upstream.URL + "/test")
		if err != nil {
			panic(fmt.Sprintf("Request failed: %v", err))
		}
		defer func() {
			if err := resp2.Body.Close(); err != nil {
				panic(fmt.Sprintf("Failed to close response body: %v", err))
			}
		}()

		assert.Equal(t, "HIT", resp2.Header.Get("X-Cache"))
	})
}
