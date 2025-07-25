package proxy

import (
	"crypto/tls"
	"fmt"

	"github.com/sirupsen/logrus"
)

// simpleCertStore implements goproxy.CertStorage for certificate caching
type simpleCertStore struct {
	certs map[string]*tls.Certificate
}

func (s *simpleCertStore) Fetch(hostname string, gen func() (*tls.Certificate, error)) (*tls.Certificate, error) {
	cert, ok := s.certs[hostname]
	if ok {
		return cert, nil
	}

	cert, err := gen()
	if err != nil {
		logrus.Errorf("Failed to generate certificate for hostname '%s': %v", hostname, err)
		return nil, fmt.Errorf("failed to generate certificate for hostname '%s': %w", hostname, err)
	}

	s.certs[hostname] = cert
	return cert, nil
}
