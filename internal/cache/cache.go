// Handles caching of HTTP responses
package cache

// Cache interface for caching operations
type Cache interface {
	// Generates a unique key to store a value, based on these attributes
	GetKey(targetURL, method string) string
	// retrieves cached response data if it exists and is not expired
	Get(key string) ([]byte, bool)
	// stores response data in the cache at the specified path
	Set(key string, value []byte) error
	// initializes the cache (e.g., creates necessary directories)
	Init() error
}
