package executor

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestResolveEnvironmentSources_DeduplicatesSameSecretAndIgnoresOrder(t *testing.T) {
	resolver := func(_ context.Context, source environmentSource) (string, error) {
		return map[string]string{"secret-a": "alpha", "secret-b": "bravo"}[source.secretID], nil
	}
	first := []environmentSource{
		{key: "TOKEN", secretID: "secret-a", origin: "repository app"},
		{key: "TOKEN", secretID: "secret-a", origin: "executor profile"},
		{key: "OTHER", secretID: "secret-b", origin: "repository tools"},
	}
	second := []environmentSource{first[2], first[0], first[1]}

	gotFirst, err := resolveEnvironmentSources(context.Background(), first, resolver)
	if err != nil {
		t.Fatalf("first resolve: %v", err)
	}
	gotSecond, err := resolveEnvironmentSources(context.Background(), second, resolver)
	if err != nil {
		t.Fatalf("second resolve: %v", err)
	}
	if len(gotFirst) != 2 || gotFirst["TOKEN"] != "alpha" || gotFirst["OTHER"] != "bravo" {
		t.Fatalf("first result = %#v", gotFirst)
	}
	if gotSecond["TOKEN"] != gotFirst["TOKEN"] || gotSecond["OTHER"] != gotFirst["OTHER"] {
		t.Fatalf("order changed result: first=%#v second=%#v", gotFirst, gotSecond)
	}
}

func TestResolveEnvironmentSources_RejectsEveryConflictingPair(t *testing.T) {
	cases := []struct {
		name  string
		first environmentSource
		last  environmentSource
	}{
		{name: "managed and executor literal", first: environmentSource{key: "TOKEN", literal: "managed", origin: "managed/runtime"}, last: environmentSource{key: "TOKEN", literal: "profile", origin: "executor profile"}},
		{name: "managed and repository secret", first: environmentSource{key: "TOKEN", literal: "managed", origin: "managed/runtime"}, last: environmentSource{key: "TOKEN", secretID: "secret-a", origin: "repository app"}},
		{name: "executor and repository secrets", first: environmentSource{key: "TOKEN", secretID: "secret-a", origin: "executor profile"}, last: environmentSource{key: "TOKEN", secretID: "secret-b", origin: "repository app"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := resolveEnvironmentSources(context.Background(), []environmentSource{tc.first, tc.last}, func(context.Context, environmentSource) (string, error) {
				return "not-used", nil
			})
			if err == nil {
				t.Fatal("resolve succeeded, want conflict")
			}
			var conflictErr *EnvironmentConflictError
			if !errors.As(err, &conflictErr) {
				t.Fatalf("error = %T %v, want EnvironmentConflictError", err, err)
			}
			for _, leaked := range []string{"secret-a", "secret-b", "not-used"} {
				if strings.Contains(err.Error(), leaked) {
					t.Fatalf("error %q leaks %q", err, leaked)
				}
			}
		})
	}
}

func TestResolveEnvironmentSources_RedactsFailedSecretReference(t *testing.T) {
	_, err := resolveEnvironmentSources(context.Background(), []environmentSource{{
		key: "TOKEN", secretID: "secret-private", origin: "repository app",
	}}, func(context.Context, environmentSource) (string, error) {
		return "", errors.New("secret-private backend failure")
	})
	if err == nil || !strings.Contains(err.Error(), "TOKEN") || !strings.Contains(err.Error(), "repository app") {
		t.Fatalf("error = %v, want key and origin", err)
	}
	if strings.Contains(err.Error(), "secret-private") || strings.Contains(err.Error(), "backend failure") {
		t.Fatalf("error %q leaks secret details", err)
	}
}
