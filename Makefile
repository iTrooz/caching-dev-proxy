.PHONY: build run test clean

# Build the binary
build:
	go build -o caching-dev-proxy ./cmd/proxy

# Run with default config
run:
	go run ./cmd/proxy

# Run tests
test:
	go test ./...

# Clean up
clean:
	rm -rf cache/ caching-dev-proxy
