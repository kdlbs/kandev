package config

import (
	"os"
	"testing"
)

func TestMain(m *testing.M) {
	// These variables pin a managed child process to its launcher's config.
	// They are injected by the Kandev test workspace and must not override the
	// temporary files created by this package's config discovery tests.
	_ = os.Unsetenv(InternalConfigFileEnv)
	_ = os.Unsetenv(InternalConfigHomeFileEnv)
	os.Exit(m.Run())
}
