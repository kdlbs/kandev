package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agentctl/server/config"
	"github.com/kandev/kandev/internal/agentctl/server/process"
	"github.com/kandev/kandev/internal/agentctl/types"
	storageworkspaces "github.com/kandev/kandev/internal/system/storage/workspaces"
)

// @covers AC-UI-TASK-WORKSPACE-CONTENT-SEARCH-001.8
// @covers AC-UI-TASK-WORKSPACE-CONTENT-SEARCH-001.9
func TestRegisteredWorkspaceSearchPreservesExactFilenames(t *testing.T) {
	for _, quotePath := range []string{"default", "true", "false"} {
		t.Run(quotePath, func(t *testing.T) {
			isolateAPIExactGit(t)
			repo := t.TempDir()
			initAPIExactRepo(t, repo)
			if quotePath != "default" {
				apiExactGit(t, repo, "config", "core.quotePath", quotePath)
			}
			files := populateAPIExactPaths(t, repo, "single")
			assertAPIExactReadOnly(t, repo)
			server := startAPIExactServer(t, repo, 1)
			assertAPIExactSearch(t, server, map[string]map[string]string{"": files})
		})
	}
}

// @covers AC-UI-TASK-WORKSPACE-CONTENT-SEARCH-001.6
// @covers AC-UI-TASK-WORKSPACE-CONTENT-SEARCH-001.7
// @covers AC-UI-TASK-WORKSPACE-CONTENT-SEARCH-001.9
func TestRegisteredWorkspaceSearchPreservesRepositoryIdentity(t *testing.T) {
	isolateAPIExactGit(t)
	root := t.TempDir()
	repositories := make(map[string]map[string]string)
	for _, name := range []string{"alpha", "beta"} {
		repo := filepath.Join(root, name)
		initAPIExactRepo(t, repo)
		repositories[name] = populateAPIExactPaths(t, repo, name)
		assertAPIExactReadOnly(t, repo)
	}
	server := startAPIExactServer(t, root, 2)
	assertAPIExactSearch(t, server, repositories)
}

func populateAPIExactPaths(t *testing.T, repo, sentinel string) map[string]string {
	t.Helper()
	files := make(map[string]string)
	for _, kind := range []string{"tracked", "untracked"} {
		for _, name := range []string{
			"ordinary-exact-" + kind + ".txt", "日本語😀exact-" + kind + ".txt",
			" leading-exact-" + kind + ".txt", "trailing-exact-" + kind + ".txt ",
			"quoted\"exact-" + kind + ".txt", "tab\texact-" + kind + ".txt",
			"line\nexact-" + kind + ".txt",
		} {
			t.Run(kind+"/"+name, func(t *testing.T) {
				if runtime.GOOS == "windows" && (strings.ContainsAny(name, "\"\t\n") || strings.HasSuffix(name, " ")) {
					t.Skip("Windows does not support this literal filename")
				}
				files[name] = "zero\n😀 transportneedle " + sentinel + " " + kind + "\n"
				writeAPIExactFile(t, repo, name, files[name])
				if kind == "tracked" {
					apiExactGit(t, repo, "add", "--", name)
				}
			})
		}
	}
	for _, name := range []string{"decoy-exact.txt", "nested/" + storageworkspaces.OwnershipMarkerFilename} {
		files[name] = "zero\n😀 transportneedle " + sentinel + " decoyonly\n"
		writeAPIExactFile(t, repo, name, files[name])
	}
	head := strings.TrimSpace(string(apiExactGit(t, repo, "rev-parse", "HEAD")))
	apiExactGit(t, repo, "update-index", "--add", "--cacheinfo", "160000,"+head+",gitlink-exact")
	if runtime.GOOS != "windows" {
		for _, mode := range []string{"160000", "100644"} {
			name := mode + " " + head + " 0\tdecoy-exact.txt"
			files[name] = "zero\n😀 transportneedle " + sentinel + " selected" + mode + "payload\n"
			writeAPIExactFile(t, repo, name, files[name])
		}
	}
	for _, name := range []string{"ignored-exact.txt", storageworkspaces.OwnershipMarkerFilename} {
		writeAPIExactFile(t, repo, name, "zero\n😀 transportneedle excluded\n")
	}
	writeAPIExactFile(t, repo, ".gitignore", "ignored-exact.txt\n")
	writeAPIExactFile(t, repo, "ready-inventory.txt", "ready\n")
	return files
}

func assertAPIExactSearch(t *testing.T, server *Server, repositories map[string]map[string]string) {
	t.Helper()
	var filenames types.FileSearchResponse
	getAPIExactJSON(t, server, "/api/v1/workspace/search", url.Values{"q": {"exact"}, "limit": {"50"}}, &filenames)
	wantFiles := make([]string, 0)
	for repository, files := range repositories {
		for path := range files {
			if strings.Contains(path, "exact") {
				wantFiles = append(wantFiles, apiExactPrefixedPath(repository, path))
			}
		}
	}
	gotFiles := append([]string(nil), filenames.Files...)
	sort.Strings(gotFiles)
	sort.Strings(wantFiles)
	if !reflect.DeepEqual(gotFiles, wantFiles) {
		t.Errorf("registered filename paths = %q, want %q", gotFiles, wantFiles)
	}
	if len(filenames.Results) != len(filenames.Files) {
		t.Fatalf("legacy/structured filename lengths = %d/%d", len(filenames.Files), len(filenames.Results))
	}
	for index, match := range filenames.Results {
		if match.Path != filenames.Files[index] {
			t.Errorf("legacy/structured path mismatch: %+v / %q", match, filenames.Files[index])
		}
		path := match.Path
		if match.RepositoryName != "" {
			path = strings.TrimPrefix(path, match.RepositoryName+"/")
		}
		assertAPIExactSelected(t, server, match.RepositoryName, path, repositories[match.RepositoryName][path])
	}
	var contents types.WorkspaceContentSearchResponse
	getAPIExactJSON(t, server, "/api/v1/workspace/content-search",
		url.Values{"q": {"transportneedle"}, "limit_per_repo": {"50"}}, &contents)
	got := make(map[string]string)
	for _, match := range contents.Results {
		got[match.RepositoryName+"\x00"+match.Path] = match.Preview
		if match.Line != 2 || match.Column != 4 ||
			!reflect.DeepEqual(match.MatchRanges, []types.WorkspaceContentMatchRange{{Start: 3, End: 18}}) {
			t.Errorf("registered content coordinates/ranges = %+v", match)
		}
		assertAPIExactSelected(t, server, match.RepositoryName, match.Path, repositories[match.RepositoryName][match.Path])
	}
	want := make(map[string]string)
	for repository, files := range repositories {
		for path, content := range files {
			want[repository+"\x00"+path] = strings.Split(content, "\n")[1]
		}
	}
	if !reflect.DeepEqual(got, want) || len(contents.Results) != len(want) {
		t.Errorf("registered content identities/previews = %q, want %q", got, want)
	}
}

func assertAPIExactSelected(t *testing.T, server *Server, repo, path, want string) {
	t.Helper()
	if want == "" {
		t.Errorf("search returned unknown repository/path: %q / %q", repo, path)
		return
	}
	var selected types.FileContentResponse
	getAPIExactJSON(t, server, "/api/v1/workspace/file/content", url.Values{"repo": {repo}, "path": {path}}, &selected)
	if selected.Path != path || selected.Content != want || selected.Size != int64(len(want)) ||
		selected.IsBinary || selected.Error != "" || selected.ResolvedPath != "" {
		t.Errorf("selected %q / %q = %+v, want exact content %q", repo, path, selected, want)
	}
}

func startAPIExactServer(t *testing.T, root string, repositories int) *Server {
	t.Helper()
	log := newTestLogger()
	cfg := &config.InstanceConfig{WorkDir: root, AgentEnv: os.Environ()}
	manager := process.NewManager(cfg, log)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := manager.Stop(ctx); err != nil {
			t.Errorf("join manager: %v", err)
		}
	})
	server := NewServer(cfg, manager, nil, nil, log)
	manager.StartAllWorkspaceTrackers(context.Background())
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		var response types.FileSearchResponse
		getAPIExactJSON(t, server, "/api/v1/workspace/search", url.Values{"q": {"ready-inventory"}}, &response)
		if len(response.Files) == repositories {
			return server
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("registered filename search did not observe initial inventory")
	return nil
}

func getAPIExactJSON(t *testing.T, server *Server, route string, query url.Values, response any) {
	t.Helper()
	request := httptest.NewRequest(http.MethodGet, route+"?"+query.Encode(), nil)
	recorder := httptest.NewRecorder()
	server.router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("GET %s: %d: %s", route, recorder.Code, recorder.Body.String())
	}
	if err := json.NewDecoder(recorder.Body).Decode(response); err != nil {
		t.Fatal(err)
	}
}

func initAPIExactRepo(t *testing.T, repo string) {
	t.Helper()
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatal(err)
	}
	apiExactGit(t, repo, "init", "-b", "main")
	apiExactGit(t, repo, "config", "user.name", "Transport Test")
	apiExactGit(t, repo, "config", "user.email", "transport@example.com")
	apiExactGit(t, repo, "config", "core.autocrlf", "false")
	apiExactGit(t, repo, "commit", "--allow-empty", "-m", "fixture")
}

func apiExactGit(t *testing.T, repo string, args ...string) []byte {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", repo, "-c", "commit.gpgsign=false"}, args...)...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v: %s", args, err, output)
	}
	return output
}

func isolateAPIExactGit(t *testing.T) {
	t.Helper()
	for _, item := range os.Environ() {
		key, value, _ := strings.Cut(item, "=")
		if strings.HasPrefix(key, "GIT_") {
			if err := os.Unsetenv(key); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = os.Setenv(key, value) })
		}
	}
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
}

func writeAPIExactFile(t *testing.T, repo, path, content string) {
	t.Helper()
	fullPath := filepath.Join(repo, filepath.FromSlash(path))
	if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(fullPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func assertAPIExactReadOnly(t *testing.T, repo string) {
	t.Helper()
	before := snapshotAPIExactBytes(t, repo)
	t.Cleanup(func() {
		if after := snapshotAPIExactBytes(t, repo); !reflect.DeepEqual(after, before) {
			t.Error("registered search/read changed Git/index/config/filesystem bytes")
		}
	})
}

func snapshotAPIExactBytes(t *testing.T, repo string) map[string]string {
	t.Helper()
	snapshot := make(map[string]string)
	err := filepath.WalkDir(repo, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		relative, relErr := filepath.Rel(repo, path)
		if relErr != nil {
			return relErr
		}
		snapshot[filepath.ToSlash(relative)] = string(data)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	snapshot["refs"] = string(bytes.TrimSuffix(apiExactGit(t, repo, "for-each-ref", "--format=%(refname) %(objectname)"), []byte{'\n'}))
	return snapshot
}

func apiExactPrefixedPath(repo, path string) string {
	if repo == "" {
		return path
	}
	return repo + "/" + path
}
