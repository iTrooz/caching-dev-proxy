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
		logrus.Errorf("Failed to generate certificate for hostname '%s': %v", hostname, err)
		return nil, fmt.Errorf("failed to generate certificate for hostname '%s': %w", hostname, err)
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
	cacheManager *cache.HTTPCache
	proxy        *goproxy.ProxyHttpServer
	rules        []Rule
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

func loadCertificate(cfg *config.Config) (*tls.Certificate, error) {
	if cfg.Server.SSLBumping.CACertFile == "" || cfg.Server.SSLBumping.CAKeyFile == "" {
		return nil, nil // Use default goproxy certificate
	}

	cert, err := tls.LoadX509KeyPair(cfg.Server.SSLBumping.CACertFile, cfg.Server.SSLBumping.CAKeyFile)
	if err != nil {
		return nil, fmt.Errorf("failed to load CA certificate and key: %w", err)
	}
	return &cert, nil
}

// setupProxyHandlers configures the goproxy handlers
func (s *Server) setupProxyHandlers() {
	// Handle CONNECT requests (HTTPS tunneling)
	logrus.Debugf("SSL bumping enabled: %v", s.config.Server.SSLBumping.Enabled)
	if s.config.Server.SSLBumping.Enabled {

		// Load CA certificate
		caCert, err := loadCertificate(s.config)
		if err != nil {
			logrus.Errorf("Failed to load CA certificate: %v", err)
			return
		}

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
				logrus.Debugf("Handling CONNECT request for %s", host)
				return customCaMitm, host
			})
			s.proxy.OnRequest().HandleConnect(customAlwaysMitm)
		}
	}
	// If SSL bumping is disabled, goproxy will handle CONNECT requests normally by default

	// Handle HTTP requests with caching
	s.proxy.OnRequest().DoFunc(func(req *http.Request, ctx *goproxy.ProxyCtx) (*http.Request, *http.Response) {
		logrus.Debugf("OnRequest()")
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
		logrus.Debugf("OnResponse()")
		if resp == nil || ctx.Req == nil {
			return resp
		}

		// Read response body for caching
		if resp.Body != nil {
			// Cache the response if it should be cached and it's not already a cache hit
			isCacheHit := resp.Header.Get("X-Cache") == "HIT"
			if !isCacheHit && s.shouldBeCached(ctx.Req, resp) {
				respCopy, err := copyResponse(resp)
				if err != nil {
					logrus.Errorf("Failed to copy response for caching: %v", err)
				} else {
					s.cacheResponse(ctx.Req, respCopy)
				}
			}

			// Add cache information header only if not already set (to avoid overwriting cache hits)
			if resp.Header.Get("X-Cache") == "" {
				if !s.shouldBeCached(ctx.Req, resp) {
					resp.Header.Set("X-Cache", "DISABLED")
				} else {
					resp.Header.Set("X-Cache", "MISS")
				}
			}

			logrus.Infof("Forwarded request: %s %s -> %d", ctx.Req.Method, ctx.Req.URL.String(), resp.StatusCode)
		}

		return resp
	})
}

// Start starts the proxy server
func (s *Server) Start() error {
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
func (s *Server) getCachedResponse(requ *http.Request) *http.Response {
	resp, err := s.cacheManager.Get(requ)
	// If cache lookup fails, only log as error if the request should be cached
	if err != nil {
		if s.shouldBeCached(requ, nil) {
			logrus.Errorf("Failed to get cached data for %s: %v", requ.URL, err)
		} else {
			logrus.Debugf("Cache not found for %s (caching disabled by rules)", requ.URL)
		}
		return nil
	}
	if resp == nil {
		logrus.Debugf("No cached data found for %s", requ.URL)
		return nil
	}

	resp.Header.Set("X-Cache", "HIT")

	return resp
}

// HandleRequest handles HTTP requests (exported for testing)
func (s *Server) HandleRequest(w http.ResponseWriter, requ *http.Request) {
	s.handleRequest(w, requ)
}

func (s *Server) handleRequest(w http.ResponseWriter, requ *http.Request) {
	// This method is kept for backward compatibility but is no longer used
	// The goproxy handlers now handle all requests
	logrus.Debugf("Legacy handleRequest called for: %s %s", requ.Method, requ.URL.String())
	http.Error(w, "This endpoint should not be called directly", http.StatusInternalServerError)
}

// isCached checks if we should attempt to serve from cache
func (s *Server) isCached(requ *http.Request) bool {
	// Check if cached file exists and is not expired
	data, err := s.cacheManager.Get(requ)
	if err != nil {
		logrus.Errorf("Failed to get cached data for %s: %v", requ.URL, err)
		return false
	}
	if data != nil {
		logrus.Debugf("Cache hit for %s", requ.URL)
		return true
	}
	return false
}

// shouldBeCached determines if a response should be cached based on rules
func (s *Server) shouldBeCached(requ *http.Request, resp *http.Response) bool {
	matched := false
	for _, rule := range s.rules {
		if rule.Match(requ, resp) {
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
func (s *Server) cacheResponse(requ *http.Request, resp *http.Response) {
	if err := s.cacheManager.Set(requ, resp); err != nil {
		logrus.Errorf("Failed to cache response for %s: %v", requ.URL.String(), err)
	}
}
