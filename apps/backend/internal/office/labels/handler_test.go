package labels

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/require"

	settingsstore "github.com/kandev/kandev/internal/agent/settings/store"
	"github.com/kandev/kandev/internal/db"
	officesqlite "github.com/kandev/kandev/internal/office/repository/sqlite"
	tasksqlite "github.com/kandev/kandev/internal/task/repository/sqlite"
)

const taskLabelsPath = "/api/v1/office/workspaces/labels-home/tasks/selected/labels"

type labelHTTPFixture struct {
	t      *testing.T
	router *gin.Engine
	repo   *officesqlite.Repository
}

func newLabelHTTPFixture(t *testing.T) *labelHTTPFixture {
	t.Helper()
	conn, err := db.OpenSQLite(filepath.Join(t.TempDir(), "labels.db"))
	require.NoError(t, err)
	database := sqlx.NewDb(conn, "sqlite3")
	t.Cleanup(func() { require.NoError(t, database.Close()) })
	var foreignKeys int
	require.NoError(t, database.Get(&foreignKeys, "PRAGMA foreign_keys"))
	require.Equal(t, 1, foreignKeys)
	_, cleanup, err := settingsstore.Provide(database, database, nil)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, cleanup()) })
	_, err = tasksqlite.NewWithDB(database, database, nil)
	require.NoError(t, err)
	repo, err := officesqlite.NewWithDB(database, database, nil)
	require.NoError(t, err)
	now := time.Now().UTC()
	for _, ws := range []string{"labels-home", "labels-other"} {
		_, err = database.Exec(`INSERT INTO workspaces (id, name, created_at, updated_at) VALUES (?, ?, ?, ?)`,
			ws, ws, now, now)
		require.NoError(t, err)
	}
	for _, task := range []struct{ id, workspace string }{
		{"selected", "labels-home"}, {"sibling", "labels-home"}, {"foreign", "labels-other"},
	} {
		_, err = database.Exec(`INSERT INTO tasks (id, workspace_id, title, created_at, updated_at) VALUES (?, ?, ?, ?, ?)`,
			task.id, task.workspace, task.id, now, now)
		require.NoError(t, err)
	}
	gin.SetMode(gin.TestMode)
	router := gin.New()
	RegisterRoutes(router.Group("/api/v1/office"), NewLabelService(repo))
	return &labelHTTPFixture{t: t, router: router, repo: repo}
}

func (f *labelHTTPFixture) request(method, path, body string) *httptest.ResponseRecorder {
	f.t.Helper()
	response := httptest.NewRecorder()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	f.router.ServeHTTP(response, req)
	return response
}

func (f *labelHTTPFixture) add(path, name string) Label {
	f.t.Helper()
	body, err := json.Marshal(map[string]string{"name": name})
	require.NoError(f.t, err)
	response := f.request(http.MethodPost, path, string(body))
	require.Equal(f.t, http.StatusCreated, response.Code, response.Body.String())
	var result struct {
		Label Label `json:"label"`
	}
	require.NoError(f.t, json.Unmarshal(response.Body.Bytes(), &result))
	require.Equal(f.t, name, result.Label.Name)
	return result.Label
}

func (f *labelHTTPFixture) assertNames(task string, expected ...string) {
	f.t.Helper()
	rows, err := f.repo.ListLabelsForTask(context.Background(), task)
	require.NoError(f.t, err)
	names := make([]string, 0, len(rows))
	for _, label := range rows {
		names = append(names, label.Name)
	}
	require.ElementsMatch(f.t, expected, names)
}

// @covers AC-TASKS-LABELS-002.1, AC-TASKS-LABELS-002.2
func TestTaskLabelRemovalNameIdentity(t *testing.T) {
	for _, name := range []string{
		"area/backend", "a/b/c", "area%2Fbackend", "a+b", "a b", "100%", "needs review", " edge ", " ",
	} {
		t.Run(strings.ReplaceAll(name, "%", "percent"), func(t *testing.T) {
			f := newLabelHTTPFixture(t)
			f.add(taskLabelsPath, name)
			neighbor := "literal:" + name
			f.add(taskLabelsPath, neighbor)
			response := f.request(http.MethodDelete, taskLabelsPath+"?"+url.Values{"name": {name}}.Encode(), "")
			require.Equal(t, http.StatusOK, response.Code, response.Body.String())
			require.JSONEq(t, `{"ok":true}`, response.Body.String())
			f.assertNames("selected", neighbor)
		})
	}
}

// @covers AC-TASKS-LABELS-002.3
func TestTaskLabelRemovalLegacyCompatibility(t *testing.T) {
	f := newLabelHTTPFixture(t)
	for _, name := range []string{"urgent", "needs review", "a+b", "100%", "area%2Fbackend"} {
		f.add(taskLabelsPath, name)
		response := f.request(http.MethodDelete, taskLabelsPath+"/"+url.PathEscape(name), "")
		require.Equal(t, http.StatusOK, response.Code, response.Body.String())
		require.JSONEq(t, `{"ok":true}`, response.Body.String())
		f.assertNames("selected")
	}
	f.add(taskLabelsPath, "area/backend")
	response := f.request(http.MethodDelete, taskLabelsPath+"/area%2Fbackend", "")
	require.Equal(t, http.StatusNotFound, response.Code)
	f.assertNames("selected", "area/backend")
}

// @covers AC-TASKS-LABELS-002.4
func TestTaskLabelRemovalInvalidQuery(t *testing.T) {
	f := newLabelHTTPFixture(t)
	f.add(taskLabelsPath, "urgent")
	for _, query := range []string{"", "?name", "?name=", "?name=urgent&name=other", "?name=%", "?name=urgent&extra=%"} {
		t.Run(query, func(t *testing.T) {
			response := f.request(http.MethodDelete, taskLabelsPath+query, "")
			require.Equal(t, http.StatusBadRequest, response.Code, response.Body.String())
			var result map[string]string
			require.NoError(t, json.Unmarshal(response.Body.Bytes(), &result))
			require.NotEmpty(t, result["error"])
			f.assertNames("selected", "urgent")
		})
	}
}

// @covers AC-TASKS-LABELS-002.1, AC-TASKS-LABELS-002.2, AC-TASKS-LABELS-002.4
func TestTaskLabelRemovalIsolationAndIdempotency(t *testing.T) {
	f := newLabelHTTPFixture(t)
	label := f.add(taskLabelsPath, "area/backend")
	f.add(taskLabelsPath, "area%2Fbackend")
	f.add(taskLabelsPath, "urgent")
	f.add("/api/v1/office/workspaces/labels-home/tasks/sibling/labels", label.Name)
	foreign := f.add("/api/v1/office/workspaces/labels-other/tasks/foreign/labels", label.Name)
	require.NotEqual(t, label.ID, foreign.ID)
	before, err := f.repo.ListLabelsByWorkspace(context.Background(), "labels-home")
	require.NoError(t, err)
	for _, name := range []string{label.Name, label.Name, "missing", "area%2Fbackend"} {
		response := f.request(http.MethodDelete, taskLabelsPath+"?name="+url.QueryEscape(name)+"&unused=ok", "")
		require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	}
	f.assertNames("selected", "urgent")
	f.assertNames("sibling", label.Name)
	f.assertNames("foreign", label.Name)
	after, err := f.repo.ListLabelsByWorkspace(context.Background(), "labels-home")
	require.NoError(t, err)
	require.Equal(t, before, after)
	storedForeign, err := f.repo.GetLabelByName(context.Background(), "labels-other", label.Name)
	require.NoError(t, err)
	require.Equal(t, foreign.ID, storedForeign.ID)
}
