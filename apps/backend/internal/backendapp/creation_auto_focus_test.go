package backendapp

import (
	userdto "github.com/kandev/kandev/internal/user/dto"
	"testing"
)

func TestAutoFocusNewTasksBootProjection(t *testing.T) {
	for _, enabled := range []bool{true, false} {
		got := mapUserSettingsState(userdto.UserSettingsResponse{Settings: userdto.UserSettingsDTO{AutoFocusNewTasks: enabled}}, "workspace")
		if got["autoFocusNewTasks"] != enabled {
			t.Fatalf("boot autoFocusNewTasks = %#v, want %v", got["autoFocusNewTasks"], enabled)
		}
	}
}
