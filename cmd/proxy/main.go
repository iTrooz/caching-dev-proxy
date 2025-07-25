package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"caching-dev-proxy/internal/config"
	"caching-dev-proxy/internal/proxy"

	"github.com/sirupsen/logrus"
)

func setupLogrus(level string) {
	logrus.SetFormatter(&logrus.TextFormatter{
		FullTimestamp: true,
	})

	switch level {
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
}

func main() {
	// Parse CLI flags
	configPathPtr := flag.String("config", "", "Configuration file path")
	addressPtr := flag.String("a", "", "Address to listen on (example: :8080)")
	verbosePtr := flag.Bool("v", false, "Enable verbose (debug) logging, overrides config")
	flag.Parse()

	// Set log level from CLI for setup logging (will be overriden later)
	if *verbosePtr {
		logrus.SetLevel(logrus.DebugLevel)
	}

	// Load config
	configPath := resolveConfigPath(*configPathPtr)
	cfg, err := config.Load(configPath)
	if err != nil {
		logrus.Fatalf("Failed to load config: %v", err)
	}

	// Handle CLI overrides
	if *addressPtr != "" {
		cfg.Server.HTTP.Address = *addressPtr
	}
	if *verbosePtr {
		cfg.Log.Level = "debug"
	}

	// Validate config
	if err := cfg.Validate(); err != nil {
		logrus.Fatalf("Invalid configuration: %v", err)
	}

	// Setup logging
	setupLogrus(cfg.Log.Level)

	// Launch proxy
	launchProxy(cfg)
}

func resolveConfigPath(cliPath string) string {
	if cliPath != "" {
		return cliPath
	}

	if envPath := os.Getenv("APP_CONFIG"); envPath != "" {
		return envPath
	}

	home := os.Getenv("HOME")
	xdg := os.Getenv("XDG_CONFIG_HOME")
	var base string
	if xdg != "" {
		base = xdg
	} else if home != "" {
		base = filepath.Join(home, ".config")
	} else {
		base = fmt.Sprintf("/home/%s/.config", os.Getenv("USER"))
	}
	return filepath.Join(base, "caching-dev-proxy", "config.yaml")
}

func launchProxy(cfg *config.Config) {
	server, err := proxy.New(cfg)
	if err != nil {
		logrus.Fatalf("Failed to create proxy server: %v", err)
	}

	if err := server.Start(); err != nil {
		logrus.Fatalf("Server failed: %v", err)
	}
}
