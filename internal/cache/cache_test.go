package cache

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestNew(t *testing.T) {
	cacheDir := "/tmp/test_cache"
	ttl := time.Hour

	cache := NewDisk(cacheDir, ttl)

	// Test that NewDisk returns a non-nil cache
	if cache == nil {
		t.Error("Expected non-nil cache")
	}

	// Test that the cache behaves correctly by testing its methods
	testURL := "https://example.com/test"
	testMethod := "GET"
	path, err := cache.GetKey(testURL, testMethod)
	if err != nil {
		t.Errorf("Expected no error, got: %v", err)
	}
	if path == "" {
		t.Error("Expected non-empty path")
	}
}

func TestGetPath(t *testing.T) {
	cache := NewDisk("/tmp/cache", time.Hour)

	tests := []struct {
		name      string
		targetURL string
		method    string
		want      string
	}{
		{
			name:      "simple URL",
			targetURL: "https://example.com/api/users",
			method:    "GET",
			want:      "/tmp/cache/example.com/api/users/GET.bin",
		},
		{
			name:      "URL with query params",
			targetURL: "https://api.github.com/users?page=1",
			method:    "GET",
			want:      "/tmp/cache/api.github.com/users/GET_c5c34f0f.bin",
		},
		{
			name:      "root path",
			targetURL: "https://example.com/",
			method:    "POST",
			want:      "/tmp/cache/example.com/POST.bin",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := cache.GetKey(tt.targetURL, tt.method)
			if err != nil {
				t.Errorf("GetKey() error = %v", err)
				return
			}
			if got != tt.want {
				t.Errorf("GetPath() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSetAndGet(t *testing.T) {
	tempDir := t.TempDir()
	cache := NewDisk(tempDir, time.Hour)

	// Test data
	cachePath := filepath.Join(tempDir, "test", "cache.bin")
	testData := []byte("test response data")

	// Test Set
	err := cache.Set(cachePath, testData)
	if err != nil {
		t.Fatalf("Set() error = %v", err)
	}

	// Verify file exists
	if _, err := os.Stat(cachePath); os.IsNotExist(err) {
		t.Fatalf("Cache file was not created")
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

func TestGetExpired(t *testing.T) {
	tempDir := t.TempDir()
	cache := NewDisk(tempDir, 100*time.Millisecond) // Very short TTL

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

func TestInit(t *testing.T) {
	tempDir := t.TempDir()
	cacheDir := filepath.Join(tempDir, "new", "cache", "dir")

	cache := NewDisk(cacheDir, time.Hour)

	err := cache.Init()
	if err != nil {
		t.Fatalf("Init() error = %v", err)
	}

	// Verify directory was created
	if _, err := os.Stat(cacheDir); os.IsNotExist(err) {
		t.Fatalf("Cache directory was not created")
	}
}

func TestGetKeyError(t *testing.T) {
	cache := NewDisk("/tmp/cache", time.Hour)

	// Test with invalid URL
	_, err := cache.GetKey("://invalid-url", "GET")
	if err == nil {
		t.Error("Expected error for invalid URL, got nil")
	}

	// Test with valid URL should not error
	_, err = cache.GetKey("https://example.com", "GET")
	if err != nil {
		t.Errorf("Expected no error for valid URL, got: %v", err)
	}
}
