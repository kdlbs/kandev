package client

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func readRecoveryControlEndpoint(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	if len(data) == 0 {
		return "", errors.New("recovery control endpoint is not published yet")
	}
	return string(data), nil
}

func TestRecoveryControlEndpointPublication(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ready")
	if endpoint, err := readRecoveryControlEndpoint(path); err == nil || endpoint != "" {
		t.Fatalf("missing publication = %q, %v", endpoint, err)
	}
	// The child creates its regular file before writing its endpoint.
	if err := os.WriteFile(path, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if endpoint, err := readRecoveryControlEndpoint(path); err == nil || endpoint != "" {
		t.Fatalf("empty publication was accepted as ready: endpoint=%q, error=%v", endpoint, err)
	}
	const address = "127.0.0.1:39429"
	if err := os.WriteFile(path, []byte(address), 0o600); err != nil {
		t.Fatal(err)
	}
	if endpoint, err := readRecoveryControlEndpoint(path); err != nil || endpoint != address {
		t.Fatalf("published endpoint = %q, %v; want %q", endpoint, err, address)
	}
}
