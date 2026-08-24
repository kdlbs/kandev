package lifecycle

import (
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
)

func TestReuseRequiredRemoteTaskDir(t *testing.T) {
	for _, test := range []struct {
		name   string
		req    *ExecutorCreateRequest
		want   string
		unsafe bool
	}{
		{name: "nil request", unsafe: true},
		{name: "missing directory", req: &ExecutorCreateRequest{}, unsafe: true},
		{name: "blank directory", req: &ExecutorCreateRequest{Metadata: map[string]interface{}{MetadataKeySSHRemoteTaskDir: "  "}}, unsafe: true},
		{name: "canonical directory", req: &ExecutorCreateRequest{Metadata: map[string]interface{}{MetadataKeySSHRemoteTaskDir: "/srv/kandev/task"}}, want: "/srv/kandev/task"},
	} {
		t.Run(test.name, func(t *testing.T) {
			got, err := reuseRequiredRemoteTaskDir(test.req)
			if test.unsafe {
				if !errors.Is(err, models.ErrWorkspaceReuseUnsafe) {
					t.Fatalf("error = %v, want workspace reuse unsafe", err)
				}
				return
			}
			if err != nil || got != test.want {
				t.Fatalf("reuseRequiredRemoteTaskDir = %q, %v; want %q, nil", got, err, test.want)
			}
		})
	}
}
