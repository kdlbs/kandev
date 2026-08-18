package handlers

import (
	"testing"

	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/stretchr/testify/assert"
)

func TestRejectOperatorConfigKeys_RejectsReservedKey(t *testing.T) {
	err := rejectOperatorConfigKeys(map[string]string{
		lifecycle.MetadataKeyAllowUserNamespaces: "true",
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), lifecycle.MetadataKeyAllowUserNamespaces)
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


