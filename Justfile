default:
    just --summary

build:
	go build -o caching-dev-proxy ./cmd/proxy

run *args:
	go run ./cmd/proxy {{args}}

test:
	go test ./...

clean:
	rm -rf cache/ caching-dev-proxy
