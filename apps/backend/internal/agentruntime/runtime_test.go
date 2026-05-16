package agentruntime

import "testing"

func TestRuntimeIsContainerized(t *testing.T) {
	t.Parallel()

	cases := []struct {
		runtime Runtime
		want    bool
	}{
		{RuntimeStandalone, false},
		{RuntimeDocker, true},
		{RuntimeRemoteDocker, true},
		{RuntimeSprites, true},
		{Runtime(""), false},
		{Runtime("unknown"), false},
	}

	for _, tc := range cases {
		t.Run(string(tc.runtime), func(t *testing.T) {
			t.Parallel()
			if got := tc.runtime.IsContainerized(); got != tc.want {
				t.Fatalf("Runtime(%q).IsContainerized() = %v, want %v", tc.runtime, got, tc.want)
			}
		})
	}
}

func TestRuntimeString(t *testing.T) {
	t.Parallel()

	cases := []struct {
		runtime Runtime
		want    string
	}{
		{RuntimeStandalone, "standalone"},
		{RuntimeDocker, "docker"},
		{RuntimeRemoteDocker, "remote_docker"},
		{RuntimeSprites, "sprites"},
		{Runtime(""), ""},
	}

	for _, tc := range cases {
		t.Run(tc.want, func(t *testing.T) {
			t.Parallel()
			if got := tc.runtime.String(); got != tc.want {
				t.Fatalf("Runtime(%q).String() = %q, want %q", tc.runtime, got, tc.want)
			}
		})
	}
}
