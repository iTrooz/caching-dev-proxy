package httpcache

import "github.com/iTrooz/caching-dev-proxy/internal/cache"

func New(cache cache.GenericCache) *HTTPCache {
	return &HTTPCache{
		cache: cache,
	}
}
