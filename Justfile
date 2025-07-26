default:
    just --summary

build:
	go build -o caching-dev-proxy ./

run *args:
	go run ./ {{args}}

test *args:
	go test ./... {{args}}

clean:
	rm -rf cache/ caching-dev-proxy
