package handlers

import (
	"bytes"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/auth/authn"
)

// agentctlUploadServer answers the two agentctl upload routes and records
// whether either was reached, so a denial can be proven to land first.
func agentctlUploadServer(t *testing.T) (*httptest.Server, *atomic.Bool) {
	t.Helper()
	reached := &atomic.Bool{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/workspace/file/upload":
			reached.Store(true)
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"path":"fixtures/sample.txt","size_bytes":7,"resolution_applied":""}`))
		case "/api/v1/workspace/file/upload-preflight":
			reached.Store(true)
			_, _ = w.Write([]byte(`{"conflicts":[{"path":"fixtures/there.txt","is_dir":false}]}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(server.Close)
	return server, reached
}

// uploadRequestAs builds a multipart upload for sess-b as userID.
func uploadRequestAs(t *testing.T, userID string, fields map[string]string, content string) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	for k, v := range fields {
		require.NoError(t, writer.WriteField(k, v))
	}
	part, err := writer.CreateFormFile("file", "sample.txt")
	require.NoError(t, err)
	_, err = part.Write([]byte(content))
	require.NoError(t, err)
	require.NoError(t, writer.Close())
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/task-sessions/sess-b/workspace/files", &body)
	c.Request.Header.Set("Content-Type", writer.FormDataContentType())
	c.Request = c.Request.WithContext(authn.WithIdentity(
		c.Request.Context(), authn.Identity{UserID: userID, Role: authn.RoleMember}))
	c.Params = gin.Params{{Key: "id", Value: "sess-b"}}
	return c, rec
}
func preflightRequestAs(t *testing.T, userID, body string) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost,
		"/api/v1/task-sessions/sess-b/workspace/files/preflight", strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Request = c.Request.WithContext(authn.WithIdentity(
		c.Request.Context(), authn.Identity{UserID: userID, Role: authn.RoleMember}))
	c.Params = gin.Params{{Key: "id", Value: "sess-b"}}
	return c, rec
}
func uploadFields(content string) map[string]string {
	return map[string]string{
		"dir":           "fixtures",
		"relative_path": "sample.txt",
		"size_bytes":    strconv.Itoa(len(content)),
	}
}

// TestWorkspaceFileUploadDeniesForeignSession proves the scoping guard lands
// before any byte reaches agentctl. The owner request is the witness that the
// denial is scoping and not an unrelated failure.
func TestWorkspaceFileUploadDeniesForeignSession(t *testing.T) {
	server, reached := agentctlUploadServer(t)
	h := newForeignProcessHandlers(t, managerWithSessionExecution(t, server.URL, "sess-b"))
	content := "payload"
	c, rec := uploadRequestAs(t, "user-a", uploadFields(content), content)
	h.httpUploadWorkspaceFile(c)
	require.Equal(t, http.StatusNotFound, rec.Code, "body: %s", rec.Body.String())
	require.False(t, reached.Load(), "the denial must land before agentctl is contacted")
	ownerCtx, ownerRec := uploadRequestAs(t, "user-b", uploadFields(content), content)
	h.httpUploadWorkspaceFile(ownerCtx)
	require.Equal(t, http.StatusCreated, ownerRec.Code, "body: %s", ownerRec.Body.String())
	require.Contains(t, ownerRec.Body.String(), "fixtures/sample.txt")
	require.True(t, reached.Load())
}
func TestWorkspaceFilePreflightDeniesForeignSession(t *testing.T) {
	server, reached := agentctlUploadServer(t)
	h := newForeignProcessHandlers(t, managerWithSessionExecution(t, server.URL, "sess-b"))
	c, rec := preflightRequestAs(t, "user-a", `{"dir":"fixtures","paths":["there.txt"]}`)
	h.httpPreflightWorkspaceUpload(c)
	require.Equal(t, http.StatusNotFound, rec.Code)
	require.False(t, reached.Load(), "the denial must land before agentctl is contacted")
	ownerCtx, ownerRec := preflightRequestAs(t, "user-b", `{"dir":"fixtures","paths":["there.txt"]}`)
	h.httpPreflightWorkspaceUpload(ownerCtx)
	require.Equal(t, http.StatusOK, ownerRec.Code, "body: %s", ownerRec.Body.String())
	require.Contains(t, ownerRec.Body.String(), "fixtures/there.txt")
	require.True(t, reached.Load())
}
func TestWorkspaceFileUploadRequiresRelativePath(t *testing.T) {
	server, reached := agentctlUploadServer(t)
	h := newForeignProcessHandlers(t, managerWithSessionExecution(t, server.URL, "sess-b"))
	fields := uploadFields("payload")
	delete(fields, "relative_path")
	c, rec := uploadRequestAs(t, "user-b", fields, "payload")
	h.httpUploadWorkspaceFile(c)
	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.False(t, reached.Load(), "a malformed request must not reach agentctl")
}
func TestWorkspaceFileUploadRejectsSizeMismatch(t *testing.T) {
	server, reached := agentctlUploadServer(t)
	h := newForeignProcessHandlers(t, managerWithSessionExecution(t, server.URL, "sess-b"))
	fields := uploadFields("payload")
	fields["size_bytes"] = "9999"
	c, rec := uploadRequestAs(t, "user-b", fields, "payload")
	h.httpUploadWorkspaceFile(c)
	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.False(t, reached.Load(), "a size mismatch must not reach agentctl")
}

// TestWorkspaceFileUploadWithoutExecution covers a session that is authorized
// but has no live workspace to write into.
func TestWorkspaceFileUploadWithoutExecution(t *testing.T) {
	log := newTestLogger(t)
	h := newForeignProcessHandlers(t, newLifecycleManager(t, log))
	content := "payload"
	c, rec := uploadRequestAs(t, "user-b", uploadFields(content), content)
	h.httpUploadWorkspaceFile(c)
	require.Equal(t, http.StatusServiceUnavailable, rec.Code, "body: %s", rec.Body.String())
}

// TestRegisterWorkspaceFileRoutes pins the two route paths so a rename is a
// deliberate change rather than a silent 404 for every client.
func TestRegisterWorkspaceFileRoutes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	log := newTestLogger(t)
	RegisterWorkspaceFileRoutes(router, &ProcessHandlers{lifecycleMgr: newLifecycleManager(t, log), logger: log})
	paths := map[string]bool{}
	for _, route := range router.Routes() {
		paths[route.Method+" "+route.Path] = true
	}
	require.True(t, paths["POST /api/v1/task-sessions/:id/workspace/files"], "upload route missing: %v", paths)
	require.True(t, paths["POST /api/v1/task-sessions/:id/workspace/files/preflight"], "preflight route missing: %v", paths)
}
