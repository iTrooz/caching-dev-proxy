package proxy

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"time"

	"caching-dev-proxy/internal/cache"
	"caching-dev-proxy/internal/config"

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
	cacheManager *cache.Manager
}

// New creates a new proxy server
func New(cfg *config.Config) (*Server, error) {
	cacheTTL, err := cfg.GetCacheTTL()
	if err != nil {
		return nil, fmt.Errorf("invalid cache TTL: %w", err)
	}

	cacheManager := cache.New(cfg.Cache.Folder, cacheTTL)

	return &Server{
		config:       cfg,
		cacheManager: cacheManager,
	}, nil
}

// Start starts the proxy server
func (s *Server) Start() error {
	// Ensure cache directory exists
	if err := s.cacheManager.EnsureDir(); err != nil {
		return fmt.Errorf("failed to create cache directory: %w", err)
	}

	http.HandleFunc("/", s.handleRequest)

	logrus.Infof("Starting caching proxy on port %d", s.config.Server.Port)
	logrus.Infof("Cache directory: %s", s.config.Cache.Folder)
	logrus.Infof("Cache TTL: %s", s.config.Cache.TTL)
	logrus.Infof("Rules mode: %s", s.config.Rules.Mode)

	return http.ListenAndServe(fmt.Sprintf(":%d", s.config.Server.Port), nil)
}

// HandleRequest handles HTTP requests (exported for testing)
func (s *Server) HandleRequest(w http.ResponseWriter, r *http.Request) {
	s.handleRequest(w, r)
}

func (s *Server) handleRequest(w http.ResponseWriter, r *http.Request) {
	// Handle CONNECT method for HTTPS tunneling
	if r.Method == http.MethodConnect {
		s.handleConnect(w, r)
		return
	}

	// Check if we have a cached response
	if s.isCached(r) {
		if s.serveCached(w, r) {
			return
		}
	}

	// Forward the request and get response
	resp, err := s.makeRequest(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}

	// Write response to client
	s.writeResponse(w, resp, false)

	// Cache the response if it should be cached
	if s.shouldBeCached(r, resp) {
		s.cacheResponse(r, resp)
	}

	targetURL := getTargetURL(r)
	logrus.Infof("Forwarded request: %s %s -> %d", r.Method, targetURL, resp.StatusCode)
}

func (s *Server) handleConnect(w http.ResponseWriter, r *http.Request) {
	// For HTTPS, we establish a tunnel
	destConn, err := net.Dial("tcp", r.Host)
	if err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}
	defer destConn.Close()

	w.WriteHeader(http.StatusOK)
	hijacker, ok := w.(http.Hijacker)
	if !ok {
		http.Error(w, "Hijacking not supported", http.StatusInternalServerError)
		return
	}

	clientConn, _, err := hijacker.Hijack()
	if err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}
	defer clientConn.Close()

	// Start bidirectional copying
	go func() {
		if _, err := io.Copy(destConn, clientConn); err != nil {
			logrus.Errorf("Error copying from client to destination: %v", err)
		}
	}()
	if _, err := io.Copy(clientConn, destConn); err != nil {
		logrus.Errorf("Error copying from destination to client: %v", err)
	}
}

// isCached checks if we should attempt to serve from cache
func (s *Server) isCached(r *http.Request) bool {
	targetURL := getTargetURL(r)
	cachePath := s.cacheManager.GetPath(targetURL, r.Method)

	// Check if cached file exists and is not expired
	_, found := s.cacheManager.Get(cachePath)
	return found
}

// writeResponse writes a ProxyResponse to the http.ResponseWriter
func (s *Server) writeResponse(w http.ResponseWriter, resp *ProxyResponse, cached bool) {
	// Copy response headers
	for key, values := range resp.Headers {
		for _, value := range values {
			w.Header().Add(key, value)
		}
	}

	// Set cache header
	if cached {
		w.Header().Set("X-Cache", "HIT")
	} else {
		w.Header().Set("X-Cache", "MISS")
	}

	w.WriteHeader(resp.StatusCode)
	if _, err := w.Write(resp.Body); err != nil {
		logrus.Errorf("Failed to write response body: %v", err)
	}
}

// shouldBeCached determines if a response should be cached based on rules
func (s *Server) shouldBeCached(r *http.Request, resp *ProxyResponse) bool {
	targetURL := getTargetURL(r)

	matched := false
	for _, rule := range s.config.Rules.Rules {
		if matchesRule(targetURL, r.Method, resp.StatusCode, rule) {
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

func (s *Server) serveCached(w http.ResponseWriter, r *http.Request) bool {
	targetURL := getTargetURL(r)
	cachePath := s.cacheManager.GetPath(targetURL, r.Method)

	data, found := s.cacheManager.Get(cachePath)
	if !found {
		return false
	}

	logrus.Infof("Serving from cache: %s", r.URL.String())

	// Set cache headers
	w.Header().Set("X-Cache", "HIT")
	w.Header().Set("X-Cache-File", cachePath)

	// Write cached response
	if _, err := w.Write(data); err != nil {
		logrus.Errorf("Failed to write cached response: %v", err)
	}
	return true
}

func (s *Server) makeRequest(r *http.Request) (*ProxyResponse, error) {
	targetURL := getTargetURL(r)

	// Create new request
	req, err := http.NewRequest(r.Method, targetURL, r.Body)
	if err != nil {
		return nil, err
	}

	// Copy headers
	for key, values := range r.Header {
		for _, value := range values {
			req.Header.Add(key, value)
		}
	}

	// Make the request
	client := &http.Client{
		Timeout: 30 * time.Second,
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	// Read response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	// Create proxy response
	proxyResp := &ProxyResponse{
		StatusCode: resp.StatusCode,
		Headers:    resp.Header,
		Body:       body,
	}

	return proxyResp, nil
}
