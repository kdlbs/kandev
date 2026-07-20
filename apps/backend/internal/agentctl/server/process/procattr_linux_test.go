//go:build linux

package process

import "testing"

func TestProcessStatMatchesLiveGroup(t *testing.T) {
	tests := []struct {
		name string
		stat string
		want bool
	}{
		{name: "running member", stat: "123 (agent worker) S 1 42 42 0", want: true},
		{name: "zombie member", stat: "123 (agent worker) Z 1 42 42 0", want: false},
		{name: "dead member", stat: "123 (agent worker) X 1 42 42 0", want: false},
		{name: "different group", stat: "123 (agent worker) S 1 41 41 0", want: false},
		{name: "malformed", stat: "malformed", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := processStatMatchesLiveGroup(tt.stat, 42); got != tt.want {
				t.Fatalf("processStatMatchesLiveGroup() = %v, want %v", got, tt.want)
			}
		})
	}
}
