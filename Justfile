default:
    just --summary

build:
	go build -o caching-dev-proxy ./cmd/proxy

run *args:
	go run ./cmd/proxy {{args}}

test *args:
	go test ./... {{args}}

clean:
	rm -rf cache/ caching-dev-proxy
