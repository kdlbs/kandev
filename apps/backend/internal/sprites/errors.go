package sprites

import (
	"errors"
	"fmt"
	"strings"
)

// ErrSpriteNotFound is returned when an upstream Sprites API call indicates the
// sandbox does not exist. The upstream SDK currently returns plain errors with
// a "sprite not found: <name>" message for 404 responses; this sentinel lets
// callers branch on that case without ad-hoc string matching.
var ErrSpriteNotFound = errors.New("sprite not found")

// WrapNotFound wraps err with ErrSpriteNotFound when err looks like an upstream
// "sprite not found" / 404 response. Other errors are returned unchanged. Pass
// the returned error to errors.Is(err, ErrSpriteNotFound) to detect the case.
func WrapNotFound(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, ErrSpriteNotFound) {
		return err
	}
	if !looksLikeNotFound(err.Error()) {
		return err
	}
	return fmt.Errorf("%w: %w", ErrSpriteNotFound, err)
}

// IsNotFound reports whether err originated from an upstream "not found"
// response, even when WrapNotFound has not been applied. Useful for callers
// that receive a raw upstream error.
func IsNotFound(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, ErrSpriteNotFound) {
		return true
	}
	return looksLikeNotFound(err.Error())
}

func looksLikeNotFound(msg string) bool {
	lower := strings.ToLower(msg)
	return strings.Contains(lower, "sprite not found") ||
		strings.Contains(lower, "not found")
}
