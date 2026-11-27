package coordinator

import (
	"regexp"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

// forbiddenSurface matches a name that would merge a pull request or move a
// task to a completing step.
var forbiddenSurface = regexp.MustCompile(`(?i)merge|complete|done|finish`)

func TestCoordinatorSurfaceNamesNoMergeOrCompletion(t *testing.T) {
	allAllowed := Policy{Version: 1, Actions: map[Action]Setting{}}
	for _, a := range AllActions {
		allAllowed.Actions[a] = SettingRequiresApproval
	}
	var names []string
	names = append(names, ToolNames(allAllowed, true, false)...)
	for action, tool := range actionTools {
		names = append(names, action, tool)
	}
	for _, a := range AllActions {
		names = append(names, string(a))
	}
	for _, name := range names {
		if forbiddenSurface.MatchString(name) {
			t.Errorf("coordinator surface name %q suggests a merge or completion", name)
		}
	}
}

func TestCoordinatorRoutesNameNoMergeOrCompletion(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc, _ := newTestServiceForSubscribers(t)
	svc.phase2 = true
	router := gin.New()
	RegisterRoutes(router, svc, newTestLogger(t))
	routes := router.Routes()
	if len(routes) == 0 {
		t.Fatal("no routes registered")
	}
	for _, r := range routes {
		if forbiddenSurface.MatchString(strings.TrimPrefix(r.Path, "/api/v1/workspaces/:id")) {
			t.Errorf("route %s %s suggests a merge or completion", r.Method, r.Path)
		}
	}
}
