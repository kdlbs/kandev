package backendapp

import (
	"testing"

	userdto "github.com/kandev/kandev/internal/user/dto"
)

func TestMapUserSettingsStateIncludesSidebarPresentation(t *testing.T) {
	for _, style := range []string{"simple", "compact"} {
		for _, fast := range []bool{false, true} {
			state := mapUserSettingsState(userdto.UserSettingsResponse{Settings: userdto.UserSettingsDTO{
				SidebarFastActionsEnabled: fast, SidebarNewTaskStyle: style,
			}}, "workspace")
			if state["sidebarFastActionsEnabled"] != fast || state["sidebarNewTaskStyle"] != style {
				t.Fatalf("presentation boot state = %v, want fast=%v style=%s", state, fast, style)
			}
		}
	}
}
