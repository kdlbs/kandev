package orchestrator

import (
	"runtime"
	"strconv"
	"testing"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

func TestDefaultSessionCeilingIsHalfTheCoresWithAFloorOfTwo(t *testing.T) {
	for _, tc := range []struct {
		numCPU int
		want   int
	}{
		{numCPU: 1, want: 2},
		{numCPU: 2, want: 2},
		{numCPU: 3, want: 2},
		{numCPU: 4, want: 2},
		{numCPU: 8, want: 4},
		{numCPU: 14, want: 7},
		{numCPU: 64, want: 32},
	} {
		if got := defaultSessionCeiling(tc.numCPU); got != tc.want {
			t.Errorf("defaultSessionCeiling(%d) = %d, want %d", tc.numCPU, got, tc.want)
		}
	}
}

func TestResolveSessionCeilingUsesTheEnvironmentValue(t *testing.T) {
	for _, tc := range []struct {
		name string
		raw  string
		want int
	}{
		{name: "zero is unlimited", raw: "0", want: unlimitedSessionCeiling},
		{name: "one", raw: "1", want: 1},
		{name: "twenty", raw: "20", want: 20},
		{name: "surrounding whitespace is tolerated", raw: "  6 ", want: 6},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, rejected := resolveSessionCeiling(tc.raw, 8)
			if got != tc.want {
				t.Errorf("resolveSessionCeiling(%q) ceiling = %d, want %d", tc.raw, got, tc.want)
			}
			if rejected != "" {
				t.Errorf("resolveSessionCeiling(%q) rejected = %q, want no rejection", tc.raw, rejected)
			}
		})
	}
}

// An unset or blank variable is the ordinary "no override" case: it takes the
// default and is not reported as a rejection, so it must not produce a WARN.
func TestResolveSessionCeilingFallsThroughSilentlyWhenUnset(t *testing.T) {
	for _, raw := range []string{"", "   "} {
		got, rejected := resolveSessionCeiling(raw, 8)
		if want := defaultSessionCeiling(8); got != want {
			t.Errorf("resolveSessionCeiling(%q) ceiling = %d, want default %d", raw, got, want)
		}
		if rejected != "" {
			t.Errorf("resolveSessionCeiling(%q) rejected = %q, want no rejection", raw, rejected)
		}
	}
}

// A supplied-but-unusable value takes the default AND reports the raw value, which
// is what the caller logs at WARN. Reporting it is the whole difference from unset.
func TestResolveSessionCeilingRejectsUnusableValues(t *testing.T) {
	for _, raw := range []string{"-1", "-20", "abc", "4.5", "1e3", "٤", "9999999999999999999999"} {
		got, rejected := resolveSessionCeiling(raw, 8)
		if want := defaultSessionCeiling(8); got != want {
			t.Errorf("resolveSessionCeiling(%q) ceiling = %d, want default %d", raw, got, want)
		}
		if rejected != raw {
			t.Errorf("resolveSessionCeiling(%q) rejected = %q, want %q", raw, rejected, raw)
		}
	}
}

// The production default must be reachable through the same path the tests pin.
func TestResolveSessionCeilingDefaultMatchesThisHost(t *testing.T) {
	got, _ := resolveSessionCeiling("", runtime.NumCPU())
	want := defaultSessionCeiling(runtime.NumCPU())
	if got != want {
		t.Fatalf("resolveSessionCeiling(\"\") = %d, want %d", got, want)
	}
	if want < 2 {
		t.Fatalf("default ceiling %s is below the floor of 2", strconv.Itoa(want))
	}
}

// AC-18a: the ceiling is resolved from maxConcurrentSessionsEnvVar once, at
// controller construction, rather than read ad hoc by callers.
func TestNewSessionCeilingControllerFromEnvUsesTheOperatorValue(t *testing.T) {
	t.Setenv(maxConcurrentSessionsEnvVar, "3")
	c := newSessionCeilingControllerFromEnv(nil, zap.NewNop())
	if c.ceiling != 3 {
		t.Fatalf("ceiling = %d, want 3", c.ceiling)
	}
}

func TestNewSessionCeilingControllerFromEnvFallsBackToDefaultWhenUnset(t *testing.T) {
	t.Setenv(maxConcurrentSessionsEnvVar, "")
	c := newSessionCeilingControllerFromEnv(nil, zap.NewNop())
	want := defaultSessionCeiling(runtime.NumCPU())
	if c.ceiling != want {
		t.Fatalf("ceiling = %d, want default %d", c.ceiling, want)
	}
}

// AC-20: a set-but-unparseable value takes the default and is reported at WARN,
// distinguishing it from the silent unset case above.
func TestNewSessionCeilingControllerFromEnvWarnsOnRejectedValue(t *testing.T) {
	t.Setenv(maxConcurrentSessionsEnvVar, "not-a-number")
	core, logs := observer.New(zapcore.WarnLevel)
	c := newSessionCeilingControllerFromEnv(nil, zap.New(core))

	want := defaultSessionCeiling(runtime.NumCPU())
	if c.ceiling != want {
		t.Fatalf("ceiling = %d, want default %d", c.ceiling, want)
	}
	if n := logs.FilterMessageSnippet("could not be parsed").Len(); n != 1 {
		t.Fatalf("expected exactly one WARN about the rejected value, got %d: %v", n, logs.All())
	}
}
