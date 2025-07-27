package httpcache

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/iTrooz/caching-dev-proxy/internal/cache"
)

type HTTPCache struct {
	cache cache.GenericCache
}

func NewHTTP(cache cache.GenericCache) *HTTPCache {
	return &HTTPCache{
		cache: cache,
	}
}

// Generates a unique key to store a value, based on URL, method, selected headers, and body
func (d *HTTPCache) GenerateKey(request *http.Request) (string, error) {
	// Hash query parameters
	hash := sha256.Sum256([]byte(request.URL.RawQuery))
	queryHash := hex.EncodeToString(hash[:])[:8]

	// Hash selected headers
	headersToHash := []string{"Host", "Accept", "Accept-Encoding", "Accept-Language", "Content-Type"}
	headersStr := ""
	for _, k := range headersToHash {
		if v, ok := request.Header[k]; ok {
			headersStr += k + ":" + strings.Join(v, ",") + "\n"
		}
	}
	headersHash := sha256.Sum256([]byte(headersStr))
	headersHashStr := hex.EncodeToString(headersHash[:])[:8]

	// Hash body (read and restore)
	var bodyHashStr string
	if request.Body != nil {
		bodyBytes, _ := io.ReadAll(request.Body)
		if err := request.Body.Close(); err != nil {
			return "", fmt.Errorf("failed to close request body: %w", err)
		}
		if len(bodyBytes) > 0 {
			request.Body = io.NopCloser(strings.NewReader(string(bodyBytes))) // restore
			bodyHash := sha256.Sum256(bodyBytes)
			bodyHashStr = hex.EncodeToString(bodyHash[:])[:8]
		}
	}

	// Build path: /cache_folder/host/path/METHOD[_queryhash][_headershash][_bodyhash].bin
	host := strings.TrimSuffix(strings.TrimSuffix(request.URL.Host, ":80"), ":443")
	pathParts := []string{host}

	if request.URL.Path != "" && request.URL.Path != "/" {
		pathParts = append(pathParts, strings.Trim(request.URL.Path, "/"))
	}

	filename := request.Method
	if request.URL.RawQuery != "" {
		filename += "_q" + queryHash
	}
	if headersStr != "" {
		filename += "_h" + headersHashStr
	}
	if bodyHashStr != "" {
		filename += "_b" + bodyHashStr
	}
	filename += ".bin"

	pathParts = append(pathParts, filename)

	return filepath.Join(pathParts...), nil
}

func (d *HTTPCache) SetReq(request *http.Request, resp *http.Response) error {
	cacheKey, err := d.GenerateKey(request)
	if err != nil {
		return fmt.Errorf("failed to generate cache key: %w", err)
	}

	return d.SetKey(cacheKey, resp)
}

func (d *HTTPCache) SetKey(requestKey string, resp *http.Response) error {
	data, err := Serialize(resp)
	if err != nil {
		return fmt.Errorf("failed to read response body: %w", err)
	}

	if err := d.cache.Set(requestKey, data); err != nil {
		return fmt.Errorf("failed to set cache: %w", err)
	}

	return nil
}

func (d *HTTPCache) GetReq(req *http.Request) (*http.Response, error) {
	requestKey, err := d.GenerateKey(req)
	if err != nil {
		return nil, fmt.Errorf("failed to generate cache key: %w", err)
	}

	resp, err := d.GetKey(requestKey)
	if err != nil {
		return nil, err
	}
	// Handle no cache hit
	if resp == nil {
		return nil, nil
	}

	// Associate the original request with the response
	resp.Request = req
	return resp, nil
}

func (d *HTTPCache) GetKey(requestKey string) (*http.Response, error) {
	data, err := d.cache.Get(requestKey)
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
	return resp, nil
}
