package proxy

import (
	"strings"

	"caching-dev-proxy/internal/config"
)

// Rule interface for matching requests against caching rules
type Rule interface {
	Match(targetURL, method string, statusCode int) bool
}

// ConfigRule implements Rule interface for config-based rules
type ConfigRule struct {
	config.CacheRule
}

// Match checks if a request matches this rule
func (r *ConfigRule) Match(targetURL, method string, statusCode int) bool {
	// Check if URL starts with base URI
	if !strings.HasPrefix(targetURL, r.BaseURI) {
		return false
	}

	// Check if method matches
	methodMatches := false
	for _, m := range r.Methods {
		if strings.EqualFold(m, method) {
			methodMatches = true
			break
		}
	}
	if !methodMatches {
		return false
	}

	// Check if status code matches (if specified)
	if len(r.StatusCodes) > 0 {
		statusMatches := false
		for _, statusPattern := range r.StatusCodes {
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
