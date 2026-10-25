package environment

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// @covers AC-WORKSPACES-REPOSITORY-SECRETS-001.10
func TestSecretRecoveryNamesSourceAndRepair(t *testing.T) {
	for _, origin := range []string{OriginAgentProfile, OriginExecutorProfile, RepositoryOrigin("app")} {
		t.Run(origin, func(t *testing.T) {
			cause := errors.New("missing private-secret-id with private-value")
			env, records, err := Resolve(context.Background(), []Definition{
				{Key: "MY_TOKEN", SecretID: "private-secret-id", Origin: origin},
			}, func(context.Context, Definition) (string, error) { return "", cause })
			if err == nil || env != nil || records != nil || !errors.Is(err, cause) {
				t.Fatalf("resolution = %v, %v, %v", env, records, err)
			}
			for _, want := range []string{"MY_TOKEN", origin, "Re-select"} {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("error %q does not contain %q", err, want)
				}
			}
			if strings.Contains(err.Error(), "private-") {
				t.Fatalf("error leaked secret details: %v", err)
			}
		})
	}
}
