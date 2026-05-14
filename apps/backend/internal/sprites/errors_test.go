package sprites

import (
	"errors"
	"fmt"
	"testing"
)

func TestWrapNotFoundWraps404Errors(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{name: "sprite not found exact", err: fmt.Errorf("sprite not found: kandev-abc"), want: true},
		{name: "uppercase variant", err: fmt.Errorf("Sprite Not Found: kandev-abc"), want: true},
		{name: "wrapped not found", err: fmt.Errorf("get sprite: %w", fmt.Errorf("sprite not found: kandev-abc")), want: true},
		{name: "generic 404 mention", err: fmt.Errorf("HTTP 404: not found"), want: true},
		{name: "auth failure", err: fmt.Errorf("401 unauthorized"), want: false},
		{name: "transient 500", err: fmt.Errorf("internal server error"), want: false},
		{name: "nil", err: nil, want: false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			wrapped := WrapNotFound(tc.err)
			got := errors.Is(wrapped, ErrSpriteNotFound)
			if got != tc.want {
				t.Fatalf("errors.Is(WrapNotFound(%v), ErrSpriteNotFound) = %v, want %v", tc.err, got, tc.want)
			}
			if got != IsNotFound(tc.err) {
				t.Fatalf("IsNotFound(%v) disagrees with errors.Is on wrapped form", tc.err)
			}
		})
	}
}

func TestWrapNotFoundPreservesOriginalError(t *testing.T) {
	orig := fmt.Errorf("sprite not found: kandev-abc")
	wrapped := WrapNotFound(orig)

	if !errors.Is(wrapped, orig) {
		t.Fatalf("WrapNotFound dropped the original error from the chain")
	}
	if !errors.Is(wrapped, ErrSpriteNotFound) {
		t.Fatalf("WrapNotFound dropped the sentinel from the chain")
	}
}

func TestWrapNotFoundIsIdempotent(t *testing.T) {
	once := WrapNotFound(fmt.Errorf("sprite not found: kandev-abc"))
	twice := WrapNotFound(once)
	if once.Error() != twice.Error() {
		t.Fatalf("WrapNotFound is not idempotent: once=%q, twice=%q", once, twice)
	}
}

func TestWrapNotFoundNilPassthrough(t *testing.T) {
	if WrapNotFound(nil) != nil {
		t.Fatalf("WrapNotFound(nil) should return nil")
	}
}
