package proxy

import (
	"caching-dev-proxy/internal/config"
	"fmt"
	"net/http"
	"strings"
)

func matchesRule(targetURL, method string, statusCode int, rule config.CacheRule) bool {
	// Check if URL starts with base URI
	if !strings.HasPrefix(targetURL, rule.BaseURI) {
		return false
	}

	// Check if method matches
	methodMatches := false
	for _, m := range rule.Methods {
		if strings.EqualFold(m, method) {
			methodMatches = true
			break
		}
	}
	if !methodMatches {
		return false
	}

	// Check if status code matches (if specified)
	if len(rule.StatusCodes) > 0 {
		statusMatches := false
		for _, statusPattern := range rule.StatusCodes {
			if config.MatchesStatusCode(statusCode, statusPattern) {
				statusMatches = true
				break
			}
		}
		if !statusMatches {
			return false
		}
	}

	return true
}

func getTargetURL(r *http.Request) string {
	if r.URL.IsAbs() {
		return r.URL.String()
	}

	// Reconstruct URL from Host header
	scheme := "http"
	if r.TLS != nil || r.URL.Scheme == "https" {
		scheme = "https"
	}

	// Handle case where URL already has scheme set (from SSL bumping)
	if r.URL.Scheme != "" {
		scheme = r.URL.Scheme
	}

	host := r.Host
	if host == "" && r.URL.Host != "" {
		host = r.URL.Host
	}

	path := r.URL.Path
	if path == "" {
		path = "/"
	}

	query := ""
	if r.URL.RawQuery != "" {
		query = "?" + r.URL.RawQuery
	}

	return fmt.Sprintf("%s://%s%s%s", scheme, host, path, query)
}
