// Handles caching of HTTP responses
package cache

import (
	"crypto/sha256"
	"encoding/hex"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
)

// Manager handles caching operations
type Manager struct {
	cacheDir string
	ttl      time.Duration
}

// New creates a new cache manager
func New(cacheDir string, ttl time.Duration) *Manager {
	return &Manager{
		cacheDir: cacheDir,
		ttl:      ttl,
	}
}

// GetPath generates the cache file path for a request
func (m *Manager) GetPath(targetURL, method string) string {
	parsedURL, err := url.Parse(targetURL)
	if err != nil {
		return ""
	}

	// Create hash for query parameters to handle complex URLs
	hash := sha256.Sum256([]byte(parsedURL.RawQuery))
	queryHash := hex.EncodeToString(hash[:])[:8]

	// Build path: /cache_folder/host/path/METHOD[_queryhash].bin
	pathParts := []string{m.cacheDir, parsedURL.Host}

	if parsedURL.Path != "" && parsedURL.Path != "/" {
		pathParts = append(pathParts, strings.Trim(parsedURL.Path, "/"))
	}

	filename := method
	if parsedURL.RawQuery != "" {
		filename += "_" + queryHash
	}
	filename += ".bin"

	pathParts = append(pathParts, filename)

	return filepath.Join(pathParts...)
}

// Get retrieves cached response if it exists and is not expired
func (m *Manager) Get(cachePath string) ([]byte, bool) {
	if cachePath == "" {
		return nil, false
	}

	// Check if cache file exists and is not expired
	info, err := os.Stat(cachePath)
	if err != nil {
		return nil, false
	}

	if time.Since(info.ModTime()) > m.ttl {
		// Cache expired, remove it
		if err := os.Remove(cachePath); err != nil {
			logrus.Errorf("Failed to remove expired cache file %s: %v", cachePath, err)
		}
		return nil, false
	}

	// Read cached response
	data, err := os.ReadFile(cachePath)
	if err != nil {
		return nil, false
	}

	return data, true
}

// Set stores a response in the cache
func (m *Manager) Set(cachePath string, data []byte) error {
	if cachePath == "" {
		return nil
	}

	// Ensure directory exists
	dir := filepath.Dir(cachePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	// Write to cache
	if err := os.WriteFile(cachePath, data, 0644); err != nil {
		return err
	}

	logrus.Debugf("Cached response: %s", cachePath)
	return nil
}

// EnsureDir ensures the cache directory exists
func (m *Manager) EnsureDir() error {
	return os.MkdirAll(m.cacheDir, 0755)
}
