package tests

import (
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"

	"caching-dev-proxy/internal/config"
)

func init() {
	logrus.SetLevel(logrus.DebugLevel)
}

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
			panic(err)
		}

		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.Equal(t, "MISS", resp.Header.Get("X-Cache"))

		body := helper_readBodyAndClose(resp)
		assert.Contains(t, body, "Hello from upstream")
	})

	// Test second request (should hit cache)
	t.Run("second request - cache hit", func(t *testing.T) {
		resp, err := client.Get(upstream.URL + "/test")
		if err != nil {
			panic(err)
		}

		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.Equal(t, "HIT", resp.Header.Get("X-Cache"))

		body := helper_readBodyAndClose(resp)
		assert.Contains(t, body, "Hello from upstream")
	})

	// Verify cache file was created
	t.Run("verify cache file exists", func(t *testing.T) {
		upstreamURL, err := url.Parse(upstream.URL)
		if err != nil {
			panic(err)
		}
		expectedCachePath := filepath.Join(tempDir, upstreamURL.Host, "test", "GET.bin")

		_, err = os.Stat(expectedCachePath)
		assert.NoError(t, err, "Cache file should exist at %s", expectedCachePath)
	})
}

// Host is not in blacklist, should return MISS/HIT
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
			panic(err)
		}
		helper_readBodyAndClose(resp)

		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.Equal(t, "MISS", resp.Header.Get("X-Cache"))
	})

	t.Run("second request - cache hit", func(t *testing.T) {
		resp2, err := client.Get(upstream.URL + "/test")
		if err != nil {
			panic(err)
		}
		helper_readBodyAndClose(resp2)

		assert.Equal(t, "HIT", resp2.Header.Get("X-Cache"))
	})
}

// Host is in whitelist, should return MISS/HIT
func TestHitWithWhitelist(t *testing.T) {
	// Create a test upstream server
	upstream := fixture_upstream()
	defer upstream.Close()

	// Create config with whitelist rules that match our upstream URL
	customRules := config.NewRulesConfig(config.RulesModeWhitelist,
		config.NewCacheRule(upstream.URL, "GET"),
	)
	cfg := fixture_config(t.TempDir(), customRules)

	// Create proxy server
	_, proxyTestServer, client := fixture_proxy(cfg)
	defer proxyTestServer.Close()

	// Test that requests are cached (since we're using whitelist mode and the upstream URL is in the whitelist)
	t.Run("first request - cache miss", func(t *testing.T) {
		resp, err := client.Get(upstream.URL + "/test")
		if err != nil {
			panic(err)
		}
		helper_readBodyAndClose(resp)

		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.Equal(t, "MISS", resp.Header.Get("X-Cache"))
	})

	t.Run("second request - cache hit", func(t *testing.T) {
		resp2, err := client.Get(upstream.URL + "/test")
		if err != nil {
			panic(err)
		}
		helper_readBodyAndClose(resp2)

		assert.Equal(t, "HIT", resp2.Header.Get("X-Cache"))
	})
}

// Host is not in whitelist, should return DISABLED
func TestMissWithWhitelist(t *testing.T) {
	// Create a test upstream server
	upstream := fixture_upstream()
	defer upstream.Close()

	// Create config with whitelist rules that DON'T match our upstream URL
	// This should result in requests NOT being cached
	customRules := config.NewRulesConfig(config.RulesModeWhitelist,
		config.NewCacheRule("https://example.com", "GET"), // Different URL than upstream
	)
	cfg := fixture_config(t.TempDir(), customRules)

	// Create proxy server
	_, proxyTestServer, client := fixture_proxy(cfg)
	defer proxyTestServer.Close()

	// Test that requests are NOT cached (since upstream URL is not in whitelist)
	t.Run("first request - not cached", func(t *testing.T) {
		resp, err := client.Get(upstream.URL + "/test")
		if err != nil {
			panic(err)
		}
		helper_readBodyAndClose(resp)

		assert.Equal(t, http.StatusOK, resp.StatusCode)
		// Should have X-Cache: DISABLED since it's not being cached
		assert.Equal(t, "DISABLED", resp.Header.Get("X-Cache"))
	})

	t.Run("second request - still not cached", func(t *testing.T) {
		resp2, err := client.Get(upstream.URL + "/test")
		if err != nil {
			panic(err)
		}
		helper_readBodyAndClose(resp2)

		assert.Equal(t, http.StatusOK, resp2.StatusCode)
		// Should still have X-Cache: DISABLED since it's not being cached
		assert.Equal(t, "DISABLED", resp2.Header.Get("X-Cache"))
	})
}

// Host is in blacklist, should return DISABLED
func TestMissWithBlacklist(t *testing.T) {
	// Create a test upstream server
	upstream := fixture_upstream()
	defer upstream.Close()

	// Create config with blacklist rules that DO match our upstream URL
	// This should result in requests NOT being cached
	customRules := config.NewRulesConfig(config.RulesModeBlacklist,
		config.NewCacheRule(upstream.URL, "GET"), // Same URL as upstream
	)
	cfg := fixture_config(t.TempDir(), customRules)

	// Create proxy server
	_, proxyTestServer, client := fixture_proxy(cfg)
	defer proxyTestServer.Close()

	// Test that requests are NOT cached (since upstream URL is in blacklist)
	t.Run("first request - not cached", func(t *testing.T) {
		resp, err := client.Get(upstream.URL + "/test")
		if err != nil {
			panic(err)
		}
		helper_readBodyAndClose(resp)

		assert.Equal(t, http.StatusOK, resp.StatusCode)
		// Should have X-Cache: DISABLED since it's not being cached
		assert.Equal(t, "DISABLED", resp.Header.Get("X-Cache"))
	})

	t.Run("second request - still not cached", func(t *testing.T) {
		resp2, err := client.Get(upstream.URL + "/test")
		if err != nil {
			panic(err)
		}
		helper_readBodyAndClose(resp2)

		assert.Equal(t, http.StatusOK, resp2.StatusCode)
		// Should still have X-Cache: DISABLED since it's not being cached
		assert.Equal(t, "DISABLED", resp2.Header.Get("X-Cache"))
	})
}

// Test that upstream gets no connection at all when cache hit occurs
// (Unlike squid, which will initialize a TCP connection to upstream even on requests with cache hits)
func TestNoUpstreamConnectionOnCacheHitHTTP(t *testing.T) {
	upstream := fixture_upstream()
	upstreamURL := upstream.URL

	cfg := fixture_config(t.TempDir(), nil)
	_, proxyTestServer, client := fixture_proxy(cfg)
	defer proxyTestServer.Close()

	proxyURL, err := url.Parse(proxyTestServer.URL)
	if err != nil {
		panic(err)
	}
	client.Transport = &http.Transport{
		Proxy: http.ProxyURL(proxyURL),
	}

	// First request: should hit upstream and cache
	t.Run("cache miss - upstream connection", func(t *testing.T) {
		resp, err := client.Get(upstreamURL + "/test")
		if err != nil {
			panic(err)
		}
		helper_readBodyAndClose(resp)
		assert.Equal(t, "MISS", resp.Header.Get("X-Cache"))
	})

	upstream.Close() // Close after first request
	// Second: setup raw TCP server for cache hit
	parsedURL, _ := url.Parse(upstreamURL)
	closeTcp := fixture_tcp_upstream(t, parsedURL.Host)

	// Second request: should hit cache, no upstream connection
	t.Run("cache hit - no upstream connection", func(t *testing.T) {
		resp2, err := client.Get(upstreamURL + "/test")
		if err != nil {
			panic(err)
		}
		helper_readBodyAndClose(resp2)
		assert.Equal(t, "HIT", resp2.Header.Get("X-Cache"))

		connCount := closeTcp()
		if connCount != 0 {
			t.Fatalf("Expected no TCP connection to upstream on cache hit, got %d", connCount)
		}
	})
}

// Same as above, but for HTTPS requests
func TestNoUpstreamConnectionOnCacheHitHTTPS(t *testing.T) {
	upstream := fixture_upstream_tls()
	upstreamURL := upstream.URL

	cfg := fixture_config(t.TempDir(), nil)
	_, proxyTestServer, client := fixture_proxy(cfg)
	defer proxyTestServer.Close()

	// First request: should hit upstream and cache
	t.Run("cache miss - upstream connection", func(t *testing.T) {
		resp, err := client.Get(upstreamURL + "/test")
		if err != nil {
			panic(err)
		}
		helper_readBodyAndClose(resp)
		assert.Equal(t, "MISS", resp.Header.Get("X-Cache"))
	})

	upstream.Close() // Close after first request
	// Second: setup raw TCP server for cache hit
	parsedURL, _ := url.Parse(upstreamURL)
	closeTcp := fixture_tcp_upstream(t, parsedURL.Host)

	// Second request: should hit cache, no upstream connection
	t.Run("cache hit - no upstream connection", func(t *testing.T) {
		resp2, err := client.Get(upstreamURL + "/test")
		if err != nil {
			panic(err)
		}
		_ = resp2.Body.Close()
		assert.Equal(t, "HIT", resp2.Header.Get("X-Cache"))

		connCount := closeTcp()
		if connCount != 0 {
			t.Fatalf("Expected no TCP connection to upstream on cache hit, got %d", connCount)
		}
	})
}
