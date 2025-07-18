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

// DiskCache implements Cache interface for disk-based caching
type DiskCache struct {
	cacheDir string
	ttl      time.Duration
}

// NewDisk creates a new disk cache
func NewDisk(cacheDir string, ttl time.Duration) Cache {
	return &DiskCache{
		cacheDir: cacheDir,
		ttl:      ttl,
	}
}

// GetKey generates the cache file path for a request
func (d *DiskCache) GetKey(targetURL, method string) string {
	parsedURL, err := url.Parse(targetURL)
	if err != nil {
		return ""
	}

	// Create hash for query parameters to handle complex URLs
	hash := sha256.Sum256([]byte(parsedURL.RawQuery))
	queryHash := hex.EncodeToString(hash[:])[:8]

	// Build path: /cache_folder/host/path/METHOD[_queryhash].bin
	pathParts := []string{d.cacheDir, parsedURL.Host}

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
func (d *DiskCache) Get(cachePath string) ([]byte, bool) {
	if cachePath == "" {
		return nil, false
	}

	// Check if cache file exists and is not expired
	info, err := os.Stat(cachePath)
	if err != nil {
		return nil, false
	}

	if time.Since(info.ModTime()) > d.ttl {
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
func (d *DiskCache) Set(cachePath string, data []byte) error {
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

// Init ensures the cache directory exists
func (d *DiskCache) Init() error {
	return os.MkdirAll(d.cacheDir, 0755)
}
