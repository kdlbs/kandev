package docker

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/common/logger"
	"go.uber.org/zap"
)

// fakeDockerAPI stands in for *Client so handler tests need no daemon.
type fakeDockerAPI struct {
	containers []ContainerInfo
	listErr    error

	listCalls int
	inspected []string
	stopped   []string
	removed   []string
	builtTags []string
}

func (f *fakeDockerAPI) find(containerID string) (ContainerInfo, bool) {
	for _, ctr := range f.containers {
		if ctr.ID == containerID {
			return ctr, true
		}
	}
	return ContainerInfo{}, false
}

func (f *fakeDockerAPI) BuildImage(_ context.Context, _ string, tag string, _ map[string]*string) (io.ReadCloser, error) {
	f.builtTags = append(f.builtTags, tag)
	return io.NopCloser(strings.NewReader(`{"stream":"built"}`)), nil
}

func (f *fakeDockerAPI) ListContainers(_ context.Context, _ map[string]string) ([]ContainerInfo, error) {
	f.listCalls++
	if f.listErr != nil {
		return nil, f.listErr
	}
	return f.containers, nil
}

func (f *fakeDockerAPI) GetContainerLabels(_ context.Context, containerID string) (map[string]string, error) {
	f.inspected = append(f.inspected, containerID)
	ctr, ok := f.find(containerID)
	if !ok {
		return nil, errors.New("no such container: " + containerID)
	}
	return ctr.Labels, nil
}

func (f *fakeDockerAPI) StopContainer(_ context.Context, containerID string, _ time.Duration) error {
	if _, ok := f.find(containerID); !ok {
		return errors.New("no such container: " + containerID)
	}
	f.stopped = append(f.stopped, containerID)
	return nil
}

func (f *fakeDockerAPI) RemoveContainer(_ context.Context, containerID string, _ bool) error {
	if _, ok := f.find(containerID); !ok {
		return errors.New("no such container: " + containerID)
	}
	f.removed = append(f.removed, containerID)
	return nil
}

// fakeAuthorizer mirrors task/service's scoping contract: no identity or a
// synthetic identity (auth disabled) is unscoped, a real identity sees only
// the sessions and tasks it owns, and denials are indistinguishable from
// missing rows.
type fakeAuthorizer struct {
	sessionOwners map[string]string
	taskOwners    map[string]string
}

var errFakeNotFound = errors.New("not found")

func (f *fakeAuthorizer) scope(ctx context.Context) (string, bool) {
	identity, ok := authn.IdentityFromContext(ctx)
	if !ok || identity.Synthetic {
		return "", false
	}
	return identity.UserID, true
}

func (f *fakeAuthorizer) AuthorizeSessionAccess(ctx context.Context, sessionID string) error {
	userID, scoped := f.scope(ctx)
	if !scoped {
		return nil
	}
	owner, ok := f.sessionOwners[sessionID]
	if !ok || owner != userID {
		return errFakeNotFound
	}
	return nil
}

func (f *fakeAuthorizer) AuthorizeTaskAccess(ctx context.Context, taskID string) error {
	userID, scoped := f.scope(ctx)
	if !scoped {
		return nil
	}
	owner, ok := f.taskOwners[taskID]
	if !ok || owner != userID {
		return errFakeNotFound
	}
	return nil
}

// titleRecorder records every task ID whose title was resolved, so tests can
// prove a foreign container's title is never looked up (filtering after
// resolution would still leak titles into logs and metrics).
type titleRecorder struct {
	titles  map[string]string
	queried []string
}

func (r *titleRecorder) provider() TaskTitleProvider {
	return func(_ context.Context, taskID string) (string, bool) {
		r.queried = append(r.queried, taskID)
		title, ok := r.titles[taskID]
		return title, ok
	}
}

func (r *titleRecorder) queriedTask(taskID string) bool {
	for _, seen := range r.queried {
		if seen == taskID {
			return true
		}
	}
	return false
}

func kandevContainer(id, taskID, sessionID string) ContainerInfo {
	return ContainerInfo{
		ID:     id,
		Name:   "kandev-agent-" + id,
		Image:  "kandev/agent:test",
		State:  "running",
		Status: "Up 2 seconds",
		Labels: map[string]string{
			"kandev.managed":    "true",
			"kandev.task_id":    taskID,
			"kandev.session_id": sessionID,
		},
	}
}

func testLogger(t *testing.T) *logger.Logger {
	t.Helper()
	log, err := logger.NewFromZap(zap.NewNop())
	if err != nil {
		t.Fatalf("logger: %v", err)
	}
	return log
}

// dockerTestRouter registers the Docker routes against fakes. A nil identity
// means no auth middleware ran at all.
func dockerTestRouter(
	t *testing.T, client containerAPI, titles TaskTitleProvider,
	authorizer SessionAuthorizer, identity *authn.Identity,
) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	router := gin.New()
	if identity != nil {
		router.Use(func(c *gin.Context) {
			authn.SetOnGin(c, *identity)
			c.Next()
		})
	}
	registerRoutes(router, func() containerAPI { return client }, titles, authorizer, testLogger(t))
	return router
}

func member(userID string) *authn.Identity {
	return &authn.Identity{UserID: userID, Role: authn.RoleMember}
}

func do(router *gin.Engine, method, path string, body string) *httptest.ResponseRecorder {
	var reader io.Reader
	if body != "" {
		reader = strings.NewReader(body)
	}
	request := httptest.NewRequest(method, path, reader)
	if body != "" {
		request.Header.Set("Content-Type", "application/json")
	}
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	return response
}

func twoUserFixture() (*fakeDockerAPI, *fakeAuthorizer, *titleRecorder) {
	client := &fakeDockerAPI{containers: []ContainerInfo{
		kandevContainer("ctr-a", "task-a", "session-a"),
		kandevContainer("ctr-b", "task-b", "session-b"),
	}}
	authorizer := &fakeAuthorizer{
		sessionOwners: map[string]string{"session-a": "user-a", "session-b": "user-b"},
		taskOwners:    map[string]string{"task-a": "user-a", "task-b": "user-b"},
	}
	titles := &titleRecorder{titles: map[string]string{"task-a": "Alice Task", "task-b": "Bob Task"}}
	return client, authorizer, titles
}

func TestListContainersHidesForeignContainersAndTitles(t *testing.T) {
	client, authorizer, titles := twoUserFixture()
	router := dockerTestRouter(t, client, titles.provider(), authorizer, member("user-b"))

	response := do(router, http.MethodGet, "/api/v1/docker/containers", "")

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	body := response.Body.String()
	if strings.Contains(body, "ctr-a") || strings.Contains(body, "task-a") || strings.Contains(body, "Alice Task") {
		t.Fatalf("foreign container leaked into listing: %s", body)
	}
	if !strings.Contains(body, "ctr-b") {
		t.Fatalf("own container missing from listing: %s", body)
	}
	if titles.queriedTask("task-a") {
		t.Fatalf("task title resolved for foreign container: %v", titles.queried)
	}
}

func TestListContainersDropsContainersWithoutResolvableOwner(t *testing.T) {
	client, authorizer, titles := twoUserFixture()
	client.containers = append(client.containers, ContainerInfo{
		ID: "ctr-unlabeled", Name: "stray", State: "running",
	})
	router := dockerTestRouter(t, client, titles.provider(), authorizer, member("user-b"))

	response := do(router, http.MethodGet, "/api/v1/docker/containers", "")

	if strings.Contains(response.Body.String(), "ctr-unlabeled") {
		t.Fatalf("unowned container leaked into listing: %s", response.Body.String())
	}
}

func TestStopForeignContainerIsNotFoundAndDoesNotStop(t *testing.T) {
	client, authorizer, titles := twoUserFixture()
	router := dockerTestRouter(t, client, titles.provider(), authorizer, member("user-b"))

	response := do(router, http.MethodPost, "/api/v1/docker/containers/ctr-a/stop", "")

	if response.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404; body = %s", response.Code, response.Body.String())
	}
	if len(client.stopped) != 0 {
		t.Fatalf("docker StopContainer was invoked: %v", client.stopped)
	}
}

func TestRemoveForeignContainerIsNotFoundAndDoesNotRemove(t *testing.T) {
	client, authorizer, titles := twoUserFixture()
	router := dockerTestRouter(t, client, titles.provider(), authorizer, member("user-b"))

	response := do(router, http.MethodDelete, "/api/v1/docker/containers/ctr-a", "")

	if response.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404; body = %s", response.Code, response.Body.String())
	}
	if len(client.removed) != 0 {
		t.Fatalf("docker RemoveContainer was invoked: %v", client.removed)
	}
}

func TestForeignContainerIsIndistinguishableFromMissingContainer(t *testing.T) {
	client, authorizer, titles := twoUserFixture()
	router := dockerTestRouter(t, client, titles.provider(), authorizer, member("user-b"))

	cases := []struct{ method, foreign, missing string }{
		{http.MethodPost, "/api/v1/docker/containers/ctr-a/stop", "/api/v1/docker/containers/ctr-nope/stop"},
		{http.MethodDelete, "/api/v1/docker/containers/ctr-a", "/api/v1/docker/containers/ctr-nope"},
	}
	for _, tc := range cases {
		foreign := do(router, tc.method, tc.foreign, "")
		missing := do(router, tc.method, tc.missing, "")
		if foreign.Code != missing.Code {
			t.Fatalf("%s: foreign status %d != missing status %d", tc.method, foreign.Code, missing.Code)
		}
		if foreign.Body.String() != missing.Body.String() {
			t.Fatalf("%s: foreign body %q != missing body %q", tc.method, foreign.Body.String(), missing.Body.String())
		}
	}
}

func TestOwnerCanListStopAndRemoveOwnContainer(t *testing.T) {
	client, authorizer, titles := twoUserFixture()
	router := dockerTestRouter(t, client, titles.provider(), authorizer, member("user-a"))

	list := do(router, http.MethodGet, "/api/v1/docker/containers", "")
	if list.Code != http.StatusOK || !strings.Contains(list.Body.String(), "Alice Task") {
		t.Fatalf("owner listing = %d %s", list.Code, list.Body.String())
	}

	stop := do(router, http.MethodPost, "/api/v1/docker/containers/ctr-a/stop", `{"timeout_seconds":5}`)
	if stop.Code != http.StatusOK {
		t.Fatalf("owner stop status = %d, body = %s", stop.Code, stop.Body.String())
	}
	if len(client.stopped) != 1 || client.stopped[0] != "ctr-a" {
		t.Fatalf("stopped = %v, want [ctr-a]", client.stopped)
	}

	remove := do(router, http.MethodDelete, "/api/v1/docker/containers/ctr-a", "")
	if remove.Code != http.StatusOK {
		t.Fatalf("owner remove status = %d, body = %s", remove.Code, remove.Body.String())
	}
	if len(client.removed) != 1 || client.removed[0] != "ctr-a" {
		t.Fatalf("removed = %v, want [ctr-a]", client.removed)
	}
}

func TestBuildImageIsAdminOnly(t *testing.T) {
	client, authorizer, titles := twoUserFixture()
	const body = `{"dockerfile":"FROM scratch","tag":"evil:latest"}`

	memberRouter := dockerTestRouter(t, client, titles.provider(), authorizer, member("user-b"))
	memberResponse := do(memberRouter, http.MethodPost, "/api/v1/docker/build", body)
	if memberResponse.Code != http.StatusForbidden {
		t.Fatalf("member build status = %d, want 403; body = %s", memberResponse.Code, memberResponse.Body.String())
	}
	if len(client.builtTags) != 0 {
		t.Fatalf("member build reached Docker: %v", client.builtTags)
	}

	adminIdentity := &authn.Identity{UserID: "admin-1", Role: authn.RoleAdmin}
	adminRouter := dockerTestRouter(t, client, titles.provider(), authorizer, adminIdentity)
	adminResponse := do(adminRouter, http.MethodPost, "/api/v1/docker/build", body)
	if adminResponse.Code != http.StatusOK {
		t.Fatalf("admin build status = %d, want 200; body = %s", adminResponse.Code, adminResponse.Body.String())
	}
	if len(client.builtTags) != 1 || client.builtTags[0] != "evil:latest" {
		t.Fatalf("builtTags = %v, want [evil:latest]", client.builtTags)
	}
}

// TestAuthDisabledLeavesEveryRouteUnchanged pins the single-user contract: the
// synthetic identity injected while authentication is disabled must behave
// exactly like the pre-fix handlers.
func TestAuthDisabledLeavesEveryRouteUnchanged(t *testing.T) {
	client, authorizer, titles := twoUserFixture()
	client.containers = append(client.containers, ContainerInfo{ID: "ctr-unlabeled", Name: "stray", State: "running"})
	synthetic := &authn.Identity{UserID: "single-user", Role: authn.RoleAdmin, Synthetic: true}
	router := dockerTestRouter(t, client, titles.provider(), authorizer, synthetic)

	list := do(router, http.MethodGet, "/api/v1/docker/containers", "")
	if list.Code != http.StatusOK {
		t.Fatalf("list status = %d, body = %s", list.Code, list.Body.String())
	}
	for _, want := range []string{"ctr-a", "ctr-b", "ctr-unlabeled", "Alice Task", "Bob Task"} {
		if !strings.Contains(list.Body.String(), want) {
			t.Fatalf("single-user listing missing %q: %s", want, list.Body.String())
		}
	}

	stop := do(router, http.MethodPost, "/api/v1/docker/containers/ctr-a/stop", "")
	if stop.Code != http.StatusOK || len(client.stopped) != 1 {
		t.Fatalf("single-user stop = %d %s, stopped = %v", stop.Code, stop.Body.String(), client.stopped)
	}
	remove := do(router, http.MethodDelete, "/api/v1/docker/containers/ctr-b", "")
	if remove.Code != http.StatusOK || len(client.removed) != 1 {
		t.Fatalf("single-user remove = %d %s, removed = %v", remove.Code, remove.Body.String(), client.removed)
	}
	build := do(router, http.MethodPost, "/api/v1/docker/build", `{"dockerfile":"FROM scratch","tag":"local:dev"}`)
	if build.Code != http.StatusOK {
		t.Fatalf("single-user build status = %d, body = %s", build.Code, build.Body.String())
	}

	// A missing container still surfaces the daemon's failure, not a 404, and
	// no ownership inspect happens on the unscoped path.
	missing := do(router, http.MethodPost, "/api/v1/docker/containers/ctr-nope/stop", "")
	if missing.Code != http.StatusInternalServerError {
		t.Fatalf("single-user stop of missing container = %d, want 500; body = %s", missing.Code, missing.Body.String())
	}
	if len(client.inspected) != 0 {
		t.Fatalf("unscoped path inspected containers: %v", client.inspected)
	}
}

// TestNoIdentityIsUnscoped covers internal callers (no auth middleware in the
// chain), which must keep the pre-auth behavior.
func TestNoIdentityIsUnscoped(t *testing.T) {
	client, authorizer, titles := twoUserFixture()
	router := dockerTestRouter(t, client, titles.provider(), authorizer, nil)

	list := do(router, http.MethodGet, "/api/v1/docker/containers", "")
	if !strings.Contains(list.Body.String(), "ctr-a") || !strings.Contains(list.Body.String(), "ctr-b") {
		t.Fatalf("unauthenticated listing = %s", list.Body.String())
	}
}

// TestStaleSessionLabelFallsBackToTaskOwnership covers a container whose
// session row is gone (rolled back or removed) while its task and container
// live on: the owner must still see and control it.
func TestStaleSessionLabelFallsBackToTaskOwnership(t *testing.T) {
	client, authorizer, titles := twoUserFixture()
	delete(authorizer.sessionOwners, "session-a")
	router := dockerTestRouter(t, client, titles.provider(), authorizer, member("user-a"))

	list := do(router, http.MethodGet, "/api/v1/docker/containers", "")
	if !strings.Contains(list.Body.String(), "ctr-a") {
		t.Fatalf("owner lost their container to a stale session label: %s", list.Body.String())
	}

	stop := do(router, http.MethodPost, "/api/v1/docker/containers/ctr-a/stop", "")
	if stop.Code != http.StatusOK {
		t.Fatalf("stop with stale session label = %d, want 200; body = %s", stop.Code, stop.Body.String())
	}
	remove := do(router, http.MethodDelete, "/api/v1/docker/containers/ctr-a", "")
	if remove.Code != http.StatusOK {
		t.Fatalf("remove with stale session label = %d, want 200; body = %s", remove.Code, remove.Body.String())
	}
}

// TestStaleSessionLabelStillDeniesForeignTask pins that the task label is a
// fallback, not a bypass: a stale session on someone else's task stays hidden.
func TestStaleSessionLabelStillDeniesForeignTask(t *testing.T) {
	client, authorizer, titles := twoUserFixture()
	delete(authorizer.sessionOwners, "session-a")
	router := dockerTestRouter(t, client, titles.provider(), authorizer, member("user-b"))

	list := do(router, http.MethodGet, "/api/v1/docker/containers", "")
	if strings.Contains(list.Body.String(), "ctr-a") {
		t.Fatalf("foreign container leaked via task fallback: %s", list.Body.String())
	}
	stop := do(router, http.MethodPost, "/api/v1/docker/containers/ctr-a/stop", "")
	if stop.Code != http.StatusNotFound || len(client.stopped) != 0 {
		t.Fatalf("foreign stop = %d, stopped = %v", stop.Code, client.stopped)
	}
}

// TestNilAuthorizerDeniesScopedCallers pins the fail-closed half of the wiring
// contract: with no authorizer wired (partial builds), a scoped caller sees
// nothing and can act on nothing.
func TestNilAuthorizerDeniesScopedCallers(t *testing.T) {
	client, _, titles := twoUserFixture()
	router := dockerTestRouter(t, client, titles.provider(), nil, member("user-a"))

	list := do(router, http.MethodGet, "/api/v1/docker/containers", "")
	if list.Code != http.StatusOK || strings.Contains(list.Body.String(), "ctr-") {
		t.Fatalf("nil-authorizer listing = %d %s, want an empty listing", list.Code, list.Body.String())
	}
	stop := do(router, http.MethodPost, "/api/v1/docker/containers/ctr-a/stop", "")
	if stop.Code != http.StatusNotFound {
		t.Fatalf("nil-authorizer stop = %d, want 404", stop.Code)
	}
	remove := do(router, http.MethodDelete, "/api/v1/docker/containers/ctr-a", "")
	if remove.Code != http.StatusNotFound {
		t.Fatalf("nil-authorizer remove = %d, want 404", remove.Code)
	}
	if len(client.stopped) != 0 || len(client.removed) != 0 {
		t.Fatalf("nil-authorizer reached Docker: stopped=%v removed=%v", client.stopped, client.removed)
	}
}

// TestBuildWithoutAnyIdentityIsRejected documents the deliberate asymmetry
// between /build and the container routes. Every HTTP request goes through the
// global identity middleware, so an identity-free request is unreachable in
// production; where the container routes stay unscoped for internal callers,
// the host-level build fails closed with RequireAdmin's 401.
func TestBuildWithoutAnyIdentityIsRejected(t *testing.T) {
	client, authorizer, titles := twoUserFixture()
	router := dockerTestRouter(t, client, titles.provider(), authorizer, nil)

	build := do(router, http.MethodPost, "/api/v1/docker/build", `{"dockerfile":"FROM scratch","tag":"x:1"}`)
	if build.Code != http.StatusUnauthorized {
		t.Fatalf("identity-free build = %d, want 401; body = %s", build.Code, build.Body.String())
	}
	if len(client.builtTags) != 0 {
		t.Fatalf("identity-free build reached Docker: %v", client.builtTags)
	}
}

// TestUnownedContainerIsDeniedOnStopAndRemove is the other half of the task
// fallback: a container that resolves through neither label must stay denied,
// so the fallback cannot be widened into a fail-open path.
func TestUnownedContainerIsDeniedOnStopAndRemove(t *testing.T) {
	client, authorizer, titles := twoUserFixture()
	client.containers = append(client.containers, ContainerInfo{
		ID: "ctr-unlabeled", Name: "stray", State: "running",
	})
	// A container carrying Kandev's marker label but neither ownership label
	// must be denied too, not just a wholly unlabeled one.
	client.containers = append(client.containers, ContainerInfo{
		ID: "ctr-marker-only", Name: "marked", State: "running",
		Labels: map[string]string{"kandev.managed": "true"},
	})
	router := dockerTestRouter(t, client, titles.provider(), authorizer, member("user-a"))

	for _, containerID := range []string{"ctr-unlabeled", "ctr-marker-only"} {
		stop := do(router, http.MethodPost, "/api/v1/docker/containers/"+containerID+"/stop", "")
		if stop.Code != http.StatusNotFound {
			t.Fatalf("%s stop = %d, want 404; body = %s", containerID, stop.Code, stop.Body.String())
		}
		remove := do(router, http.MethodDelete, "/api/v1/docker/containers/"+containerID, "")
		if remove.Code != http.StatusNotFound {
			t.Fatalf("%s remove = %d, want 404; body = %s", containerID, remove.Code, remove.Body.String())
		}
	}
	if len(client.stopped) != 0 || len(client.removed) != 0 {
		t.Fatalf("unowned container reached Docker: stopped=%v removed=%v", client.stopped, client.removed)
	}

	list := do(router, http.MethodGet, "/api/v1/docker/containers", "")
	if strings.Contains(list.Body.String(), "ctr-unlabeled") || strings.Contains(list.Body.String(), "ctr-marker-only") {
		t.Fatalf("unowned container leaked into listing: %s", list.Body.String())
	}
}
