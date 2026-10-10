package api

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agentctl/server/config"
	"github.com/kandev/kandev/internal/agentctl/server/process"
	"github.com/kandev/kandev/internal/common/logger"
)

const ignoredHTTPGitlink = "deps/library"

type ignoredHTTPRepo struct {
	scope, parent, child, base, head, old, next string
}

func ignoredHTTPEnv(t *testing.T) []string {
	t.Helper()
	privateHome := t.TempDir()
	for _, key := range []string{"HOME", "USERPROFILE", "XDG_CONFIG_HOME"} {
		t.Setenv(key, privateHome)
	}
	var env []string
	for _, entry := range os.Environ() {
		if !strings.HasPrefix(entry, "GIT_") {
			env = append(env, entry)
		}
	}
	return append(env, "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL="+os.DevNull)
}

func ignoredHTTPGit(t *testing.T, env []string, repo string, args ...string) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, "git", append([]string{"-C", repo}, args...)...)
	command.Env = append([]string(nil), env...)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("HTTP fixture git %v: %v\n%s", args, err, output)
	}
	return string(output)
}

func ignoredHTTPInit(t *testing.T, env []string, dir string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	ignoredHTTPGit(t, env, dir, "init", "--initial-branch=main")
	for key, value := range map[string]string{
		"user.name": "Ignored Gitlink HTTP", "user.email": "ignored-http@test.invalid",
		"commit.gpgsign": "false", "core.autocrlf": "false", "core.hooksPath": os.DevNull,
	} {
		ignoredHTTPGit(t, env, dir, "config", key, value)
	}
}

func ignoredHTTPSeed(t *testing.T, env []string, root, scope string) ignoredHTTPRepo {
	t.Helper()
	r := ignoredHTTPRepo{scope: scope, parent: filepath.Join(root, scope)}
	source := t.TempDir()
	ignoredHTTPInit(t, env, source)
	writeFileAPI(t, source, "payload.txt", scope+" old bytes\n")
	ignoredHTTPGit(t, env, source, "add", ".")
	ignoredHTTPGit(t, env, source, "commit", "-m", scope+" child baseline")
	r.old = strings.TrimSpace(ignoredHTTPGit(t, env, source, "rev-parse", "HEAD"))
	writeFileAPI(t, source, "payload.txt", scope+" new bytes\n")
	ignoredHTTPGit(t, env, source, "commit", "-am", scope+" child upgrade")
	r.next = strings.TrimSpace(ignoredHTTPGit(t, env, source, "rev-parse", "HEAD"))
	ignoredHTTPInit(t, env, r.parent)
	ignoredHTTPGit(t, env, r.parent, "-c", "protocol.file.allow=always", "submodule", "add", filepath.ToSlash(source), ignoredHTTPGitlink)
	r.child = filepath.Join(r.parent, filepath.FromSlash(ignoredHTTPGitlink))
	ignoredHTTPGit(t, env, r.child, "checkout", "--detach", r.old)
	ignoredHTTPGit(t, env, r.parent, "add", ".")
	ignoredHTTPGit(t, env, r.parent, "commit", "-m", scope+" parent baseline")
	r.base = strings.TrimSpace(ignoredHTTPGit(t, env, r.parent, "rev-parse", "HEAD"))
	ignoredHTTPGit(t, env, r.parent, "checkout", "-b", "feature/upgrade")
	ignoredHTTPGit(t, env, r.child, "checkout", "--detach", r.next)
	ignoredHTTPGit(t, env, r.parent, "add", ignoredHTTPGitlink)
	ignoredHTTPGit(t, env, r.parent, "commit", "-m", scope+" dependency upgrade")
	r.head = strings.TrimSpace(ignoredHTTPGit(t, env, r.parent, "rev-parse", "HEAD"))
	return r
}

func ignoredHTTPFixture(t *testing.T, mode string, initialized bool) (*Server, []ignoredHTTPRepo, []string) {
	t.Helper()
	env, root := ignoredHTTPEnv(t), t.TempDir()
	repos := []ignoredHTTPRepo{ignoredHTTPSeed(t, env, root, "alpha"), ignoredHTTPSeed(t, env, root, "beta")}
	if repos[0].base == repos[1].base || repos[0].old == repos[1].old || repos[0].next == repos[1].next {
		t.Fatal("HTTP routing fixtures must have independent parent and child identities")
	}
	for _, repo := range repos {
		if mode != "default" {
			ignoredHTTPGit(t, env, repo.parent, "config", "diff.ignoreSubmodules", mode)
		}
		if !initialized {
			ignoredHTTPGit(t, env, repo.parent, "submodule", "deinit", "--force", "--all")
		}
	}
	return ignoredHTTPServer(t, env, root), repos, env
}

func ignoredHTTPServer(t *testing.T, env []string, root string) *Server {
	t.Helper()
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error"})
	if err != nil {
		t.Fatal(err)
	}
	cfg := &config.InstanceConfig{WorkDir: root, AgentEnv: append([]string(nil), env...), BaseBranches: map[string]string{"alpha": "main", "beta": "main"}}
	manager := process.NewManager(cfg, log)
	t.Cleanup(func() {
		if err := manager.StopForTeardown(context.Background()); err != nil {
			t.Error(err)
		}
	})
	return NewServer(cfg, manager, nil, nil, log)
}

func ignoredHTTPPatch(t *testing.T, env []string, repo, base string) string {
	t.Helper()
	return ignoredHTTPGit(t, env, repo, "diff", "--no-color", "--no-ext-diff", "--no-textconv", "--submodule=short", "--ignore-submodules=none", "--src-prefix=a/", "--dst-prefix=b/", base)
}

func ignoredHTTPState(t *testing.T, repos []ignoredHTTPRepo, env []string) map[string]string {
	t.Helper()
	state := map[string]string{}
	for _, r := range repos {
		for _, dir := range []string{r.parent, r.child} {
			for _, name := range []string{".git", ".gitmodules", "payload.txt"} {
				data, err := os.ReadFile(filepath.Join(dir, name))
				switch {
				case err == nil:
					state[dir+name] = string(data)
				case os.IsNotExist(err):
					state[dir+name] = "absent"
				case name != ".git":
					t.Fatal(err)
				}
			}
			if dir == r.child && state[dir+".git"] == "absent" {
				continue
			}
			state[dir+"head"] = ignoredHTTPGit(t, env, dir, "rev-parse", "HEAD")
			state[dir+"refs"] = ignoredHTTPGit(t, env, dir, "show-ref", "--head")
			state[dir+"index"] = ignoredHTTPGit(t, env, dir, "ls-files", "--stage", "-z")
			state[dir+"status"] = ignoredHTTPGit(t, env, dir, "--no-optional-locks", "status", "--porcelain=v1", "--ignore-submodules=none", "-z")
			state[dir+"patch"] = ignoredHTTPPatch(t, env, dir, "HEAD")
		}
		for _, name := range []string{"config", "index", "modules/" + ignoredHTTPGitlink + "/config", "modules/" + ignoredHTTPGitlink + "/HEAD", "modules/" + ignoredHTTPGitlink + "/index"} {
			p := strings.TrimSpace(ignoredHTTPGit(t, env, r.parent, "rev-parse", "--git-path", name))
			if !filepath.IsAbs(p) {
				p = filepath.Join(r.parent, p)
			}
			data, err := os.ReadFile(p)
			if err != nil {
				t.Fatal(err)
			}
			state[r.parent+name] = string(data)
		}
	}
	return state
}

func readIgnoredHTTP(t *testing.T, server *Server, repos []ignoredHTTPRepo, env []string, route string, result interface{}) {
	t.Helper()
	before := ignoredHTTPState(t, repos, env)
	readStatusMetadataHTTP(t, server, route, result)
	if after := ignoredHTTPState(t, repos, env); !reflect.DeepEqual(before, after) {
		t.Error("registered comparison changed owned repository state")
	}
}

func assertIgnoredHTTPParent(t *testing.T, files map[string]interface{}, r ignoredHTTPRepo, env []string, aggregate bool) {
	t.Helper()
	key := ignoredHTTPGitlink
	if aggregate {
		key = r.scope + "\x00" + key
	}
	patch := ignoredHTTPPatch(t, env, r.parent, r.base)
	for _, id := range []string{r.old, r.next} {
		if !strings.Contains(patch, "Subproject commit "+id) {
			t.Fatalf("raw built-in oracle missing child %s", id)
		}
	}
	file := assertStatusMetadataHTTPFile(t, files, key, ignoredHTTPGitlink, "modified", 1, 1, patch)
	if aggregate && (file["repository_name"] != r.scope || file["base_ref"] != r.base) {
		t.Errorf("parent repository metadata = %#v", file)
	}
	if _, present := file["is_submodule"]; present {
		t.Errorf("parent gitlink unexpectedly marked as child-scope file: %#v", file)
	}
}

// @covers AC-PLATFORM-GIT-DIFF-FILE-METADATA-001.14
func TestGitComparisonIgnoreSubmodulesHTTP(t *testing.T) {
	for _, mode := range []string{"default", "all"} {
		t.Run(mode, func(t *testing.T) {
			server, repos, env := ignoredHTTPFixture(t, mode, false)
			for _, r := range repos {
				t.Run(r.scope+"_commit", func(t *testing.T) {
					var result process.CommitDiffResult
					readIgnoredHTTP(t, server, repos, env, "/api/v1/git/commit/"+r.head+"?repo="+r.scope, &result)
					metadata := strings.Split(strings.TrimSpace(ignoredHTTPGit(t, env, r.parent, "show", "--no-patch", "--format=%H%n%s%n%an <%ae>%n%aI", r.head)), "\n")
					if !result.Success || result.CommitSHA != metadata[0] || result.Message != metadata[1] || result.Author != metadata[2] || result.Date != metadata[3] || result.FilesChanged != 1 || result.Insertions != 1 || result.Deletions != 1 || len(result.Files) != 1 {
						t.Errorf("selected commit = %+v", result)
					}
					assertIgnoredHTTPParent(t, result.Files, r, env, false)
				})
				t.Run(r.scope+"_cumulative", func(t *testing.T) {
					var result process.CumulativeDiffResult
					readIgnoredHTTP(t, server, repos, env, "/api/v1/git/cumulative-diff?repo="+r.scope+"&base="+r.base, &result)
					if !result.Success || result.BaseCommit != r.base || result.HeadCommit != r.head || result.TotalCommits != 1 || result.TruncatedFilesCount != 0 || len(result.Files) != 1 {
						t.Errorf("selected cumulative = %+v", result)
					}
					assertIgnoredHTTPParent(t, result.Files, r, env, false)
				})
			}
		})
	}
}

func assertIgnoredHTTPAggregate(t *testing.T, server *Server, repos []ignoredHTTPRepo, env []string, initialized bool) {
	t.Helper()
	var result process.CumulativeDiffResult
	readIgnoredHTTP(t, server, repos, env, "/api/v1/git/cumulative-diff?base="+repos[0].base, &result)
	expected := 2
	if initialized {
		expected = 4
	}
	if !result.Success || len(result.Files) != expected || result.TotalCommits != expected || result.TruncatedFilesCount != 0 {
		t.Errorf("aggregate fallback/child counts = %+v", result)
	}
	for _, r := range repos {
		t.Run(r.scope, func(t *testing.T) {
			assertIgnoredHTTPParent(t, result.Files, r, env, true)
			if initialized {
				scope := r.scope + "/" + ignoredHTTPGitlink
				patch := ignoredHTTPPatch(t, env, r.child, r.old)
				child := assertStatusMetadataHTTPFile(t, result.Files, scope+"\x00payload.txt", "payload.txt", "modified", 1, 1, patch)
				if child["repository_name"] != scope || child["base_ref"] != r.old || child["is_submodule"] != true {
					t.Errorf("initialized child identity/parent anchor = %#v", child)
				}
			}
		})
	}
}

// @covers AC-PLATFORM-GIT-DIFF-FILE-METADATA-001.3
// @covers AC-PLATFORM-GIT-DIFF-FILE-METADATA-001.14
func TestGitComparisonIgnoreSubmodulesMultiRepoHTTP(t *testing.T) {
	for _, mode := range []string{"default", "all"} {
		t.Run(mode, func(t *testing.T) {
			server, repos, env := ignoredHTTPFixture(t, mode, false)
			assertIgnoredHTTPAggregate(t, server, repos, env, false)
		})
	}
}

// @covers AC-PLATFORM-GIT-DIFF-FILE-METADATA-001.13
func TestGitComparisonIgnoreSubmodulesInitializedChildHTTP(t *testing.T) {
	server, repos, env := ignoredHTTPFixture(t, "all", true)
	assertIgnoredHTTPAggregate(t, server, repos, env, true)
}

// @covers AC-PLATFORM-GIT-DIFF-FILE-METADATA-001.13
// @covers AC-PLATFORM-GIT-DIFF-FILE-METADATA-001.14
func TestGitComparisonIgnoreSubmodulesChildDirtHTTP(t *testing.T) {
	env, root := ignoredHTTPEnv(t), t.TempDir()
	repos := []ignoredHTTPRepo{ignoredHTTPSeed(t, env, root, "alpha"), ignoredHTTPSeed(t, env, root, "beta")}
	for i := range repos {
		r := &repos[i]
		ignoredHTTPGit(t, env, r.parent, "config", "diff.ignoreSubmodules", "all")
		ignoredHTTPGit(t, env, r.parent, "update-ref", "refs/heads/main", r.head)
		r.base, r.old = r.head, r.next
		writeFileAPI(t, r.child, "payload.txt", r.scope+" dirty child bytes\n")
		if !strings.Contains(ignoredHTTPPatch(t, env, r.parent, r.base), "-dirty") {
			t.Fatal("ignore-none parent control did not expose child dirt")
		}
		patch := ignoredHTTPGit(t, env, r.parent, "diff", "--no-color", "--no-ext-diff", "--no-textconv", "--submodule=short", "--ignore-submodules=dirty", "--src-prefix=a/", "--dst-prefix=b/", r.base)
		if patch != "" {
			t.Fatalf("unchanged parent pointer oracle = %q", patch)
		}
	}
	server := ignoredHTTPServer(t, env, root)
	var result process.CumulativeDiffResult
	readIgnoredHTTP(t, server, repos, env, "/api/v1/git/cumulative-diff?base="+repos[0].base, &result)
	if !result.Success || result.TotalCommits != 0 || result.TruncatedFilesCount != 0 || len(result.Files) != 2 {
		t.Errorf("aggregate child-only dirt = %+v", result)
	}
	for _, r := range repos {
		if parent, present := result.Files[r.scope+"\x00"+ignoredHTTPGitlink]; present {
			t.Errorf("unchanged parent pointer returned child dirt: %#v", parent)
		}
		scope := r.scope + "/" + ignoredHTTPGitlink
		patch := ignoredHTTPPatch(t, env, r.child, r.old)
		if !strings.Contains(patch, "+"+r.scope+" dirty child bytes") {
			t.Fatal("child oracle did not preserve independently owned dirty bytes")
		}
		child := assertStatusMetadataHTTPFile(t, result.Files, scope+"\x00payload.txt", "payload.txt", "modified", 1, 1, patch)
		if child["repository_name"] != scope || child["base_ref"] != r.old || child["is_submodule"] != true {
			t.Errorf("dirty child scope/parent anchor = %#v", child)
		}
	}
}
