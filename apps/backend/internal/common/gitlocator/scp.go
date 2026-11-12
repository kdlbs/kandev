// Package gitlocator validates credential-free Git clone locators.
package gitlocator

import (
	"errors"
	"net"
	"strings"
)

// ErrInvalidConventionalSCP reports a locator that is not the supported
// conventional SCP form. It deliberately does not echo the submitted value.
var ErrInvalidConventionalSCP = errors.New("invalid conventional SCP locator")

// ConventionalSCP is a parsed git@host:path locator.
type ConventionalSCP struct {
	Host string
	Path string
}

const maxHostnameLength = 253

// ParseConventionalSCP parses the complete conventional SCP form
// git@host:path. It accepts a DNS host, including a single label, or an IPv4
// address. The path must contain one or more relative segments made of ASCII
// letters, digits, dots, underscores, and hyphens. A terminal .git suffix is
// allowed but not required.
//
// Every other input, including SSH ports, IPv6 hosts, URL user information,
// whitespace, controls, queries, fragments, percent escapes, and absolute or
// traversing paths, is rejected. The result is never normalized.
func ParseConventionalSCP(locator string) (ConventionalSCP, error) {
	if !strings.HasPrefix(locator, "git@") {
		return ConventionalSCP{}, ErrInvalidConventionalSCP
	}
	remainder := strings.TrimPrefix(locator, "git@")
	if strings.Count(remainder, ":") != 1 {
		return ConventionalSCP{}, ErrInvalidConventionalSCP
	}
	host, path, _ := strings.Cut(remainder, ":")
	if !validSCPHost(host) || !validSCPPath(path) {
		return ConventionalSCP{}, ErrInvalidConventionalSCP
	}
	return ConventionalSCP{Host: host, Path: path}, nil
}

func validSCPHost(host string) bool {
	if host == "" || len(host) > maxHostnameLength {
		return false
	}
	if numericHost(host) {
		address := net.ParseIP(host)
		return address != nil && address.To4() != nil
	}
	for _, label := range strings.Split(host, ".") {
		if !validDNSLabel(label) {
			return false
		}
	}
	return true
}

func validDNSLabel(label string) bool {
	if label == "" || len(label) > 63 {
		return false
	}
	if label[0] == '-' || label[len(label)-1] == '-' {
		return false
	}
	for index := 0; index < len(label); index++ {
		character := label[index]
		if isASCIILetter(character) || isASCIIDigit(character) || character == '-' {
			continue
		}
		return false
	}
	return true
}

func validSCPPath(path string) bool {
	if path == "" || strings.HasPrefix(path, "/") || strings.HasSuffix(path, "/") {
		return false
	}
	for _, segment := range strings.Split(path, "/") {
		if segment == "" || segment == "." || segment == ".." || segment[0] == '-' {
			return false
		}
		for index := 0; index < len(segment); index++ {
			character := segment[index]
			if isASCIILetter(character) || isASCIIDigit(character) ||
				character == '.' || character == '_' || character == '-' {
				continue
			}
			return false
		}
	}
	return true
}

func numericHost(host string) bool {
	for index := 0; index < len(host); index++ {
		character := host[index]
		if !isASCIIDigit(character) && character != '.' {
			return false
		}
	}
	return true
}

func isASCIILetter(character byte) bool {
	return character >= 'a' && character <= 'z' || character >= 'A' && character <= 'Z'
}

func isASCIIDigit(character byte) bool {
	return character >= '0' && character <= '9'
}
