package profileconfig

import (
	"reflect"
	"testing"
)

func TestSanitizeConfigOptions(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   map[string]string
		want map[string]string
	}{
		{name: "nil input returns nil", in: nil, want: nil},
		{name: "empty returns nil", in: map[string]string{}, want: nil},
		{
			name: "reserved model key dropped",
			in:   map[string]string{"model": "opus", "effort": "high"},
			want: map[string]string{"effort": "high"},
		},
		{
			name: "reserved mode key dropped",
			in:   map[string]string{"mode": "plan", "effort": "low"},
			want: map[string]string{"effort": "low"},
		},
		{
			name: "blank value dropped",
			in:   map[string]string{"effort": "", "level": "high"},
			want: map[string]string{"level": "high"},
		},
		{
			name: "blank key dropped",
			in:   map[string]string{"": "x", "level": "high"},
			want: map[string]string{"level": "high"},
		},
		{
			name: "whitespace trimmed",
			in:   map[string]string{" effort ": " high "},
			want: map[string]string{"effort": "high"},
		},
		{
			name: "all reserved returns nil",
			in:   map[string]string{"model": "opus", "mode": "plan"},
			want: nil,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := SanitizeConfigOptions(tc.in)
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
		})
	}
}
