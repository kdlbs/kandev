package main

import "testing"

func TestViteIndexHasEntrypoint(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		html string
		want bool
	}{
		{
			name: "vite dist index",
			html: `<!doctype html><html><head><script type="module" crossorigin src="/assets/index-abc123.js"></script></head><body><div id="root"></div></body></html>`,
			want: true,
		},
		{
			name: "fallback shell has no app entrypoint",
			html: `<!doctype html><html><head><title>Kandev</title></head><body><div id="root"></div></body></html>`,
			want: false,
		},
		{
			name: "source index still points at vite dev source",
			html: `<!doctype html><html><body><div id="root"></div><script type="module" src="/src/main.tsx"></script></body></html>`,
			want: false,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if got := viteIndexHasEntrypoint(tc.html); got != tc.want {
				t.Fatalf("viteIndexHasEntrypoint() = %v, want %v", got, tc.want)
			}
		})
	}
}
