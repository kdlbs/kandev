package handlers

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRejectOperatorConfigKeys_RejectsReservedKey(t *testing.T) {
	err := rejectOperatorConfigKeys(map[string]string{
		allowUserNamespacesProfileConfigKey: "true",
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), allowUserNamespacesProfileConfigKey)
}

func TestRejectOperatorConfigKeys_AcceptsNormalKeys(t *testing.T) {
	err := rejectOperatorConfigKeys(map[string]string{
		"image_tag": "kandev/agent:latest",
	})
	assert.NoError(t, err)
}

func TestRejectOperatorConfigKeys_AcceptsEmptyConfig(t *testing.T) {
	err := rejectOperatorConfigKeys(map[string]string{})
	assert.NoError(t, err)
}

func TestRejectOperatorConfigKeys_AcceptsNilConfig(t *testing.T) {
	err := rejectOperatorConfigKeys(nil)
	assert.NoError(t, err)
}
