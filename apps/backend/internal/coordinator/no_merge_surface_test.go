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

// mergeSurface is the one merge entry the surface may name: the guarded merge
// proposal tool of the merge spec. Every other name that suggests a merge or a
// completion stays forbidden.
var mergeSurface = map[string]bool{
	"coordinator.propose_merge": true,
	"propose_merge_kandev":      true,
}

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
		if forbiddenSurface.MatchString(name) && !mergeSurface[name] {
			t.Errorf("coordinator surface name %q suggests a merge or completion", name)
		}
	}
}

func TestCoordinatorRoutesNameNoMergeOrCompletion(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc, _ := newTestServiceForSubscribers(t)
	svc.phase2 = true
	svc.merge = true
	router := gin.New()
	RegisterRoutes(router, svc, newTestLogger(t))
	routes := router.Routes()
	if len(routes) == 0 {
		t.Fatal("no routes registered")
	}
	for _, r := range routes {
		path := strings.TrimPrefix(r.Path, "/api/v1/workspaces/:id")
		if forbiddenSurface.MatchString(path) && !mergeRoute(path) {
			t.Errorf("route %s %s suggests a merge or completion", r.Method, r.Path)
		}
	}
}

// mergeRoute reports whether a path belongs to the merge spec: the protection
// refresh and the two manager actions on an uncertain merge proposal.
func mergeRoute(path string) bool {
	return strings.Contains(path, "/merge/protection/") || strings.HasSuffix(path, "/merge/check") || strings.HasSuffix(path, "/merge/release")
}
