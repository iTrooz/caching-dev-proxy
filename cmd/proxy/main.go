package main

import (
	"log"
	"os"

	"caching-dev-proxy/internal/config"
	"caching-dev-proxy/internal/proxy"
)

func main() {
	configPath := "configs/config.yaml"
	if len(os.Args) > 1 {
		configPath = os.Args[1]
	}

	cfg, err := config.Load(configPath)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	if err := cfg.Validate(); err != nil {
		log.Fatalf("Invalid configuration: %v", err)
	}

	server, err := proxy.New(cfg)
	if err != nil {
		log.Fatalf("Failed to create proxy server: %v", err)
	}

	if err := server.Start(); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}
