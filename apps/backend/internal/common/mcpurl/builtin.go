package mcpurl

import (
	"errors"
	"net"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
)

const InstanceRoutePrefix = "/i/"

var ErrInvalidBuiltinRoute = errors.New("invalid built-in MCP route")

func BuiltinPath(instanceID string, scoped bool, endpoint string) (string, error) {
	prefix, err := RoutePrefix(instanceID, scoped)
	if err != nil {
		return "", err
	}
	return RoutePath(prefix, endpoint)
}

func RoutePath(prefix, endpoint string) (string, error) {
	if !validEndpoint(endpoint) {
		return "", ErrInvalidBuiltinRoute
	}
	if prefix != "" {
		if !strings.HasPrefix(prefix, InstanceRoutePrefix) || !validInstanceID(strings.TrimPrefix(prefix, InstanceRoutePrefix)) {
			return "", ErrInvalidBuiltinRoute
		}
	}
	return prefix + endpoint, nil
}

func BuiltinURL(host string, port int, instanceID string, scoped bool, endpoint string) (string, error) {
	path, err := BuiltinPath(instanceID, scoped, endpoint)
	if err != nil || port < 0 || port > 65535 {
		return "", ErrInvalidBuiltinRoute
	}
	return builtinURL(host, port, path), nil
}

func RoutePrefix(instanceID string, scoped bool) (string, error) {
	if !scoped {
		return "", nil
	}
	if !validInstanceID(instanceID) {
		return "", ErrInvalidBuiltinRoute
	}
	return InstanceRoutePrefix + instanceID, nil
}

func ValidateInstanceID(id string) error {
	if !validInstanceID(id) {
		return ErrInvalidBuiltinRoute
	}
	return nil
}

func IsBuiltinEndpoint(path string) bool {
	return validEndpoint(path)
}

func OriginURL(host string, port int) (string, error) {
	if port < 0 || port > 65535 {
		return "", ErrInvalidBuiltinRoute
	}
	return builtinURL(host, port, ""), nil
}

func builtinURL(host string, port int, path string) string {
	if strings.TrimSpace(host) == "" {
		host = "localhost"
	} else {
		host = ReachableHost(host)
	}
	hostPort := net.JoinHostPort(host, strconv.Itoa(port))
	return (&url.URL{Scheme: "http", Host: hostPort, Path: path}).String()
}

func validInstanceID(id string) bool {
	if id == "" {
		return false
	}
	for _, char := range id {
		if (char < 'A' || char > 'Z') && (char < 'a' || char > 'z') && (char < '0' || char > '9') && char != '_' && char != '-' {
			return false
		}
	}
	return true
}

func validEndpoint(endpoint string) bool {
	parts := strings.Split(endpoint, "/")
	if len(parts) == 2 && parts[0] == "" {
		return parts[1] == "mcp" || parts[1] == "sse" || parts[1] == "message"
	}
	if len(parts) == 4 && parts[0] == "" && (parts[1] == "mcp" || parts[1] == "sse" || parts[1] == "message") && parts[2] == "packages" {
		switch parts[3] {
		case "core", "configuration", "rich-output", "walkthroughs", "diagnostics":
			return true
		}
	}
	return false
}

func ReachableHost(listenHost string) string {
	host := strings.TrimSpace(strings.Trim(listenHost, "[]"))
	if host == "" {
		return "127.0.0.1"
	}
	if addr, err := netip.ParseAddr(host); err == nil {
		if addr.IsUnspecified() {
			if addr.Is6() {
				return "::1"
			}
			return "127.0.0.1"
		}
		return addr.String()
	}
	return host
}
