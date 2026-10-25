package api

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"syscall"
	"testing"

	"github.com/kandev/kandev/internal/agentctl/server/config"
	"github.com/kandev/kandev/internal/agentctl/server/process"
	"github.com/kandev/kandev/internal/agentctl/types"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/common/logger"
	"go.uber.org/zap"
)

type apiEntryMutationFixture struct {
	server  *Server
	root    string
	tracker *process.WorkspaceTracker
	sub     types.WorkspaceStreamSubscriber
	target  string
	dirLink bool
}

func apiEntryMutationLink(t *testing.T, value, path string) {
	t.Helper()
	if err := os.Symlink(value, path); err != nil {
		if runtime.GOOS == "windows" && errors.Is(err, syscall.Errno(1314)) {
			t.Skipf("symlink fixture requires native privilege: %v", err)
		}
		t.Fatalf("create link %q -> %q: %v", path, value, err)
	}
}

func newAPIEntryMutationFixture(t *testing.T, directory bool) *apiEntryMutationFixture {
	t.Helper()
	root := t.TempDir()
	target := "target.txt"
	if directory {
		target = "target-directory"
	}
	for _, scope := range []string{"", "alpha", "beta"} {
		bytesPath := filepath.Join(scope, target)
		if directory {
			bytesPath = filepath.Join(bytesPath, "nested", "precious.txt")
		}
		if err := os.MkdirAll(filepath.Dir(filepath.Join(root, bytesPath)), 0o755); err != nil {
			t.Fatal(err)
		}
		writeFileAPI(t, root, bytesPath, "target owner="+scope+"\n")
		writeFileAPI(t, root, filepath.Join(scope, "neighbor.txt"), "neighbor owner="+scope+"\n")
		apiEntryMutationLink(t, target, filepath.Join(root, scope, "alias"))
		apiEntryMutationLink(t, target, filepath.Join(root, scope, "unselected-alias"))
	}
	log, err := logger.NewFromZap(zap.NewNop())
	if err != nil {
		t.Fatal(err)
	}
	cfg := &config.InstanceConfig{WorkDir: root}
	manager := process.NewManager(cfg, log)
	t.Cleanup(func() {
		if err := manager.StopForTeardown(context.Background()); err != nil {
			t.Errorf("stop owned file mutation manager: %v", err)
		}
	})
	tracker := manager.GetWorkspaceTracker()
	if tracker == nil {
		t.Fatal("real manager has no workspace tracker")
	}
	sub := tracker.SubscribeWorkspaceStream()
	t.Cleanup(func() { tracker.UnsubscribeWorkspaceStream(sub) })
	return &apiEntryMutationFixture{
		server: NewServer(cfg, manager, nil, nil, log), root: root,
		tracker: tracker, sub: sub, target: target, dirLink: directory,
	}
}

func apiEntryMutationAssertLink(t *testing.T, path, value string) {
	t.Helper()
	info, err := os.Lstat(path)
	if err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Errorf("HTTP mutation lost symbolic entry %q: %v, %v", path, info, err)
		return
	}
	actual, err := os.Readlink(path)
	if err != nil || actual != value {
		t.Errorf("HTTP mutation changed stored link %q: %q, %v; expected %q", path, actual, err, value)
	}
}

func apiEntryMutationAssertAbsent(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Lstat(path); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("selected old entry still present at %q: %v", path, err)
	}
}

func apiEntryMutationAssertEvents(t *testing.T, sub types.WorkspaceStreamSubscriber, operation string, paths ...string) {
	t.Helper()
	for _, path := range paths {
		select {
		case envelope := <-sub:
			change := envelope.FileChange
			if change == nil || change.Path != path || change.Operation != operation || change.RepositoryName != "" {
				t.Errorf("registered HTTP event = %+v; expected %s %q with root repository identity", change, operation, path)
			}
		default:
			t.Errorf("HTTP response returned before %s event for %q", operation, path)
		}
	}
	select {
	case envelope := <-sub:
		t.Errorf("unexpected HTTP mutation event: %+v", envelope)
	default:
	}
}

func (f *apiEntryMutationFixture) assertPreserved(t *testing.T, selected, dest string, sourceAbsent bool) {
	t.Helper()
	for _, scope := range []string{"", "alpha", "beta"} {
		bytesPath := filepath.Join(scope, f.target)
		if f.dirLink {
			bytesPath = filepath.Join(bytesPath, "nested", "precious.txt")
		}
		assertFileUpdateSaveBytes(t, f.root, bytesPath, "target owner="+scope+"\n")
		info, err := os.Lstat(filepath.Join(f.root, scope, f.target))
		if err != nil || info.IsDir() != f.dirLink || info.Mode()&os.ModeSymlink != 0 {
			t.Errorf("target entry changed in scope %q: %v, %v", scope, info, err)
		}
		assertFileUpdateSaveBytes(t, f.root, filepath.Join(scope, "neighbor.txt"), "neighbor owner="+scope+"\n")
		apiEntryMutationAssertLink(t, filepath.Join(f.root, scope, "unselected-alias"), f.target)
		alias := filepath.Join(scope, "alias")
		if alias == selected && sourceAbsent {
			apiEntryMutationAssertAbsent(t, filepath.Join(f.root, alias))
		} else {
			apiEntryMutationAssertLink(t, filepath.Join(f.root, alias), f.target)
		}
	}
	if dest != "" {
		apiEntryMutationAssertLink(t, filepath.Join(f.root, dest), f.target)
	}
}

func (f *apiEntryMutationFixture) assertTreeAlias(t *testing.T, scope string) {
	t.Helper()
	f.assertTreeEntry(t, scope, "alias")
}

func (f *apiEntryMutationFixture) assertTreeEntry(t *testing.T, scope, name string) {
	t.Helper()
	query := url.Values{"path": {scope}, "depth": {"1"}}
	rec := workspaceRequest(t, f.server, http.MethodGet, "/api/v1/workspace/tree?"+query.Encode(), nil)
	response := decodeWorkspaceBody[streams.FileTreeResponse](t, rec)
	if rec.Code != http.StatusOK || response.Root == nil || response.Error != "" {
		t.Fatalf("registered inventory failed: status=%d body=%+v", rec.Code, response)
	}
	alias := findTreeChild(response.Root, name)
	if alias == nil || alias.Path != filepath.Join(scope, name) || !alias.IsSymlink || alias.IsDir != f.dirLink {
		t.Fatalf("registered inventory selected alias = %+v", alias)
	}
}

type apiEntryMutationCase struct {
	name      string
	repo      string
	prefix    string
	directory bool
}

// @covers AC-WORKSPACES-FILE-ENTRY-MUTATIONS-001.1
// @covers AC-WORKSPACES-FILE-ENTRY-MUTATIONS-001.2
// @covers AC-WORKSPACES-FILE-ENTRY-MUTATIONS-001.3
// @covers AC-WORKSPACES-FILE-ENTRY-MUTATIONS-001.5
// @covers AC-WORKSPACES-FILE-ENTRY-MUTATIONS-001.6
func TestRegisteredWorkspaceFileEntryMutations(t *testing.T) {
	for _, route := range []apiEntryMutationCase{
		{name: "root"},
		{name: "selected-repository", repo: "alpha"},
		{name: "aggregate-path", prefix: "alpha"},
	} {
		for _, directory := range []bool{false, true} {
			route.directory = directory
			kind := "file"
			if directory {
				kind = "directory"
			}
			t.Run(route.name+"/"+kind, func(t *testing.T) { runAPIEntryMutationOperations(t, route) })
		}
	}
	t.Run("root-alias-rejection", runAPIEntryMutationRootRejection)
	t.Run("external-parent-rejection", runAPIEntryMutationExternalParent)
}

// @covers AC-WORKSPACES-FILE-ENTRY-MUTATIONS-001.2
// @covers AC-WORKSPACES-FILE-ENTRY-MUTATIONS-001.6
func TestRegisteredWorkspaceAbsoluteLeafMutations(t *testing.T) {
	for _, route := range []apiEntryMutationCase{
		{name: "selected-repository", repo: "alpha"},
		{name: "aggregate-path", prefix: "alpha"},
	} {
		for _, directory := range []bool{false, true} {
			route.directory = directory
			kind := "file"
			if directory {
				kind = "directory"
			}
			for _, operation := range []string{"rename", "move"} {
				t.Run(route.name+"/"+kind+"/"+operation, func(t *testing.T) {
					runAPIAbsoluteLeafMutation(t, route, operation)
				})
			}
		}
	}
}

func runAPIAbsoluteLeafMutation(t *testing.T, route apiEntryMutationCase, operation string) {
	t.Helper()
	fixture := newAPIEntryMutationFixture(t, route.directory)
	scope := route.repo
	if scope == "" {
		scope = route.prefix
	}
	value := filepath.Join(fixture.root, scope, fixture.target)
	selected := filepath.Join(scope, "absolute-alias")
	apiEntryMutationLink(t, value, filepath.Join(fixture.root, selected))
	fixture.assertTreeEntry(t, scope, "absolute-alias")
	targetBefore, err := os.Lstat(value)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(route.prefix, "absolute-alias")
	dest := filepath.Join(route.prefix, "renamed-absolute")
	if operation == "move" {
		dest = filepath.Join(route.prefix, "created", "deep", "moved-absolute")
	}
	rec := workspaceRequest(t, fixture.server, http.MethodPost, "/api/v1/workspace/file/rename",
		streams.FileRenameRequest{Repo: route.repo, OldPath: path, NewPath: dest})
	response := decodeWorkspaceBody[streams.FileRenameResponse](t, rec)
	if rec.Code != http.StatusOK || !response.Success || response.Error != "" || response.OldPath != path || response.NewPath != dest {
		t.Errorf("absolute contained leaf %s response: status=%d body=%+v", operation, rec.Code, response)
	}
	fixture.assertPreserved(t, "", "", false)
	targetAfter, err := os.Lstat(value)
	if err != nil || !os.SameFile(targetBefore, targetAfter) {
		t.Errorf("absolute leaf mutation changed target identity: %v", err)
	}
	newEntry := filepath.Join(route.repo, dest)
	apiEntryMutationAssertAbsent(t, filepath.Join(fixture.root, selected))
	apiEntryMutationAssertLink(t, filepath.Join(fixture.root, newEntry), value)
	apiEntryMutationAssertEvents(t, fixture.sub, types.FileOpRename, selected, newEntry)
}

func runAPIEntryMutationOperations(t *testing.T, route apiEntryMutationCase) {
	t.Helper()
	for _, operation := range []string{"delete", "rename", "move", "same-entry", "occupied", "dangling-destination"} {
		t.Run(operation, func(t *testing.T) { runAPIEntryMutationRequest(t, route, operation) })
	}
}

func runAPIEntryMutationRequest(t *testing.T, route apiEntryMutationCase, operation string) {
	t.Helper()
	fixture := newAPIEntryMutationFixture(t, route.directory)
	scope := route.repo
	if scope == "" {
		scope = route.prefix
	}
	fixture.assertTreeAlias(t, scope)
	path := filepath.Join(route.prefix, "alias")
	selected := filepath.Join(scope, "alias")
	if operation == "delete" {
		rec := workspaceRequest(t, fixture.server, http.MethodDelete, "/api/v1/workspace/file?"+url.Values{"path": {path}, "repo": {route.repo}}.Encode(), nil)
		response := decodeWorkspaceBody[streams.FileDeleteResponse](t, rec)
		if rec.Code != http.StatusOK || !response.Success || response.Path != path || response.Error != "" {
			t.Errorf("delete response: status=%d body=%+v", rec.Code, response)
		}
		fixture.assertPreserved(t, selected, "", true)
		apiEntryMutationAssertEvents(t, fixture.sub, types.FileOpRemove, selected)
		return
	}
	runAPIEntryMutationRename(t, fixture, route, operation, scope, path)
}

func runAPIEntryMutationRename(t *testing.T, fixture *apiEntryMutationFixture, route apiEntryMutationCase, operation, scope, path string) {
	t.Helper()
	dest, status, absent := filepath.Join(route.prefix, "renamed"), http.StatusOK, true
	switch operation {
	case "move":
		dest = filepath.Join(route.prefix, "created", "deep", "moved")
	case "same-entry":
		dest, absent = path, false
	case "occupied":
		dest, status, absent = filepath.Join(route.prefix, "occupied"), http.StatusBadRequest, false
		apiEntryMutationLink(t, fixture.target, filepath.Join(fixture.root, scope, "occupied"))
	case "dangling-destination":
		dest, status, absent = filepath.Join(route.prefix, "occupied"), http.StatusBadRequest, false
		apiEntryMutationLink(t, "missing.txt", filepath.Join(fixture.root, scope, "occupied"))
	}
	rec := workspaceRequest(t, fixture.server, http.MethodPost, "/api/v1/workspace/file/rename", streams.FileRenameRequest{Repo: route.repo, OldPath: path, NewPath: dest})
	response := decodeWorkspaceBody[streams.FileRenameResponse](t, rec)
	if rec.Code != status || response.Success != (status == http.StatusOK) || response.OldPath != path || response.NewPath != dest || (response.Error == "") != (status == http.StatusOK) {
		t.Errorf("rename response: status=%d body=%+v; expected status=%d", rec.Code, response, status)
	}
	newEntry := ""
	paths := []string{}
	if absent {
		newEntry = filepath.Join(route.repo, dest)
		paths = append(paths, filepath.Join(scope, "alias"), newEntry)
	}
	fixture.assertPreserved(t, filepath.Join(scope, "alias"), newEntry, absent)
	if operation == "occupied" {
		apiEntryMutationAssertLink(t, filepath.Join(fixture.root, scope, "occupied"), fixture.target)
	}
	if operation == "dangling-destination" {
		apiEntryMutationAssertLink(t, filepath.Join(fixture.root, scope, "occupied"), "missing.txt")
		apiEntryMutationAssertAbsent(t, filepath.Join(fixture.root, scope, "missing.txt"))
	}
	apiEntryMutationAssertEvents(t, fixture.sub, types.FileOpRename, paths...)
}

func assertAPIEntryMutationRejected(t *testing.T, path, newPath string, rec *httptest.ResponseRecorder) {
	t.Helper()
	if rec.Code != http.StatusBadRequest {
		t.Errorf("root/escape mutation status = %d; body=%s", rec.Code, rec.Body.String())
	}
	if newPath != "" {
		body := decodeWorkspaceBody[streams.FileRenameResponse](t, rec)
		if body.Success || body.Error == "" || body.OldPath != path || body.NewPath != newPath {
			t.Errorf("rejected rename body = %+v", body)
		}
	} else {
		body := decodeWorkspaceBody[streams.FileDeleteResponse](t, rec)
		if body.Success || body.Error == "" || body.Path != path {
			t.Errorf("rejected delete body = %+v", body)
		}
	}
}

func runAPIEntryMutationRootRejection(t *testing.T) {
	t.Helper()
	for _, operation := range []string{"delete", "rename", "same-entry"} {
		t.Run(operation, func(t *testing.T) {
			fixture := newAPIEntryMutationFixture(t, false)
			apiEntryMutationLink(t, ".", filepath.Join(fixture.root, "root-alias"))
			var rec *httptest.ResponseRecorder
			dest := ""
			if operation == "delete" {
				rec = workspaceRequest(t, fixture.server, http.MethodDelete, "/api/v1/workspace/file?path=root-alias", nil)
			} else {
				dest = "moved-root"
				if operation == "same-entry" {
					dest = "root-alias"
				}
				rec = workspaceRequest(t, fixture.server, http.MethodPost, "/api/v1/workspace/file/rename", streams.FileRenameRequest{OldPath: "root-alias", NewPath: dest})
			}
			assertAPIEntryMutationRejected(t, "root-alias", dest, rec)
			fixture.assertPreserved(t, "", "", false)
			apiEntryMutationAssertLink(t, filepath.Join(fixture.root, "root-alias"), ".")
			apiEntryMutationAssertAbsent(t, filepath.Join(fixture.root, "moved-root"))
			apiEntryMutationAssertEvents(t, fixture.sub, types.FileOpRename)
		})
	}
}

func runAPIEntryMutationExternalParent(t *testing.T) {
	t.Helper()
	for _, operation := range []string{"delete", "rename"} {
		t.Run(operation, func(t *testing.T) {
			fixture := newAPIEntryMutationFixture(t, false)
			external := t.TempDir()
			writeFileAPI(t, external, "precious.txt", "outside bytes\n")
			apiEntryMutationLink(t, external, filepath.Join(fixture.root, "outside"))
			path := filepath.Join("outside", "precious.txt")
			var rec *httptest.ResponseRecorder
			dest := ""
			if operation == "delete" {
				rec = workspaceRequest(t, fixture.server, http.MethodDelete, "/api/v1/workspace/file?"+url.Values{"path": {path}}.Encode(), nil)
			} else {
				dest = "moved"
				rec = workspaceRequest(t, fixture.server, http.MethodPost, "/api/v1/workspace/file/rename", streams.FileRenameRequest{OldPath: path, NewPath: "moved"})
			}
			assertAPIEntryMutationRejected(t, path, dest, rec)
			assertFileUpdateSaveBytes(t, external, "precious.txt", "outside bytes\n")
			fixture.assertPreserved(t, "", "", false)
			apiEntryMutationAssertLink(t, filepath.Join(fixture.root, "outside"), external)
			apiEntryMutationAssertAbsent(t, filepath.Join(fixture.root, "moved"))
			apiEntryMutationAssertEvents(t, fixture.sub, types.FileOpRename)
		})
	}
}
