package launcher

import (
	"os"
	"testing"

	"github.com/kandev/kandev/internal/common/config"
)

const backupSignalHelperArg = "kandev-backup-signal-test-helper"
const backupSignalHelperEnv = "KANDEV_BACKUP_SIGNAL_TEST_HELPER"

var backupSignalHelper func() int

func TestMain(m *testing.M) {
	// Launcher subprocess wiring inherited from the managed task runner must
	// not pin tests to the runner's config file. Individual tests set these
	// internal variables when exercising environment propagation.
	_ = os.Unsetenv(config.InternalConfigFileEnv)
	_ = os.Unsetenv(config.InternalConfigHomeFileEnv)
	if len(os.Args) == 2 && os.Args[1] == backupSignalHelperArg && os.Getenv(backupSignalHelperEnv) == "1" && backupSignalHelper != nil {
		os.Exit(backupSignalHelper())
	}
	os.Exit(m.Run())
}
