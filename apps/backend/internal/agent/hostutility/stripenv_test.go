package hostutility

import (
	"slices"
	"testing"

	"github.com/kandev/kandev/internal/agent/agents"
)

func TestStripEnvFor(t *testing.T) {
	t.Parallel()

	// DevinACP 声明 Runtime().StripEnv=["ACP_BACKEND"] → helper 返回该列表。
	got := stripEnvFor(agents.NewDevinACP())
	if want := []string{"ACP_BACKEND"}; !slices.Equal(got, want) {
		t.Errorf("stripEnvFor(DevinACP) = %v, want %v", got, want)
	}

	// 未声明 StripEnv 的 agent（QwenACP）→ nil。
	got = stripEnvFor(agents.NewQwenACP())
	if got != nil {
		t.Errorf("stripEnvFor(QwenACP) = %v, want nil", got)
	}
}
