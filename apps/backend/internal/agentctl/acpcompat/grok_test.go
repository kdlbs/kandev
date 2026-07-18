package acpcompat

import (
	"math"
	"testing"
)

func TestNonNegativeInt64RejectsInvalidFloatTokenCounts(t *testing.T) {
	tests := []struct {
		name  string
		value any
		want  int64
		ok    bool
	}{
		{name: "zero", value: float64(0), want: 0, ok: true},
		{name: "whole number", value: float64(42), want: 42, ok: true},
		{name: "largest representable below int64 limit", value: math.Nextafter(math.Exp2(63), 0), want: 9_223_372_036_854_774_784, ok: true},
		{name: "negative", value: float64(-1)},
		{name: "fractional", value: 1.5},
		{name: "nan", value: math.NaN()},
		{name: "positive infinity", value: math.Inf(1)},
		{name: "int64 overflow", value: math.Exp2(63)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := nonNegativeInt64(map[string]any{"tokens": tt.value}, "tokens")
			if ok != tt.ok || got != tt.want {
				t.Fatalf("nonNegativeInt64() = (%d, %t), want (%d, %t)", got, ok, tt.want, tt.ok)
			}
		})
	}
}
