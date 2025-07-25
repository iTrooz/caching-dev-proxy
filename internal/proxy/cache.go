package proxy

import (
	"net/http"

	"github.com/sirupsen/logrus"
)

// getCachedResponse returns a cached HTTP response if available
func (s *Server) getCachedResponse(requ *http.Request) *http.Response {
	resp, err := s.cacheManager.Get(requ)
	// If cache lookup fails, only log as error if the request should be cached
	if err != nil {
		if s.shouldBeCached(requ, nil) {
			logrus.Errorf("Failed to get cached data for %s: %v", requ.URL, err)
		} else {
			logrus.Debugf("Cache not found for %s (caching disabled by rules)", requ.URL)
		}
		return nil
	}
	if resp == nil {
		logrus.Debugf("No cached data found for %s", requ.URL)
		return nil
	}

	resp.Header.Set("X-Cache", "HIT")

	return resp
}

// shouldBeCached determines if a response should be cached based on rules
func (s *Server) shouldBeCached(requ *http.Request, resp *http.Response) bool {
	matched := false
	for _, rule := range s.rules {
		if rule.Match(requ, resp) {
			matched = true
			break
		}
	}

	if s.config.Rules.Mode == "whitelist" {
		return matched
	} else {
		return !matched
	}
}

// cacheResponse stores a response in the cache
func (s *Server) cacheResponse(requ *http.Request, resp *http.Response) {
	if err := s.cacheManager.Set(requ, resp); err != nil {
		logrus.Errorf("Failed to cache response for %s: %v", requ.URL.String(), err)
	}
}
