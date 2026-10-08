package process

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agentctl/types"
	storageworkspaces "github.com/kandev/kandev/internal/system/storage/workspaces"
)

// @covers AC-UI-TASK-WORKSPACE-CONTENT-SEARCH-001.8
// @covers AC-UI-TASK-WORKSPACE-CONTENT-SEARCH-001.9
func TestWorkspaceInventoryPreservesExactFilenames(t *testing.T) {
	for _, quotePath := range []string{"default", "true", "false"} {
		t.Run(quotePath, func(t *testing.T) {
			repo := newExactInventoryRepo(t)
			if quotePath != "default" {
				exactInventoryGit(t, repo, nil, "config", "core.quotePath", quotePath)
			}
			files := populateExactInventory(t, repo)
			before := exactInventorySnapshot(t, repo, files)
			tracker := NewWorkspaceTrackerForRepo(repo, "exact-repo", newTestLogger(t))
			t.Cleanup(tracker.Stop)
			inventory, err := tracker.getFileList(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			if inventory.RepositoryName != "exact-repo" || inventory.Timestamp.IsZero() {
				t.Errorf("inventory metadata = %+v", inventory)
			}
			got := make([]string, 0, len(inventory.Files))
			for _, file := range inventory.Files {
				got = append(got, file.Path)
				if file.IsDir {
					t.Errorf("inventory returned directory: %q", file.Path)
				}
			}
			sort.Strings(got)
			want := exactInventoryEligiblePaths(files)
			if !reflect.DeepEqual(got, want) {
				t.Errorf("inventory paths = %q, want %q", got, want)
			}
			tracker.updateFiles(context.Background())
			for _, path := range want {
				if !containsExactInventoryPath(tracker.SearchFiles(path, 50), path) {
					t.Errorf("cached filename search lost %q", path)
				}
			}
			assertExactInventoryContent(t, tracker, files)
			for _, query := range []string{"ignored.txt", "gitlink-exact"} {
				if matches := tracker.SearchFiles(query, 50); len(matches) != 0 {
					t.Errorf("excluded filename query %q = %q", query, matches)
				}
			}
			if after := exactInventorySnapshot(t, repo, files); !reflect.DeepEqual(after, before) {
				t.Error("inventory/cache/search changed Git or fixture bytes")
			}
		})
	}
	t.Run("empty", func(t *testing.T) {
		repo := newExactInventoryRepo(t)
		tracker := NewWorkspaceTrackerForRepo(repo, "empty", newTestLogger(t))
		t.Cleanup(tracker.Stop)
		inventory, err := tracker.getFileList(context.Background())
		if err != nil || len(inventory.Files) != 0 {
			t.Fatalf("empty inventory = %+v, error %v", inventory, err)
		}
		tracker.updateFiles(context.Background())
		content, err := tracker.SearchContent(context.Background(), "inventoryneedle", 50)
		if err != nil || len(content) != 0 || len(tracker.SearchFiles("ordinary", 50)) != 0 {
			t.Fatalf("empty searches = %+v, error %v", content, err)
		}
	})
}

func populateExactInventory(t *testing.T, repo string) map[string]string {
	t.Helper()
	files := map[string]string{}
	for _, kind := range []string{"tracked", "untracked"} {
		for _, name := range exactInventoryNames(kind) {
			t.Run(kind+"/"+name, func(t *testing.T) {
				if runtime.GOOS == "windows" && (strings.ContainsAny(name, "\"\t\n") || strings.HasSuffix(name, " ")) {
					t.Skip("Windows does not support this literal filename")
				}
				content := "zero\n😀 inventoryneedle " + kind + "\n"
				writeFile(t, repo, name, content)
				files[name] = content
				if kind == "tracked" {
					exactInventoryGit(t, repo, nil, "add", "--", name)
				}
			})
		}
	}
	if err := os.MkdirAll(filepath.Join(repo, "nested"), 0o755); err != nil {
		t.Fatal(err)
	}
	marker := storageworkspaces.OwnershipMarkerFilename
	for _, path := range []string{marker, "nested/" + marker, "ignored.txt", "decoy.txt", "tracked-ignore.txt"} {
		files[path] = "zero\n😀 inventoryneedle decoyonly\n"
		writeFile(t, repo, path, files[path])
	}
	exactInventoryGit(t, repo, nil, "add", "--", "tracked-ignore.txt")
	writeFile(t, repo, ".gitignore", "ignored.txt\ntracked-ignore.txt\n")
	files[".gitignore"] = "ignored.txt\ntracked-ignore.txt\n"
	head := strings.TrimSpace(string(exactInventoryGit(t, repo, nil, "rev-parse", "HEAD")))
	exactInventoryGit(t, repo, nil, "update-index", "--add", "--cacheinfo", "160000,"+head+",gitlink-exact")
	if runtime.GOOS != "windows" {
		for _, mode := range []string{"160000", "100644"} {
			path := mode + " " + head + " 0\tdecoy.txt"
			files[path] = "zero\n😀 inventoryneedle selected" + mode + "payload\n"
			writeFile(t, repo, path, files[path])
		}
	}
	return files
}

func exactInventoryNames(kind string) []string {
	return []string{
		"ordinary-exact-" + kind + ".txt", "日本語😀exact-" + kind + ".txt",
		" leading-exact-" + kind + ".txt", "trailing-exact-" + kind + ".txt ",
		"quoted\"exact-" + kind + ".txt", "tab\texact-" + kind + ".txt",
		"line\nexact-" + kind + ".txt", strings.Repeat(" ", len(kind)),
	}
}

func exactInventoryEligiblePaths(files map[string]string) []string {
	paths := make([]string, 0, len(files))
	for path := range files {
		if path != "ignored.txt" && path != storageworkspaces.OwnershipMarkerFilename {
			paths = append(paths, path)
		}
	}
	sort.Strings(paths)
	return paths
}

func assertExactInventoryContent(t *testing.T, tracker *WorkspaceTracker, files map[string]string) {
	t.Helper()
	results, err := tracker.SearchContent(context.Background(), "inventoryneedle", 50)
	if err != nil {
		t.Fatal(err)
	}
	got := make(map[string]string)
	for _, match := range results {
		got[match.Path] = match.Preview
		wantRanges := []types.WorkspaceContentMatchRange{{Start: 3, End: 18}}
		if match.RepositoryName != "exact-repo" || match.Line != 2 || match.Column != 4 ||
			!reflect.DeepEqual(match.MatchRanges, wantRanges) {
			t.Errorf("content match metadata = %+v", match)
		}
	}
	want := make(map[string]string)
	for _, path := range exactInventoryEligiblePaths(files) {
		if path != ".gitignore" {
			want[path] = strings.Split(files[path], "\n")[1]
		}
	}
	if !reflect.DeepEqual(got, want) || len(results) != len(want) {
		t.Errorf("content paths/previews = %q, want %q", got, want)
	}
	for _, mode := range []string{"160000", "100644"} {
		query := "selected" + mode + "payload"
		matches, searchErr := tracker.SearchContent(context.Background(), query, 50)
		if runtime.GOOS == "windows" {
			continue
		}
		if searchErr != nil || len(matches) != 1 || !strings.HasPrefix(matches[0].Path, mode+" ") {
			t.Errorf("literal metadata-lookalike query %q = %+v, error %v", query, matches, searchErr)
		}
	}
}

// @covers AC-UI-TASK-WORKSPACE-CONTENT-SEARCH-001.8
// @covers AC-UI-TASK-WORKSPACE-CONTENT-SEARCH-001.9
func TestWorkspaceInventoryPreservesTrackedModesAndStages(t *testing.T) {
	repo := newExactInventoryRepo(t)
	paths := []string{"ordinary-exact", "executable-exact", "skip-exact", "assume-exact", "unmerged-exact", "deleted-exact"}
	files := make(map[string]string)
	for _, path := range paths {
		files[path] = "modecontent\n"
		writeFile(t, repo, path, files[path])
	}
	exactInventoryGit(t, repo, nil, "add", ".")
	exactInventoryGit(t, repo, nil, "update-index", "--chmod=+x", "executable-exact")
	exactInventoryGit(t, repo, nil, "update-index", "--skip-worktree", "skip-exact")
	exactInventoryGit(t, repo, nil, "update-index", "--assume-unchanged", "assume-exact")
	object := strings.TrimSpace(string(exactInventoryGit(t, repo, nil, "hash-object", "ordinary-exact")))
	head := strings.TrimSpace(string(exactInventoryGit(t, repo, nil, "rev-parse", "HEAD")))
	header := fmt.Sprintf("0 %s\tunmerged-exact\n", strings.Repeat("0", len(object)))
	for stage := 1; stage <= 3; stage++ {
		header += fmt.Sprintf("100644 %s %d\tunmerged-exact\n", object, stage)
	}
	exactInventoryGit(t, repo, []byte(header), "update-index", "--index-info")
	exactInventoryGit(t, repo, nil, "update-index", "--add", "--cacheinfo", "160000,"+head+",gitlink-exact")
	t.Run("symlink", func(t *testing.T) {
		if err := os.Symlink("ordinary-exact", filepath.Join(repo, "symlink-exact")); err != nil {
			if runtime.GOOS == "windows" {
				t.Skipf("native symlink unavailable: %v", err)
			}
			t.Fatal(err)
		}
		linkObject := strings.TrimSpace(string(exactInventoryGit(t, repo, []byte("ordinary-exact"), "hash-object", "-w", "--stdin")))
		exactInventoryGit(t, repo, nil, "update-index", "--add", "--cacheinfo", "120000,"+linkObject+",symlink-exact")
		paths = append(paths, "symlink-exact")
		files["symlink-exact"] = files["ordinary-exact"]
	})
	if err := os.Remove(filepath.Join(repo, "deleted-exact")); err != nil {
		t.Fatal(err)
	}
	delete(files, "deleted-exact")
	before := exactInventorySnapshot(t, repo, files)
	tracker := NewWorkspaceTrackerForRepo(repo, "modes", newTestLogger(t))
	t.Cleanup(tracker.Stop)
	list, err := tracker.getFileList(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	counts := make(map[string]int)
	for _, file := range list.Files {
		counts[file.Path]++
	}
	want := make(map[string]int)
	for _, path := range paths {
		want[path] = 1
	}
	want["unmerged-exact"] = 3
	if !reflect.DeepEqual(counts, want) {
		t.Errorf("indexed modes/stages = %v, want %v", counts, want)
	}
	tracker.updateFiles(context.Background())
	if got := tracker.SearchFiles("unmerged-exact", 50); len(got) != 3 {
		t.Errorf("unmerged cached multiplicity = %q, want three", got)
	}
	content, contentErr := tracker.SearchContent(context.Background(), "modecontent", 50)
	if contentErr != nil {
		t.Fatal(contentErr)
	}
	contentCounts := make(map[string]int)
	for _, match := range content {
		contentCounts[match.Path]++
	}
	delete(want, "deleted-exact")
	if !reflect.DeepEqual(contentCounts, want) {
		t.Errorf("indexed content multiplicity = %v, want %v", contentCounts, want)
	}
	if after := exactInventorySnapshot(t, repo, files); !reflect.DeepEqual(after, before) {
		t.Error("indexed mode enumeration changed Git or fixture bytes")
	}
}

func newExactInventoryRepo(t *testing.T) string {
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
	repo := t.TempDir()
	exactInventoryGit(t, repo, nil, "init", "-b", "main")
	exactInventoryGit(t, repo, nil, "config", "user.name", "Inventory Test")
	exactInventoryGit(t, repo, nil, "config", "user.email", "inventory@example.com")
	exactInventoryGit(t, repo, nil, "config", "core.autocrlf", "false")
	exactInventoryGit(t, repo, nil, "commit", "--allow-empty", "-m", "fixture")
	return repo
}

func exactInventoryGit(t *testing.T, repo string, input []byte, args ...string) []byte {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", repo, "-c", "commit.gpgsign=false"}, args...)...)
	cmd.Stdin = bytes.NewReader(input)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v: %s", args, err, output)
	}
	return output
}

func exactInventorySnapshot(t *testing.T, repo string, files map[string]string) map[string]string {
	t.Helper()
	snapshot := make(map[string]string)
	for _, path := range append([]string{".git/HEAD", ".git/index", ".git/config"}, exactInventoryAllPaths(files)...) {
		data, err := os.ReadFile(filepath.Join(repo, path))
		if err != nil {
			t.Fatal(err)
		}
		snapshot[path] = string(data)
	}
	snapshot["refs"] = string(exactInventoryGit(t, repo, nil, "for-each-ref", "--format=%(refname) %(objectname)"))
	return snapshot
}

func exactInventoryAllPaths(files map[string]string) []string {
	paths := make([]string, 0, len(files))
	for path := range files {
		paths = append(paths, path)
	}
	return paths
}

func containsExactInventoryPath(paths []string, want string) bool {
	for _, path := range paths {
		if path == want {
			return true
		}
	}
	return false
}
