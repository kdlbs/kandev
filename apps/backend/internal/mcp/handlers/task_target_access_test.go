package handlers

import (
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/assert"
)

func TestCanDirectParentAccess(t *testing.T) {
	parent := &models.Task{ID: "parent", WorkspaceID: "workspace"}
	for _, tt := range []struct {
		name   string
		target *models.Task
		want   bool
	}{
		{name: "direct child", target: &models.Task{ParentID: "parent", WorkspaceID: "workspace"}, want: true},
		{name: "self", target: parent, want: false},
		{name: "sibling", target: &models.Task{ParentID: "other", WorkspaceID: "workspace"}, want: false},
		{name: "grandchild", target: &models.Task{ParentID: "child", WorkspaceID: "workspace"}, want: false},
		{name: "other workspace", target: &models.Task{ParentID: "parent", WorkspaceID: "other"}, want: false},
		{name: "empty workspace", target: &models.Task{ParentID: "parent"}, want: false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, canDirectParentAccess(parent, tt.target))
		})
	}
}
