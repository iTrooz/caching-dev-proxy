package proxy

import (
	"net/http"
)

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
