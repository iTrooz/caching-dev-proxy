package cache

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
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
func (d *DiskCache) GetKey(targetURL, method string) (string, error) {
	parsedURL, err := url.Parse(targetURL)
	if err != nil {
		logrus.Warnf("Failed to parse URL '%s': %v", targetURL, err)
		return "", fmt.Errorf("failed to parse URL '%s': %w", targetURL, err)
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

	return filepath.Join(pathParts...), nil
}

func (d *DiskCache) Get(cachePath string) ([]byte, error) {
	if cachePath == "" {
		return nil, fmt.Errorf("cache path cannot be empty")
	}

	// Check if cache file exists and is not expired
	info, err := os.Stat(cachePath)
	if err != nil {
		if !os.IsNotExist(err) {
			logrus.Debugf("DiskCache::Get(%s) = NotFound", cachePath)
			return nil, nil
		}
		return nil, fmt.Errorf("cache file stat error for %s: %w", cachePath, err)
	}

	if time.Since(info.ModTime()) > d.ttl {
		// Cache expired, remove it
		if err := os.Remove(cachePath); err != nil {
			// Do not return error because removing an expired cache file is not critical for Get()
			logrus.Errorf("Failed to remove expired cache file %s: %v", cachePath, err)
		}
		return nil, nil
	}

	// Read cached response
	data, err := os.ReadFile(cachePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read cache file '%s': %w", cachePath, err)
	}

	return data, nil
}

// Set stores a response in the cache
func (d *DiskCache) Set(cachePath string, data []byte) error {
	if cachePath == "" {
		return fmt.Errorf("cache path cannot be empty")
	}

	// Ensure directory exists
	dir := filepath.Dir(cachePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create cache directory: %w", err)
	}

	// Write to cache
	if err := os.WriteFile(cachePath, data, 0644); err != nil {
		return fmt.Errorf("failed to write cache file: %w", err)
	}

	logrus.Debugf("Cached response: %s", cachePath)
	return nil
}

// Init ensures the cache directory exists
func (d *DiskCache) Init() error {
	if err := os.MkdirAll(d.cacheDir, 0755); err != nil {
		return fmt.Errorf("failed to create cache directory: %w", err)
	}
	return nil
}
