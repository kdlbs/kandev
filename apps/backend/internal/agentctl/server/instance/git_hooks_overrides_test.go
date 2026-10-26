package instance

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestBuildInstanceOverridesCarriesGitHooks(t *testing.T) {
	m := &Manager{}
	overrides := m.buildInstanceOverrides("inst", &CreateRequest{GitHooks: map[string]bool{"": true, "sub": true}}, "", nil)
	assert.Equal(t, map[string]bool{"": true, "sub": true}, overrides.GitHooks)

	overrides = m.buildInstanceOverrides("inst", &CreateRequest{}, "", nil)
	assert.Empty(t, overrides.GitHooks)
}
