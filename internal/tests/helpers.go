package tests

import (
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"caching-dev-proxy/internal/config"
	"caching-dev-proxy/internal/proxy"

	"github.com/sirupsen/logrus"
)

// helper_readBodyAndClose reads the response body and closes it, panicking on any errors
func helper_readBodyAndClose(resp *http.Response) string {
	defer func() {
		if err := resp.Body.Close(); err != nil {
			panic(fmt.Sprintf("Failed to close response body: %v", err))
		}
	}()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		panic(fmt.Sprintf("Failed to read response body: %v", err))
	}

	return string(body)
}

func fixture_tcp_upstream(t *testing.T, address string) func() int {
	tcpLn, err := net.Listen("tcp", address)
	if err != nil {
		t.Fatalf("Failed to start raw TCP listener: %v", err)
	}

	connCount := 0
	go func() {
		for {
			conn, err := tcpLn.Accept()
			if err != nil {
				// Listener is closed
				if errors.Is(err, net.ErrClosed) {
					break
				} else {
					logrus.Warnf("TCP listener closed unexpectedly: %v", err)
					continue
				}
			}
			connCount++
			if err := conn.Close(); err != nil {
				panic(err)
			}
		}
	}()

	return func() int {
		err := tcpLn.Close()
		if err != nil {
			panic(err)
		}
		return connCount
	}
}

// fixture_upstream creates a test upstream server
func fixture_upstream() *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, requ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"message": "Hello from upstream", "path": "` + requ.URL.Path + `"}`))
	}))
}

func fixture_upstream_tls() *httptest.Server {
	return httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		if _, err := fmt.Fprintf(w, `{"message": "Hello from upstream", "path": "%s"}`, r.URL.Path); err != nil {
			panic(fmt.Sprintf("Failed to write response: %v", err))
		}
	}))
}

// fixture_config creates a test config with optional rules
func fixture_config(tempDir string, rules *config.RulesConfig) *config.Config {
	cfg := &config.Config{
		Server: config.ServerConfig{
			Port: 0,
			TLS: config.TLSConfig{
				Enabled: true,
			},
		},
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
func fixture_proxy(cfg *config.Config) (*proxy.Server, *httptest.Server, *http.Client) {
	proxyServer, err := proxy.New(cfg)
	if err != nil {
		panic(fmt.Errorf("failed to create proxy server: %w", err))
	}

	// Create test proxy HTTP server using goproxy
	proxyTestServer := httptest.NewServer(proxyServer.GetProxy())

	// Create HTTP client that uses our proxy
	proxyURL, _ := url.Parse(proxyTestServer.URL)
	client := &http.Client{
		Transport: &http.Transport{
			Proxy:           http.ProxyURL(proxyURL),
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, // For testing HTTPS
		},
		Timeout: 10 * time.Second,
	}

	return proxyServer, proxyTestServer, client
}
