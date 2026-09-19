package routingerr

import "testing"

func TestShouldShortRetry(t *testing.T) {
	tests := []struct {
		name string
		err  *Error
		want bool
	}{
		{
			name: "transient auto-retryable fallback-allowed",
			err: &Error{
				Class:           ClassTransient,
				AutoRetryable:   true,
				FallbackAllowed: true,
			},
			want: true,
		},
		{
			name: "hard class",
			err: &Error{
				Class:           ClassHard,
				AutoRetryable:   true,
				FallbackAllowed: true,
			},
			want: false,
		},
		{
			name: "not auto-retryable",
			err: &Error{
				Class:           ClassTransient,
				AutoRetryable:   false,
				FallbackAllowed: true,
			},
			want: false,
		},
		{
			name: "fallback not allowed (cursor retriable-stream-reset shape)",
			err: &Error{
				Class:           ClassTransient,
				AutoRetryable:   true,
				FallbackAllowed: false,
			},
			want: false,
		},
		{
			name: "nil error",
			err:  nil,
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.err.ShouldShortRetry(); got != tt.want {
				t.Fatalf("ShouldShortRetry() = %v, want %v", got, tt.want)
			}
		})
	}
}
