package httpcache

import (
	"bufio"
	"bytes"
	"fmt"
	"net/http"
	"net/http/httputil"
)

const PREFIX = "---HTTP-RESPONSE---\n"

// Serialize writes the http.Response to the given writer using gob encoding.
func Serialize(resp *http.Response) ([]byte, error) {
	b, err := httputil.DumpResponse(resp, true)
	if err != nil {
		return nil, err
	}

	return append([]byte(PREFIX), b...), nil
}

func Deserialize(b []byte) (*http.Response, error) {
	got_prefix := string(b[0:len(PREFIX)])
	if got_prefix != PREFIX {
		return nil, fmt.Errorf("invalid prefix: expected '%s', got '%s'", PREFIX, got_prefix)
	}

	resp, err := http.ReadResponse(bufio.NewReader(bytes.NewReader(b[len(PREFIX):])), nil)
	if err != nil {
		return nil, fmt.Errorf("failed to deserialize response: %w", err)
	}

	return resp, nil
}
