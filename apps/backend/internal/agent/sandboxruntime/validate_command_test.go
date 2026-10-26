package sandboxruntime

import (
	"runtime"
	"testing"

	contract "github.com/kandev/kandev/internal/common/sandbox"
	"github.com/stretchr/testify/require"
)

func TestValidateCommandEmptyApprovedValueIsNotEffectiveOnDarwin(t *testing.T) {
	p := &Prepared{
		launch:      &contract.Launch{EnvironmentKeys: []string{"API_KEY"}},
		constraints: contract.OverrideConstraints{ForbiddenEnvironment: []string{"API_KEY"}},
	}
	err := p.ValidateCommand(nil, map[string]string{"API_KEY": ""})
	if runtime.GOOS == "darwin" {
		require.NoError(t, err)
	} else {
		require.ErrorIs(t, err, contract.ErrInvalidPolicy)
	}
	require.ErrorIs(t, p.ValidateCommand(nil, map[string]string{"API_KEY": "v"}), contract.ErrInvalidPolicy)
}
