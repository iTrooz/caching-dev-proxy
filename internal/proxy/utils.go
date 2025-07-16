package proxy

import (
	"caching-dev-proxy/internal/config"
	"fmt"
	"net/http"
	"strings"
)

func matchesRule(targetURL, method string, rule config.CacheRule) bool {
	// Check if URL starts with base URI
	if !strings.HasPrefix(targetURL, rule.BaseURI) {
		return false
	}

	// Check if method matches
	for _, m := range rule.Methods {
		if strings.EqualFold(m, method) {
			return true
		}
	}

	return false
}

func getTargetURL(r *http.Request) string {
	if r.URL.IsAbs() {
		return r.URL.String()
	}

	// Reconstruct URL from Host header
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}

	return fmt.Sprintf("%s://%s%s", scheme, r.Host, r.URL.String())
}
