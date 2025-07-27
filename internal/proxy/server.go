package proxy

import (
	"bytes"
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/iTrooz/caching-dev-proxy/internal/cache"
	"github.com/iTrooz/caching-dev-proxy/internal/config"

	"github.com/elazarl/goproxy"
	"github.com/sirupsen/logrus"
)

// ProxyResponse holds the response data from upstream
type ProxyResponse struct {
	StatusCode int
	Headers    http.Header
	Body       []byte
}

// Server represents the caching proxy server
type Server struct {
	config       *config.Config
	cacheManager *cache.HTTPCache
	proxy        *goproxy.ProxyHttpServer
	rules        []Rule
}

// ctxUserData holds per-request context for cache logic
type ctxUserData struct {
	start  time.Time
	key    string
	bypass bool
	source string
}

// New creates a new proxy server
func New(cfg *config.Config) (*Server, error) {
	cacheTTL, err := cfg.GetCacheTTL()
	if err != nil {
		return nil, fmt.Errorf("invalid cache TTL: %w", err)
	}

	generic := cache.NewGenericDisk(cfg.Cache.Folder, cacheTTL)
	if err := generic.Init(); err != nil {
		return nil, fmt.Errorf("failed to create cache directory: %w", err)
	}
	cacheManager := cache.NewHTTP(generic)

	// Create goproxy instance
	proxy := &goproxy.ProxyHttpServer{
		Logger:  log.New(os.Stderr, "", log.LstdFlags),
		Tr:      &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, Proxy: http.ProxyFromEnvironment},
		Verbose: cfg.Log.ThirdParty,
		// Set up certificate storage for better performance during TLS interception
		CertStore: &simpleCertStore{certs: make(map[string]*tls.Certificate)},
	}
	proxy.NonproxyHandler = http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.Host == "" {
			http.Error(w, "Cannot handle requests without Host header, e.g., HTTP 1.0", http.StatusBadRequest)
			return
		}
		req.URL.Scheme = "http"
		req.URL.Host = req.Host
		// Set source
		req = req.WithContext(context.WithValue(req.Context(), ctxUserData{}, &ctxUserData{
			source: SrcHTTPTransparent,
		}))
		proxy.ServeHTTP(w, req)
	})

	// Convert config rules to Rule interfaces
	rules := make([]Rule, len(cfg.Rules.Rules))
	for i, rule := range cfg.Rules.Rules {
		rules[i] = &ConfigRule{CacheRule: rule}
	}

	server := &Server{
		config:       cfg,
		cacheManager: cacheManager,
		proxy:        proxy,
		rules:        rules,
	}

	// Configure goproxy handlers
	server.setupProxyHandlers()

	return server, nil
}

func copyResponse(resp *http.Response) (*http.Response, error) {
	bodyBytes, _ := io.ReadAll(resp.Body)
	err := resp.Body.Close()
	if err != nil {
		return nil, fmt.Errorf("failed to close response body: %w", err)
	}

	respCopy := *resp
	respCopy.Body = io.NopCloser(bytes.NewReader(bodyBytes))
	resp.Body = io.NopCloser(bytes.NewReader(bodyBytes))

	return &respCopy, nil
}

// setupProxyHandlers configures the goproxy handlers
func (s *Server) setupProxyHandlers() {
	// Handle CONNECT requests (HTTPS explicit proxying)
	if s.config.Server.HTTPS.Enabled {
		logrus.Debugf("TLS interception enabled: %v", s.config.Server.HTTPS.Enabled)
		s.setupHTTPSProxyHandler()
	}

	// Handle HTTP requests with caching
	s.proxy.OnRequest().DoFunc(func(req *http.Request, ctx *goproxy.ProxyCtx) (*http.Request, *http.Response) {
		// Start chrono
		start := time.Now()
		logrus.Debugf("OnRequest(url=%s)", req.URL.String())

		// Read user data
		userData, ok := ctx.UserData.(*ctxUserData)
		if !ok {
			// This is used for transparent HTTP proxying
			// _ is to avoid error
			userData, _ = req.Context().Value(ctxUserData{}).(*ctxUserData)
			ctx.UserData = userData
		}

		// Set user data for plain HTTP request if not set
		if userData == nil {
			userData = &ctxUserData{
				source: SrcHTTPExplicit,
			}
			ctx.UserData = userData
		}

		// Set chrono
		userData.start = start

		// X-Cache-Bypass: if present, skip cache entirely
		if req.Header.Get("X-Cache-Bypass") != "" {
			logrus.Debugf("OnRequest(url=%s): bypassing cache because of X-Cache-Bypass", req.URL.String())
			userData.bypass = true
			req.Header.Del("X-Cache-Bypass")
			return req, nil
		}

		// Generate cache key
		key, err := s.cacheManager.GenerateKey(req)
		if err != nil {
			logrus.Errorf("OnRequest(url=%s): Failed to generate cache key: %v", req.URL.String(), err)
			return req, nil
		}
		userData.key = key

		// Check if we have a cached response
		cachedResp, err := s.cacheManager.GetKey(key)
		if err != nil {
			logrus.Errorf("OnRequest(url=%s): Failed to get cached response: %v", req.URL.String(), err)
			return req, nil
		}
		if cachedResp != nil {
			logrus.Debugf("OnRequest(url=%s): Serving from cache", req.URL.String())
			cachedResp.Request = req
			cachedResp.Header.Set("X-Cache", "HIT")
			return req, cachedResp
		}

		// Continue with the request (will be handled by OnResponse)
		logrus.Debugf("OnRequest(url=%s): Querying upstream", req.URL.String())
		return req, nil
	})

	// Handle responses for caching
	s.proxy.OnResponse().DoFunc(func(resp *http.Response, ctx *goproxy.ProxyCtx) *http.Response {
		logrus.Debugf("OnResponse(url=%s)", ctx.Req.URL.String())
		if resp == nil || ctx.Req == nil {
			return resp
		}

		userData, ok := ctx.UserData.(*ctxUserData)
		if !ok {
			logrus.Errorf("OnResponse(url=%s): ctxUserData not found in UserData, cannot process response", ctx.Req.URL.String())
			return nil
		}

		// If X-Cache-Bypass was set, mark header and skip cache logic
		if userData.bypass {
			resp.Header.Set("X-Cache", "BYPASS")
		} else {
			// Cache the response if it should be cached and it's not already a cache hit
			isCacheHit := resp.Header.Get("X-Cache") == "HIT"
			if !isCacheHit && s.shouldBeCached(ctx.Req, resp) {
				respCopy, err := copyResponse(resp)
				if err != nil {
					logrus.Errorf("Onresponse(url=%s): Failed to copy response for caching: %v", ctx.Req.URL.String(), err)
				} else {
					if err := s.cacheManager.SetKey(userData.key, respCopy); err != nil {
						logrus.Errorf("OnResponse(url=%s): Failed to cache response: %v", ctx.Req.URL.String(), err)
					}
				}
			}

			// Add cache information header, only if not already set (to avoid overwriting cache hits)
			if resp.Header.Get("X-Cache") == "" {
				if s.shouldBeCached(ctx.Req, resp) {
					resp.Header.Set("X-Cache", "MISS")
				} else {
					resp.Header.Set("X-Cache", "DISABLED")
				}
			}
		}

		// See https://github.com/elazarl/goproxy/issues/696
		if err := ctx.Req.Body.Close(); err != nil {
			logrus.Errorf("Failed to close request body: %v", err)
		}

		// Last thing to do: check time taken
		end := time.Now()
		duration := end.Sub(userData.start)
		logrus.Infof("%s %v %v <- %v %v (%v)", userData.source, resp.StatusCode, resp.Header.Get("X-Cache"), ctx.Req.Method, ctx.Req.URL.String(), roundDuration(duration))

		return resp
	})
}

func roundDuration(d time.Duration) string {
	if d < time.Millisecond {
		return d.String()
	} else if d < time.Second {
		return d.Round(time.Millisecond).String()
	} else {
		return d.Round(time.Second).String()
	}
}

// Start starts the proxy server
func (s *Server) Start() error {
	logrus.Infof("Starting caching proxy at %v", s.config.Server.HTTP.Address)
	logrus.Debugf("Cache directory: %s", s.config.Cache.Folder)
	logrus.Debugf("Cache TTL: %s", s.config.Cache.TTL)
	logrus.Debugf("Rules mode: %s", s.config.Rules.Mode)
	if s.config.Server.HTTPS.Enabled {
		logrus.Debugf("TLS interception: enabled with CA certificate: %s", s.config.Server.HTTPS.CACertFile)
		// Enable transparent HTTPS proxying if configured
		if s.config.Server.HTTPS.Transparent.Address != "" {
			go s.StartTransparentHTTPS(s.config.Server.HTTPS.Transparent.Address)
			logrus.Infof("Transparent HTTPS proxying enabled at %s", s.config.Server.HTTPS.Transparent.Address)
		}
	} else {
		logrus.Debugf("TLS interception: disabled")
	}

	return http.ListenAndServe(s.config.Server.HTTP.Address, s.proxy)
}

// GetProxy returns the underlying goproxy instance for testing
func (s *Server) GetProxy() *goproxy.ProxyHttpServer {
	return s.proxy
}
