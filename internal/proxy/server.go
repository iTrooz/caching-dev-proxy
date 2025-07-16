package proxy

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"caching-dev-proxy/internal/cache"
	"caching-dev-proxy/internal/config"

	"github.com/sirupsen/logrus"
)

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

	// For HTTP requests, check if we should cache
	if s.shouldCache(r) {
		if s.serveCached(w, r) {
			return
		}
	}

	// Forward the request
	s.forwardRequest(w, r)
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

func (s *Server) shouldCache(r *http.Request) bool {
	targetURL := getTargetURL(r)

	matched := false
	for _, rule := range s.config.Rules.Rules {
		if matchesRule(targetURL, r.Method, rule) {
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

func matchesRule(targetURL, method string, rule config.CacheRule) bool {
	// Check if URL starts with base URI
	if !strings.HasPrefix(targetURL, rule.BaseURI) {
		return false
	}

	// Check if method matches
	for _, m := range rule.Methods {
		if strings.EqualFold(m, method) {
			return true
		}
	}

	return false
}

func getTargetURL(r *http.Request) string {
	if r.URL.IsAbs() {
		return r.URL.String()
	}

	// Reconstruct URL from Host header
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}

	return fmt.Sprintf("%s://%s%s", scheme, r.Host, r.URL.String())
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

func (s *Server) forwardRequest(w http.ResponseWriter, requ *http.Request) {
	targetURL := getTargetURL(requ)

	// Create new request
	req, err := http.NewRequest(requ.Method, targetURL, requ.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Copy headers
	for key, values := range requ.Header {
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
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	// Read response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Copy response headers
	for key, values := range resp.Header {
		for _, value := range values {
			w.Header().Add(key, value)
		}
	}

	w.Header().Set("X-Cache", "MISS")
	w.WriteHeader(resp.StatusCode)
	if _, err := w.Write(body); err != nil {
		logrus.Errorf("Failed to write response body: %v", err)
	}

	// Cache the response if it should be cached and status is OK
	if s.shouldCache(requ) && resp.StatusCode == http.StatusOK {
		cachePath := s.cacheManager.GetPath(targetURL, requ.Method)
		if err := s.cacheManager.Set(cachePath, body); err != nil {
			logrus.Errorf("Failed to cache response: %v", err)
		}
	}

	logrus.Infof("Forwarded request: %s %s -> %d", requ.Method, targetURL, resp.StatusCode)
}
