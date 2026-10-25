package process

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

func TestParseGitUntrackedOutput(t *testing.T) {
	baseCtx, cancel := context.WithCancel(context.Background())
	cancel()

	tests := []struct {
		name    string
		ctx     context.Context
		output  []byte
		want    []string
		wantErr error
	}{
		{
			name:   "nul separates paths with embedded newline",
			ctx:    context.Background(),
			output: []byte("directory/line\nbreak.txt\x00plain.txt\x00"),
			want:   []string{"directory/line\nbreak.txt", "plain.txt"},
		},
		{
			name:    "cancelled context",
			ctx:     baseCtx,
			output:  []byte("ignored.txt\x00"),
			wantErr: context.Canceled,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseGitUntrackedOutput(tt.ctx, tt.output)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("parseGitUntrackedOutput() error = %v, want %v", err, tt.wantErr)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("parseGitUntrackedOutput() = %#v, want %#v", got, tt.want)
			}
		})
	}
}
