# caching-dev-proxy

This project allows you to cache any HTTP request sent by one of the tools on your computer. You can think of it as an alternative to Squid, focused on local usage and developer experience

# Features
- Can cache EVERY request (with no respect to cache headers, e.g. `Cache-Control: no-cache`)
- TTL (time to live) for cache entries
- HTTP proxying
- HTTPS proxying with MITM
- explicit & transparent proxying
- Configuration based on request metadata (url, method..)

# Installation

```sh
go install github.com/iTrooz/caching-dev-proxy@latest
```

# Usage

## Classic (explicit proxying)

1. (Optional) edit [config.yaml](./config.yaml), and place it at `~.config/caching-dev-proxy/config.yaml` (or specify it when running proxy)

2. Run proxy with
```sh
caching-dev-proxy
```

3. Run your requests through the proxy with e.g. `curl -x 127.0.0.1:8080 https://example.com`

## TLS decryption
1. Generate TLS root certificate and key that caching-dev-proxy will use for TLS decryption. For example:
```sh
openssl req -x509 -newkey rsa:4096 -keyout ca.key.pem -out ca.crt.pem -days 8250 -nodes -subj "/CN=My CA"
```
2. Add this certificate to your system store. On ArchLinux, use `trust anchor <path_to_cert.pem>`
3. Edit config and start proxy as shown above

## Transparent proxying
Note: HTTP transparent proxying uses the Host header, and HTTPS transparent proxying uses SNI to determine the upstream host to send the request to. [Unlike squid](https://www.squid-cache.org/Doc/config/host_verify_strict/), the destination IP is ignored entirely, allowing for simple domain name spoofing, e.g. by editing `/etc/hosts` to make given hosts pass through the proxy.

# Security considerations
This tool was made to be used by a developer on their development machine. It should not be let exposed to untrusted parties, and ESPECIALLY NOT be freely usable on the Internet.
See:
- https://www.squid-cache.org/Doc/config/host_verify_strict/
- CVE-2009-0801

# Development
## Run
`just run <args>`
## Build
`just build`

# Licence
GPL-2.0-only OR GPL-3.0-or-later with proxy being me (iTrooz) [What does that mean ?](https://itrooz.fr/posts/gpl_licence/)
