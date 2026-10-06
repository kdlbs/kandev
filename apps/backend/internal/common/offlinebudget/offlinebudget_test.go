package offlinebudget

import (
	"errors"
	"testing"
)

func TestResolve(t *testing.T) {
	for raw, want := range map[string]int{"": 15, "  ": 15, "30": 30, " 30 ": 30, "1": 1, "1440": 1440} {
		got, err := Resolve(raw)
		if err != nil || got != want {
			t.Fatalf("Resolve(%q) = %d, %v; want %d", raw, got, err, want)
		}
	}
	for _, raw := range []string{"0", "1441", "+30", "-5", "1.5", "abc", "3 0", "99999999999999999999"} {
		if _, err := Resolve(raw); !errors.Is(err, ErrInvalid) {
			t.Fatalf("Resolve(%q) error = %v, want ErrInvalid", raw, err)
		}
	}
}
