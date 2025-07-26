package proxy

import (
	"crypto/tls"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/url"

	"github.com/iTrooz/caching-dev-proxy/internal/config"

	"github.com/elazarl/goproxy"
	"github.com/inconshreveable/go-vhost"
	"github.com/sirupsen/logrus"
)

func loadCertificate(cfg *config.Config) (*tls.Certificate, error) {
	if cfg.Server.HTTPS.CACertFile == "" || cfg.Server.HTTPS.CAKeyFile == "" {
		logrus.Debugf("No CA certificate configured, using goproxy default certificate")
		return nil, nil // Use default goproxy certificate
	}

	cert, err := tls.LoadX509KeyPair(cfg.Server.HTTPS.CACertFile, cfg.Server.HTTPS.CAKeyFile)
	if err != nil {
		return nil, fmt.Errorf("failed to load CA certificate and key: %w", err)
	}
	logrus.Debugf("Loaded CA certificate from %s", cfg.Server.HTTPS.CACertFile)
	return &cert, nil
}

func (s *Server) setupHTTPSProxyHandler() {
	// Load CA certificate
	caCert, err := loadCertificate(s.config)
	if err != nil {
		logrus.Errorf("Failed to load CA certificate: %v", err)
		return
	}

	if caCert == nil {
		// Use goproxy's default certificate
		logrus.Warnf("TLS interception enabled but no CA certificate loaded, using goproxy default certificate")
		s.proxy.OnRequest().HandleConnect(goproxy.AlwaysMitm)
	} else {
		// Make goproxy use our provided CA certificate
		customCaMitm := &goproxy.ConnectAction{
			Action:    goproxy.ConnectMitm,
			TLSConfig: goproxy.TLSConfigFromCA(caCert),
		}
		customAlwaysMitm := goproxy.FuncHttpsHandler(func(host string, ctx *goproxy.ProxyCtx) (*goproxy.ConnectAction, string) {
			logrus.Debugf("Handling CONNECT request for %s", host)
			return customCaMitm, host
		})
		s.proxy.OnRequest().HandleConnect(customAlwaysMitm)
	}
}

// StartTransparentHTTPS enables transparent HTTPS proxying
func (s *Server) StartTransparentHTTPS(httpsAddr string) {
	ln, err := net.Listen("tcp", httpsAddr)
	if err != nil {
		log.Fatalf("Error listening for https connections - %v", err)
	}
	for {
		c, err := ln.Accept()
		if err != nil {
			log.Printf("Error accepting new connection - %v", err)
			continue
		}
		go func(c net.Conn) {
			tlsConn, err := vhost.TLS(c)
			if err != nil {
				log.Printf("Error accepting new connection - %v", err)
				return
			}
			if tlsConn.Host() == "" {
				log.Printf("Cannot support non-SNI enabled clients")
				return
			}
			connectReq := &http.Request{
				Method: http.MethodConnect,
				URL: &url.URL{
					Opaque: tlsConn.Host(),
					Host:   net.JoinHostPort(tlsConn.Host(), "443"),
				},
				Host:       tlsConn.Host(),
				Header:     make(http.Header),
				RemoteAddr: c.RemoteAddr().String(),
			}
			resp := dumbResponseWriter{tlsConn}
			s.proxy.ServeHTTP(resp, connectReq)
		}(c)
	}
}
