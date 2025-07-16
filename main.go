package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Server struct {
		Port int `yaml:"port"`
	} `yaml:"server"`
	Cache struct {
		TTL    string `yaml:"ttl"`
		Folder string `yaml:"folder"`
	} `yaml:"cache"`
	Rules struct {
		Mode  string      `yaml:"mode"` // "whitelist" or "blacklist"
		Rules []CacheRule `yaml:"rules"`
	} `yaml:"rules"`
}

type CacheRule struct {
	BaseURI string   `yaml:"base_uri"`
	Methods []string `yaml:"methods"`
}

type CachingProxy struct {
	config   Config
	cacheTTL time.Duration
}

func main() {
	configPath := "config.yaml"
	if len(os.Args) > 1 {
		configPath = os.Args[1]
	}

	config, err := loadConfig(configPath)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	cacheTTL, err := time.ParseDuration(config.Cache.TTL)
	if err != nil {
		log.Fatalf("Invalid cache TTL format: %v", err)
	}

	proxy := &CachingProxy{
		config:   config,
		cacheTTL: cacheTTL,
	}

	// Ensure cache directory exists
	if err := os.MkdirAll(config.Cache.Folder, 0755); err != nil {
		log.Fatalf("Failed to create cache directory: %v", err)
	}

	http.HandleFunc("/", proxy.handleRequest)

	port := config.Server.Port
	if port == 0 {
		port = 8080
	}

	log.Printf("Starting caching proxy on port %d", port)
	log.Printf("Cache directory: %s", config.Cache.Folder)
	log.Printf("Cache TTL: %s", config.Cache.TTL)
	log.Printf("Rules mode: %s", config.Rules.Mode)

	if err := http.ListenAndServe(fmt.Sprintf(":%d", port), nil); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}

func loadConfig(path string) (Config, error) {
	var config Config

	data, err := os.ReadFile(path)
	if err != nil {
		return config, fmt.Errorf("reading config file: %w", err)
	}

	if err := yaml.Unmarshal(data, &config); err != nil {
		return config, fmt.Errorf("parsing config YAML: %w", err)
	}

	return config, nil
}

func (p *CachingProxy) handleRequest(w http.ResponseWriter, r *http.Request) {
	// Handle CONNECT method for HTTPS tunneling
	if r.Method == http.MethodConnect {
		p.handleConnect(w, r)
		return
	}

	// For HTTP requests, check if we should cache
	if p.shouldCache(r) {
		if p.serveCached(w, r) {
			return
		}
	}

	// Forward the request
	p.forwardRequest(w, r)
}

func (p *CachingProxy) handleConnect(w http.ResponseWriter, r *http.Request) {
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
	go io.Copy(destConn, clientConn)
	io.Copy(clientConn, destConn)
}

func (p *CachingProxy) shouldCache(r *http.Request) bool {
	// Only cache GET requests for now
	if r.Method != http.MethodGet {
		return false
	}

	targetURL := p.getTargetURL(r)

	// Check rules
	for _, rule := range p.config.Rules.Rules {
		if p.matchesRule(targetURL, r.Method, rule) {
			if p.config.Rules.Mode == "whitelist" {
				return true
			} else if p.config.Rules.Mode == "blacklist" {
				return false
			}
		}
	}

	// Default behavior
	if p.config.Rules.Mode == "whitelist" {
		return false // Not in whitelist
	} else {
		return true // Not in blacklist
	}
}

func (p *CachingProxy) matchesRule(targetURL, method string, rule CacheRule) bool {
	// Check if URL starts with base URI
	if !strings.HasPrefix(targetURL, rule.BaseURI) {
		return false
	}

	// Check if method matches
	for _, m := range rule.Methods {
		if strings.ToUpper(m) == strings.ToUpper(method) {
			return true
		}
	}

	return false
}

func (p *CachingProxy) getTargetURL(r *http.Request) string {
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

func (p *CachingProxy) getCachePath(r *http.Request) string {
	targetURL := p.getTargetURL(r)

	parsedURL, err := url.Parse(targetURL)
	if err != nil {
		return ""
	}

	// Create hash for query parameters to handle complex URLs
	hash := sha256.Sum256([]byte(parsedURL.RawQuery))
	queryHash := hex.EncodeToString(hash[:])[:8]

	// Build path: /cache_folder/host/path/METHOD[_queryhash].bin
	pathParts := []string{p.config.Cache.Folder, parsedURL.Host}

	if parsedURL.Path != "" && parsedURL.Path != "/" {
		pathParts = append(pathParts, strings.Trim(parsedURL.Path, "/"))
	}

	filename := r.Method
	if parsedURL.RawQuery != "" {
		filename += "_" + queryHash
	}
	filename += ".bin"

	pathParts = append(pathParts, filename)

	return filepath.Join(pathParts...)
}

func (p *CachingProxy) serveCached(w http.ResponseWriter, r *http.Request) bool {
	cachePath := p.getCachePath(r)
	if cachePath == "" {
		return false
	}

	// Check if cache file exists and is not expired
	info, err := os.Stat(cachePath)
	if err != nil {
		return false
	}

	if time.Since(info.ModTime()) > p.cacheTTL {
		// Cache expired, remove it
		os.Remove(cachePath)
		return false
	}

	// Serve from cache
	data, err := os.ReadFile(cachePath)
	if err != nil {
		return false
	}

	log.Printf("Serving from cache: %s", r.URL.String())

	// Set cache headers
	w.Header().Set("X-Cache", "HIT")
	w.Header().Set("X-Cache-File", cachePath)

	// Write cached response
	w.Write(data)
	return true
}

func (p *CachingProxy) forwardRequest(w http.ResponseWriter, r *http.Request) {
	targetURL := p.getTargetURL(r)

	// Create new request
	req, err := http.NewRequest(r.Method, targetURL, r.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
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
	w.Write(body)

	// Cache the response if it should be cached and status is OK
	if p.shouldCache(r) && resp.StatusCode == http.StatusOK {
		p.cacheResponse(r, body)
	}

	log.Printf("Forwarded request: %s %s -> %d", r.Method, targetURL, resp.StatusCode)
}

func (p *CachingProxy) cacheResponse(r *http.Request, body []byte) {
	cachePath := p.getCachePath(r)
	if cachePath == "" {
		return
	}

	// Ensure directory exists
	dir := filepath.Dir(cachePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		log.Printf("Failed to create cache directory %s: %v", dir, err)
		return
	}

	// Write to cache
	if err := os.WriteFile(cachePath, body, 0644); err != nil {
		log.Printf("Failed to cache response to %s: %v", cachePath, err)
		return
	}

	log.Printf("Cached response: %s", cachePath)
}
