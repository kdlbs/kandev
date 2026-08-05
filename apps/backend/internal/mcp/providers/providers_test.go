package providers

import (
	"reflect"
	"testing"
)

func TestNormalize(t *testing.T) {
	tests := []struct {
		name   string
		values []string
		want   []string
	}{
		{name: "mixed and duplicate", values: []string{" GITLAB ", "github", "github"}, want: []string{GitHub, GitLab}},
		{name: "unsupported", values: []string{"local", "azure"}, want: []string{}},
		{name: "empty", values: nil, want: []string{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Normalize(tt.values)
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("Normalize(%q) = %v, want %v", tt.values, got, tt.want)
			}
		})
	}
}
