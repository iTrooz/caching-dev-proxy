package cache

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestGenericDiskCache(t *testing.T) {
	cacheDir := "/tmp/test_cache"
	ttl := time.Hour

	cache := NewGenericDisk(cacheDir, ttl)

	// Test that NewGenericDisk returns a non-nil cache
	if cache == nil {
		t.Error("Expected non-nil cache")
	}
}

func TestGenericDiskSetAndGet(t *testing.T) {
	tempDir := t.TempDir()
	cache := NewGenericDisk(tempDir, time.Hour)

	// Test data
	cachePath := filepath.Join("test", "cache.bin")
	testData := []byte("test response data")

	// Test Set
	err := cache.Set(cachePath, testData)
	if err != nil {
		t.Fatalf("Set() error = %v", err)
	}

	// Verify file exists at the correct location
	expectedPath := filepath.Join(tempDir, cachePath)
	if _, err := os.Stat(expectedPath); os.IsNotExist(err) {
		t.Fatalf("Cache file was not created at %s", expectedPath)
	}

	// Test Get
	data, err := cache.Get(cachePath)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if data == nil {
		t.Fatalf("Get() returned nil data, want cached data")
	}

	if string(data) != string(testData) {
		t.Errorf("Get() data = %s, want %s", string(data), string(testData))
	}
}

func TestGenericDiskGetExpired(t *testing.T) {
	tempDir := t.TempDir()
	cache := NewGenericDisk(tempDir, 100*time.Millisecond) // Very short TTL

	cachePath := filepath.Join(tempDir, "expired.bin")
	testData := []byte("test data")

	// Set data
	err := cache.Set(cachePath, testData)
	if err != nil {
		t.Fatalf("Set() error = %v", err)
	}

	// Wait for expiration
	time.Sleep(200 * time.Millisecond)

	// Try to get expired data
	data, err := cache.Get(cachePath)
	if err != nil {
		t.Errorf("Get() error = %v", err)
	}
	if data != nil {
		t.Errorf("Get() returned data for expired cache, want nil")
	}

	// Verify file was deleted
	if _, err := os.Stat(cachePath); !os.IsNotExist(err) {
		t.Errorf("Expired cache file should have been deleted")
	}
}

func TestGenericDiskInit(t *testing.T) {
	tempDir := t.TempDir()
	cacheDir := filepath.Join(tempDir, "new", "cache", "dir")

	cache := NewGenericDisk(cacheDir, time.Hour)

	err := cache.Init()
	if err != nil {
		t.Fatalf("Init() error = %v", err)
	}

	// Verify directory was created
	if _, err := os.Stat(cacheDir); os.IsNotExist(err) {
		t.Fatalf("Cache directory was not created")
	}
}
