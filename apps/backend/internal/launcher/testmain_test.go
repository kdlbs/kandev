package launcher

import (
	"os"
	"testing"

	"github.com/kandev/kandev/internal/common/config"
)

func TestMain(m *testing.M) {
	// Launcher subprocess wiring inherited from the managed task runner must
	// not pin tests to the runner's config file. Individual tests set these
	// internal variables when exercising environment propagation.
	_ = os.Unsetenv(config.InternalConfigFileEnv)
	_ = os.Unsetenv(config.InternalConfigHomeFileEnv)
	os.Exit(m.Run())
}
