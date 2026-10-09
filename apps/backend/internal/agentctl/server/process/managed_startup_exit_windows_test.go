//go:build windows

package process

import "testing"

func TestIsWindowsNTStatusExceptionExitCode(t *testing.T) {
	tests := []struct {
		name string
		code uint32
		want bool
	}{
		{name: "access violation", code: 0xc0000005, want: true},
		{name: "illegal instruction", code: 0xc000001d, want: true},
		{name: "ordinary exit", code: 1, want: false},
		{name: "success", code: 0, want: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := isWindowsNTStatusExceptionExitCode(test.code); got != test.want {
				t.Fatalf("isWindowsNTStatusExceptionExitCode(%#x) = %v, want %v", test.code, got, test.want)
			}
		})
	}
}
