package gitlab

import (
	"testing"

	"go.uber.org/goleak"
)

func TestMain(m *testing.M) {
	clearAmbientGitLabEnv()
	goleak.VerifyTestMain(m)
}
