package cache

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/sirupsen/logrus"
)

// DiskCache implements Cache interface for disk-based caching
type DiskCache struct {
	cacheDir string
	ttl      time.Duration
}

// NewGenericDisk creates a new disk cache
func NewGenericDisk(cacheDir string, ttl time.Duration) GenericCache {
	return &DiskCache{
		cacheDir: cacheDir,
		ttl:      ttl,
	}
}

func (d *DiskCache) Get(cacheKey string) ([]byte, error) {
	logrus.Debugf("DiskCache::Get(%s)", cacheKey)
	if cacheKey == "" {
		return nil, fmt.Errorf("cache path cannot be empty")
	}
	fullPath := filepath.Join(d.cacheDir, cacheKey)

	// Check if cache file exists and is not expired
	info, err := os.Stat(fullPath)
	if err != nil {
		if os.IsNotExist(err) {
			// Cache file does not exist: this is a cache miss, not an error
			logrus.Debugf("DiskCache::Get(%s): Not found", cacheKey)
			return nil, nil
		}
		logrus.Debugf("DiskCache::Get(%s): Error checking file: %v", cacheKey, err)
		return nil, fmt.Errorf("cache file stat error for %s: %w", fullPath, err)
	}

	// check TTL (0 means infinity)
	if d.ttl != 0 && time.Since(info.ModTime()) > d.ttl {
		logrus.Debugf("Cache expired for %s (ttl was %s), removing", cacheKey, d.ttl)
		// Cache expired, remove it
		if err := os.Remove(fullPath); err != nil {
			// Do not return error because removing an expired cache file is not critical for Get()
			logrus.Warnf("Failed to remove expired cache file %s: %v", fullPath, err)
		}
		return nil, nil
	}

	// Read cached response
	data, err := os.ReadFile(fullPath)
	if err != nil {
		logrus.Debugf("DiskCache::Get(%s): Failed to read cache file: %v", cacheKey, err)
		return nil, fmt.Errorf("failed to read cache file '%s': %w", fullPath, err)
	}

	logrus.Debugf("DiskCache::Get(%s): Cache hit", cacheKey)
	return data, nil
}

// Set stores a response in the cache
func (d *DiskCache) Set(cacheKey string, data []byte) error {
	if cacheKey == "" {
		return fmt.Errorf("cache path cannot be empty")
	}

	// Ensure directory exists
	fullpath := filepath.Join(d.cacheDir, cacheKey)
	dir := filepath.Dir(fullpath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create cache directory: %w", err)
	}

	// Write to cache
	if err := os.WriteFile(fullpath, data, 0644); err != nil {
		return fmt.Errorf("failed to write cache file: %w", err)
	}

	logrus.Debugf("Cached response to %s", fullpath)
	return nil
}

// Init ensures the cache directory exists
func (d *DiskCache) Init() error {
	if err := os.MkdirAll(d.cacheDir, 0755); err != nil {
		return fmt.Errorf("failed to create cache directory: %w", err)
	}
	return nil
}
