package proxy

import (
	"fmt"
	"net/http"
)

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
