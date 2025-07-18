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
	path := cache.GetKey(testURL, testMethod)
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
			got := cache.GetKey(tt.targetURL, tt.method)
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
	data, found := cache.Get(cachePath)
	if !found {
		t.Fatalf("Get() found = false, want true")
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
	_, found := cache.Get(cachePath)
	if found {
		t.Errorf("Get() found = true, want false (should be expired)")
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
