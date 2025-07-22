package cache

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/sirupsen/logrus"
)

type HTTPCache struct {
	cache GenericCache
}

func NewHTTP(cache GenericCache) *HTTPCache {
	return &HTTPCache{
		cache: cache,
	}
}

// Generates a unique key to store a value, based on these attributes
func (d *HTTPCache) getKey(request *http.Request) (string, error) {
	// Create hash for query parameters to handle complex URLs
	hash := sha256.Sum256([]byte(request.URL.RawQuery))
	queryHash := hex.EncodeToString(hash[:])[:8]

	// Build path: /cache_folder/host/path/METHOD[_queryhash].bin
	host := strings.TrimSuffix(strings.TrimSuffix(request.URL.Host, ":80"), ":443")
	pathParts := []string{host}

	if request.URL.Path != "" && request.URL.Path != "/" {
		pathParts = append(pathParts, strings.Trim(request.URL.Path, "/"))
	}

	filename := request.Method
	if request.URL.RawQuery != "" {
		filename += "_" + queryHash
	}
	filename += ".bin"

	pathParts = append(pathParts, filename)

	return filepath.Join(pathParts...), nil
}

func (d *HTTPCache) Set(request *http.Request, resp *http.Response) error {
	cacheKey, err := d.getKey(request)
	if err != nil {
		return fmt.Errorf("failed to generate cache key: %w", err)
	}

	data, err := Serialize(resp)
	if err != nil {
		return fmt.Errorf("failed to read response body: %w", err)
	}

	if err := d.cache.Set(cacheKey, data); err != nil {
		return fmt.Errorf("failed to set cache: %w", err)
	}

	return nil
}

func (d *HTTPCache) Get(key *http.Request) (*http.Response, error) {
	cachePath, err := d.getKey(key)
	if err != nil {
		return nil, fmt.Errorf("failed to generate cache key: %w", err)
	}

	data, err := d.cache.Get(cachePath)
	if err != nil {
		return nil, fmt.Errorf("failed to get cache: %w", err)
	}
	if data == nil {
		return nil, nil // Cache miss
	}

	resp, err := Deserialize(data)
	if err != nil {
		return nil, fmt.Errorf("failed to deserialize response: %w", err)
	}
	resp.Request = key // Associate the original request with the response

	logrus.Debugf("Cache hit for %s %s", key.Method, key.URL.String())
	return resp, nil
}
