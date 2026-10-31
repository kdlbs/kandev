package models

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRequiredSubtaskCountCannotBeClearedByGenericMetadataUpdate(t *testing.T) {
	for _, requested := range []map[string]interface{}{
		nil, {MetaKeyRequiredSubtaskCount: 0}, {MetaKeyRequiredSubtaskCount: 1},
	} {
		got := ProtectedTaskMetadataUpdate(map[string]interface{}{MetaKeyRequiredSubtaskCount: 3}, requested)
		require.Equal(t, 3, got[MetaKeyRequiredSubtaskCount])
	}
	got := ProtectedTaskMetadataUpdate(nil, map[string]interface{}{MetaKeyRequiredSubtaskCount: 3})
	require.NotContains(t, got, MetaKeyRequiredSubtaskCount)
	require.NotContains(t, got, "required_subtask_count")
}
