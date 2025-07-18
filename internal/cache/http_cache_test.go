package cache

import (
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestHTTPCache(t *testing.T) {
	tempDir := t.TempDir()
	genericCache := NewGenericDisk(tempDir, time.Hour)
	httpCache := NewHTTP(genericCache)

	// Test that NewHTTP returns a non-nil cache
	if httpCache == nil {
		t.Error("Expected non-nil HTTP cache")
	}
}

func TestHTTPCacheGetAndSet(t *testing.T) {
	tempDir := t.TempDir()
	genericCache := NewGenericDisk(tempDir, time.Hour)
	httpCache := NewHTTP(genericCache)

	// Initialize the underlying generic cache
	err := genericCache.Init()
	if err != nil {
		t.Fatalf("Init() error = %v", err)
	}

	// Create test request
	req, err := http.NewRequest("GET", "https://example.com/api/users", nil)
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}

	// Test data
	testData := "test response data"
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(testData)),
		Header:     http.Header{"Content-Type": []string{"application/json"}},
	}

	// Test Set
	err = httpCache.Set(req, resp)
	if err != nil {
		t.Fatalf("Set() error = %v", err)
	}

	// Test Get
	cachedResp, err := httpCache.Get(req)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if cachedResp == nil {
		t.Fatalf("Get() returned nil response, want cached response")
	}

	// Read the cached response body
	cachedData, err := io.ReadAll(cachedResp.Body)
	if err != nil {
		t.Fatalf("Failed to read cached response body: %v", err)
	}

	if string(cachedData) != testData {
		t.Errorf("Get() data = %s, want %s", string(cachedData), testData)
	}
}

func TestHTTPCacheGetExpired(t *testing.T) {
	tempDir := t.TempDir()
	genericCache := NewGenericDisk(tempDir, 100*time.Millisecond) // Very short TTL
	httpCache := NewHTTP(genericCache)

	// Initialize the underlying generic cache
	err := genericCache.Init()
	if err != nil {
		t.Fatalf("Init() error = %v", err)
	}

	// Create test request
	req, err := http.NewRequest("GET", "https://example.com/api/test", nil)
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}

	// Test data
	testData := "test data"
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(testData)),
		Header:     http.Header{"Content-Type": []string{"application/json"}},
	}

	// Set data
	err = httpCache.Set(req, resp)
	if err != nil {
		t.Fatalf("Set() error = %v", err)
	}

	// Wait for expiration
	time.Sleep(200 * time.Millisecond)

	// Try to get expired data
	cachedResp, err := httpCache.Get(req)
	if err != nil {
		t.Errorf("Get() error = %v", err)
	}
	if cachedResp != nil {
		t.Errorf("Get() returned response for expired cache, want nil")
	}
}

func TestHTTPCacheGetKeyError(t *testing.T) {
	tempDir := t.TempDir()
	genericCache := NewGenericDisk(tempDir, time.Hour)
	httpCache := NewHTTP(genericCache)

	// Create request with valid URL
	req, err := http.NewRequest("GET", "https://example.com", nil)
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}

	// Test with valid request should not error
	testData := "test data"
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(testData)),
		Header:     http.Header{"Content-Type": []string{"application/json"}},
	}

	err = httpCache.Set(req, resp)
	if err != nil {
		t.Errorf("Expected no error for valid request, got: %v", err)
	}

	// Test Get with valid request should not error
	cachedResp, err := httpCache.Get(req)
	if err != nil {
		t.Errorf("Expected no error for valid request, got: %v", err)
	}
	if cachedResp == nil {
		t.Errorf("Expected cached response, got nil")
	}
}
