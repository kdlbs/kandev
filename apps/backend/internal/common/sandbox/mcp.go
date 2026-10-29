package sandbox

import (
	"encoding/hex"
	"net/url"
	"strconv"
	"strings"
)

func validMCPRoute(raw string) bool {
	if raw == "" {
		return true
	}
	parsed, err := url.Parse(raw)
	if err != nil || !validMCPEndpoint(parsed) {
		return false
	}
	port, err := strconv.ParseUint(parsed.Port(), 10, 16)
	if err != nil || port == 0 || strconv.FormatUint(port, 10) != parsed.Port() {
		return false
	}
	parts := strings.Split(parsed.Path, "/")
	if len(parts) != 3 || parts[0] != "" || parts[2] != "mcp" || len(parts[1]) != 64 || strings.ToLower(parts[1]) != parts[1] {
		return false
	}
	_, err = hex.DecodeString(parts[1])
	return err == nil
}

func validMCPEndpoint(parsed *url.URL) bool {
	return parsed.Scheme == "http" && parsed.Hostname() == "127.0.0.1" && parsed.User == nil && parsed.RawQuery == "" && !parsed.ForceQuery && parsed.Fragment == "" && parsed.RawFragment == "" && parsed.RawPath == ""
}
