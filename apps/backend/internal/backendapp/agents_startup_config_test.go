package backendapp

import (
	"testing"

	"github.com/kandev/kandev/internal/common/config"
)

func TestCredentialFilePathUsesTypedStartupConfig(t *testing.T) {
	cfg := &config.Config{}
	cfg.Credentials.File = "/srv/kandev/credentials.json"
	t.Setenv("KANDEV_CREDENTIALS_FILE", "/tmp/legacy.json")

	if got := credentialFilePath(cfg); got != "/srv/kandev/credentials.json" {
		t.Fatalf("credential file = %q, want typed startup path", got)
	}
}
