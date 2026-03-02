package sprites

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNormalizeSpriteStatus(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{name: "running", input: "Running", want: "running"},
		{name: "cold", input: "Cold", want: "cold"},
		{name: "stopped", input: "STOPPED", want: "stopped"},
		{name: "with whitespace", input: "  Running  ", want: "running"},
		{name: "empty string", input: "", want: "unknown"},
		{name: "whitespace only", input: "   ", want: "unknown"},
		{name: "already lowercase", input: "starting", want: "starting"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NormalizeSpriteStatus(tt.input)
			require.Equal(t, tt.want, got)
		})
	}
}
