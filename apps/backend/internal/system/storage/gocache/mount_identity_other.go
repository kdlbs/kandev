//go:build !linux && !darwin && !windows

package gocache

import (
	"errors"
	"os"
)

func cacheMountIdentity(string) (string, error) {
	return "", errors.New("go-cache mount identity is unsupported on this platform")
}

func cacheMountIdentityFromFile(*os.File) (string, error) {
	return "", errors.New("go-cache mount identity is unsupported on this platform")
}
