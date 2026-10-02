//go:build !linux && !darwin && !windows

package gocache

import "errors"

func cacheMountIdentity(string) (string, error) {
	return "", errors.New("Go-cache mount identity is unsupported on this platform")
}
