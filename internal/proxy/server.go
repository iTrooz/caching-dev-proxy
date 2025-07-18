package proxy

import (
	"bytes"
	"crypto/tls"
	"fmt"
	"io"
	"net/http"

	"caching-dev-proxy/internal/cache"
	"caching-dev-proxy/internal/config"

	"github.com/elazarl/goproxy"
	"github.com/sirupsen/logrus"
)

// simpleCertStore implements goproxy.CertStorage for certificate caching
type simpleCertStore struct {
	certs map[string]*tls.Certificate
}

func (s *simpleCertStore) Fetch(hostname string, gen func() (*tls.Certificate, error)) (*tls.Certificate, error) {
	cert, ok := s.certs[hostname]
	if ok {
		return cert, nil
	}

	cert, err := gen()
	if err != nil {
		return nil, err
	}

	s.certs[hostname] = cert
	return cert, nil
}

// ProxyResponse holds the response data from upstream
type ProxyResponse struct {
	StatusCode int
	Headers    http.Header
	Body       []byte
}

// Server represents the caching proxy server
type Server struct {
	config       *config.Config
	cacheManager *cache.Manager
	proxy        *goproxy.ProxyHttpServer
	rules        []Rule
}

// New creates a new proxy server
func New(cfg *config.Config) (*Server, error) {
	cacheTTL, err := cfg.GetCacheTTL()
	if err != nil {
		return nil, fmt.Errorf("invalid cache TTL: %w", err)
	}

	cacheManager := cache.New(cfg.Cache.Folder, cacheTTL)

	// Create goproxy instance
	proxy := goproxy.NewProxyHttpServer()
	proxy.Verbose = cfg.Log.ThirdParty

	// Set up certificate storage for better performance during SSL bumping
	proxy.CertStore = &simpleCertStore{certs: make(map[string]*tls.Certificate)}

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

	// Load CA certificate if SSL bumping is enabled
	var caCert *tls.Certificate
	if cfg.Server.SSLBumping.Enabled {
		cert, err := tls.LoadX509KeyPair(cfg.Server.SSLBumping.CACertFile, cfg.Server.SSLBumping.CAKeyFile)
		if err != nil {
			return nil, fmt.Errorf("failed to load CA certificate and key: %w", err)
		}
		caCert = &cert
	}

	// Configure goproxy handlers
	server.setupProxyHandlers(caCert)

	return server, nil
}

// setupProxyHandlers configures the goproxy handlers
func (s *Server) setupProxyHandlers(caCert *tls.Certificate) {
	// Handle CONNECT requests (HTTPS tunneling)
	if s.config.Server.SSLBumping.Enabled {
		if caCert == nil {
			// Use goproxy's default certificate
			logrus.Warnf("SSL bumping enabled but no CA certificate loaded, using goproxy default certificate")
			s.proxy.OnRequest().HandleConnect(goproxy.AlwaysMitm)
		} else {
			// Make goproxy use our provided CA certificate
			customCaMitm := &goproxy.ConnectAction{
				Action:    goproxy.ConnectMitm,
				TLSConfig: goproxy.TLSConfigFromCA(caCert),
			}
			customAlwaysMitm := goproxy.FuncHttpsHandler(func(host string, ctx *goproxy.ProxyCtx) (*goproxy.ConnectAction, string) {
				return customCaMitm, host
			})
			s.proxy.OnRequest().HandleConnect(customAlwaysMitm)
		}
	}
	// If SSL bumping is disabled, goproxy will handle CONNECT requests normally by default

	// Handle HTTP requests with caching
	s.proxy.OnRequest().DoFunc(func(req *http.Request, ctx *goproxy.ProxyCtx) (*http.Request, *http.Response) {
		logrus.Debugf("Received request: %s %s", req.Method, req.URL.String())

		// Check if we have a cached response
		if s.isCached(req) {
			if cachedResp := s.getCachedResponse(req); cachedResp != nil {
				logrus.Infof("Serving from cache: %s", req.URL.String())
				return req, cachedResp
			}
		}

		// Continue with the request (will be handled by OnResponse)
		return req, nil
	})

	// Handle responses for caching
	s.proxy.OnResponse().DoFunc(func(resp *http.Response, ctx *goproxy.ProxyCtx) *http.Response {
		if resp == nil || ctx.Req == nil {
			return resp
		}

		// Read response body for caching
		if resp.Body != nil {
			bodyBytes, err := io.ReadAll(resp.Body)
			if err != nil {
				logrus.Errorf("Failed to read response body: %v", err)
				return resp
			}
			if err := resp.Body.Close(); err != nil {
				logrus.Errorf("Failed to close response body: %v", err)
			}

			// Create new response body
			resp.Body = io.NopCloser(bytes.NewReader(bodyBytes))

			// Create ProxyResponse for caching logic
			proxyResp := &ProxyResponse{
				StatusCode: resp.StatusCode,
				Headers:    resp.Header,
				Body:       bodyBytes,
			}

			// Cache the response if it should be cached
			if s.shouldBeCached(ctx.Req, proxyResp) {
				s.cacheResponse(ctx.Req, proxyResp)
			}

			// Add cache header only if not already set (to avoid overwriting cache hits)
			if resp.Header.Get("X-Cache") == "" {
				resp.Header.Set("X-Cache", "MISS")
			}

			logrus.Infof("Forwarded request: %s %s -> %d", ctx.Req.Method, ctx.Req.URL.String(), resp.StatusCode)
		}

		return resp
	})
}

// Start starts the proxy server
func (s *Server) Start() error {
	// Ensure cache directory exists
	if err := s.cacheManager.EnsureDir(); err != nil {
		return fmt.Errorf("failed to create cache directory: %w", err)
	}

	logrus.Infof("Starting caching proxy on port %d", s.config.Server.Port)
	logrus.Infof("Cache directory: %s", s.config.Cache.Folder)
	logrus.Infof("Cache TTL: %s", s.config.Cache.TTL)
	logrus.Infof("Rules mode: %s", s.config.Rules.Mode)
	if s.config.Server.SSLBumping.Enabled {
		logrus.Infof("SSL bumping: enabled with CA certificate: %s", s.config.Server.SSLBumping.CACertFile)
	} else {
		logrus.Infof("SSL bumping: disabled")
	}

	return http.ListenAndServe(fmt.Sprintf(":%d", s.config.Server.Port), s.proxy)
}

// GetProxy returns the underlying goproxy instance for testing
func (s *Server) GetProxy() *goproxy.ProxyHttpServer {
	return s.proxy
}

// getCachedResponse returns a cached HTTP response if available
func (s *Server) getCachedResponse(r *http.Request) *http.Response {
	targetURL := getTargetURL(r)
	cachePath := s.cacheManager.GetPath(targetURL, r.Method)

	data, found := s.cacheManager.Get(cachePath)
	if !found {
		return nil
	}

	// Create HTTP response from cached data
	resp := &http.Response{
		Status:        "200 OK",
		StatusCode:    200,
		Proto:         "HTTP/1.1",
		ProtoMajor:    1,
		ProtoMinor:    1,
		Header:        make(http.Header),
		Body:          io.NopCloser(bytes.NewReader(data)),
		ContentLength: int64(len(data)),
		Request:       r,
	}

	// Set cache headers
	resp.Header.Set("X-Cache", "HIT")
	resp.Header.Set("X-Cache-File", cachePath)
	resp.Header.Set("Content-Type", "text/html; charset=utf-8") // Default content type

	return resp
}

// HandleRequest handles HTTP requests (exported for testing)
func (s *Server) HandleRequest(w http.ResponseWriter, r *http.Request) {
	s.handleRequest(w, r)
}

func (s *Server) handleRequest(w http.ResponseWriter, r *http.Request) {
	// This method is kept for backward compatibility but is no longer used
	// The goproxy handlers now handle all requests
	logrus.Debugf("Legacy handleRequest called for: %s %s", r.Method, r.URL.String())
	http.Error(w, "This endpoint should not be called directly", http.StatusInternalServerError)
}

// isCached checks if we should attempt to serve from cache
func (s *Server) isCached(r *http.Request) bool {
	targetURL := getTargetURL(r)
	cachePath := s.cacheManager.GetPath(targetURL, r.Method)

	// Check if cached file exists and is not expired
	_, found := s.cacheManager.Get(cachePath)
	return found
}

// shouldBeCached determines if a response should be cached based on rules
func (s *Server) shouldBeCached(r *http.Request, resp *ProxyResponse) bool {
	targetURL := getTargetURL(r)

	matched := false
	for _, rule := range s.rules {
		if rule.Match(targetURL, r.Method, resp.StatusCode) {
			matched = true
			break
		}
	}

	if s.config.Rules.Mode == "whitelist" {
		return matched
	} else {
		return !matched
	}
}

// cacheResponse stores a response in the cache
func (s *Server) cacheResponse(r *http.Request, resp *ProxyResponse) {
	targetURL := getTargetURL(r)
	cachePath := s.cacheManager.GetPath(targetURL, r.Method)
	if err := s.cacheManager.Set(cachePath, resp.Body); err != nil {
		logrus.Errorf("Failed to cache response: %v", err)
	}
}
