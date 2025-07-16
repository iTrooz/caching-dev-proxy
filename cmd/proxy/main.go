package main

import (
	"os"

	"caching-dev-proxy/internal/config"
	"caching-dev-proxy/internal/proxy"
	"github.com/sirupsen/logrus"
)

func main() {
	configPath := "config.yaml"
	if len(os.Args) > 1 {
		configPath = os.Args[1]
	}

	cfg, err := config.Load(configPath)
	if err != nil {
		logrus.Fatalf("Failed to load config: %v", err)
	}

	if err := cfg.Validate(); err != nil {
		logrus.Fatalf("Invalid configuration: %v", err)
	}

	// Setup logrus based on config
	logrus.SetFormatter(&logrus.TextFormatter{
		FullTimestamp: true,
	})
	
	switch cfg.Log.Level {
	case "debug":
		logrus.SetLevel(logrus.DebugLevel)
	case "info":
		logrus.SetLevel(logrus.InfoLevel)
	case "warn":
		logrus.SetLevel(logrus.WarnLevel)
	case "error":
		logrus.SetLevel(logrus.ErrorLevel)
	default:
		logrus.SetLevel(logrus.InfoLevel)
	}

	server, err := proxy.New(cfg)
	if err != nil {
		logrus.Fatalf("Failed to create proxy server: %v", err)
	}

	if err := server.Start(); err != nil {
		logrus.Fatalf("Server failed: %v", err)
	}
}
