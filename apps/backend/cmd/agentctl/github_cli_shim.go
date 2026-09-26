package main

import (
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"github.com/kandev/kandev/internal/common/subproc"
	"github.com/kandev/kandev/internal/githubauth"
)

const envGitHubCLIShimDir = githubauth.CredentialCLIShimDirEnv

// githubCLIShimDirPrefix names every directory installGitHubCLIShim creates.
// The real-gh lookup never returns a gh inside such a directory, whichever
// agentctl it links to or copies.
const githubCLIShimDirPrefix = "kandev-github-cli-"

// envGitHubCLIShimDepth is the number of shims above the current process. The
// shim passes depth+1 to the gh it launches and refuses to run at
// maxGitHubCLIShimDepth. Nesting below the bound is valid: a gh extension may
// call gh through a Bash whose BASH_ENV restores the shim directory.
const envGitHubCLIShimDepth = "KANDEV_GITHUB_CLI_SHIM_DEPTH"

const maxGitHubCLIShimDepth = 8

const windowsOS = "windows"

type githubCLILookPath func(file, path string) (string, error)

type githubCLICommandRunner func(
	ctx context.Context,
	executable string,
	args []string,
	env []string,
	stdin io.Reader,
	stdout, stderr io.Writer,
) error

func runGitHubCLIShim(
	ctx context.Context,
	args []string,
	stdin io.Reader,
	stdout, stderr io.Writer,
	getenv func(string) string,
	environ func() []string,
	httpClient *http.Client,
	shimDir string,
	lookPath githubCLILookPath,
	runner githubCLICommandRunner,
) error {
	depth, err := githubCLIShimDepth(getenv(envGitHubCLIShimDepth))
	if err != nil {
		return err
	}
	client, err := newGitHubCLIShimCredentialBrokerClient(ctx, args, getenv, httpClient)
	if err != nil {
		return err
	}
	credential, err := client.resolveWithReissue(ctx)
	if err != nil {
		return err
	}
	realPath := pathWithoutDirectory(getenv("PATH"), shimDir)
	realPath = pathWithoutGitHubCLIShimDirectories(realPath)
	executable, err := lookPath("gh", realPath)
	if err != nil {
		return fmt.Errorf("find real gh CLI: %w", err)
	}
	configDir, err := isolatedGitHubCLIConfigDir(client.request.TaskID, client.request.SessionID)
	if err != nil {
		return err
	}
	defer func() { _ = os.RemoveAll(configDir) }()
	childEnv := replaceEnvironment(environ(), map[string]string{
		"GH_TOKEN":            credential.Password,
		"GH_CONFIG_DIR":       configDir,
		"PATH":                realPath,
		envGitHubCLIShimDepth: strconv.Itoa(depth + 1),
	}, "GITHUB_TOKEN")
	return runner(ctx, executable, args, childEnv, stdin, stdout, stderr)
}

// githubCLIShimDepth parses the inherited shim depth; the bound is exclusive.
func githubCLIShimDepth(raw string) (int, error) {
	depth := 0
	if raw = strings.TrimSpace(raw); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 0 {
			return 0, fmt.Errorf("invalid %s %q", envGitHubCLIShimDepth, raw)
		}
		depth = parsed
	}
	if depth >= maxGitHubCLIShimDepth {
		return 0, fmt.Errorf(
			"gh shim nested %d times: the gh found on PATH is a Kandev shim, not the GitHub CLI",
			depth,
		)
	}
	return depth, nil
}

type githubCLIRepository struct {
	host  string
	owner string
	repo  string
}

func newGitHubCLIShimCredentialBrokerClient(
	ctx context.Context,
	args []string,
	getenv func(string) string,
	httpClient *http.Client,
) (*githubCredentialBrokerClient, error) {
	if strings.TrimSpace(getenv(githubauth.CredentialScopesEnv)) == "" {
		return newGitHubCredentialBrokerClient(getenv, httpClient)
	}
	target, err := resolveGitHubCLIRepository(ctx, args, getenv)
	if err != nil {
		return nil, err
	}
	if target == nil {
		return newGitHubCredentialBrokerClient(getenv, httpClient)
	}
	client, err := newGitHubCredentialBrokerClientForInput(getenv, httpClient, map[string]string{
		"protocol": "https",
		"host":     target.host,
		"path":     target.owner + "/" + target.repo,
	})
	if err != nil {
		return nil, fmt.Errorf("select GitHub credential for repository: %w", err)
	}
	return client, nil
}

func resolveGitHubCLIRepository(
	ctx context.Context,
	args []string,
	getenv func(string) string,
) (*githubCLIRepository, error) {
	defaultHost := strings.TrimSpace(getenv(githubauth.CredentialHostEnv))
	raw, found, err := githubCLIRepositoryArgument(args)
	if err != nil {
		return nil, err
	}
	if found {
		return parseGitHubCLIRepository(raw, defaultHost)
	}
	if raw = strings.TrimSpace(getenv("GH_REPO")); raw != "" {
		return parseGitHubCLIRepository(raw, defaultHost)
	}
	cmd := subproc.NewGitCommand(ctx, "remote", "get-url", "origin")
	output, err := subproc.RunGitOutputClass(ctx, subproc.GitInteractive, cmd)
	if err != nil {
		return nil, nil
	}
	target, err := parseGitHubCLIRepository(strings.TrimSpace(string(output)), defaultHost)
	if err != nil {
		return nil, nil
	}
	return target, nil
}

func githubCLIRepositoryArgument(args []string) (string, bool, error) {
	for index, arg := range args {
		if arg == "--" {
			break
		}
		switch {
		case arg == "-R" || arg == "--repo":
			if index+1 >= len(args) || strings.TrimSpace(args[index+1]) == "" {
				return "", true, fmt.Errorf("GitHub CLI repository argument is empty")
			}
			return args[index+1], true, nil
		case strings.HasPrefix(arg, "--repo="):
			return strings.TrimPrefix(arg, "--repo="), true, nil
		case strings.HasPrefix(arg, "-R="):
			return strings.TrimPrefix(arg, "-R="), true, nil
		case strings.HasPrefix(arg, "-R") && len(arg) > 2:
			return strings.TrimPrefix(arg, "-R"), true, nil
		}
	}
	return "", false, nil
}

func parseGitHubCLIRepository(raw, defaultHost string) (*githubCLIRepository, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, fmt.Errorf("GitHub CLI repository is empty")
	}
	if strings.Contains(raw, "://") {
		parsed, err := url.Parse(raw)
		if err != nil || parsed.Hostname() == "" {
			return nil, fmt.Errorf("GitHub CLI repository %q is invalid", raw)
		}
		return repositoryFromHostAndPath(parsed.Hostname(), parsed.Path, raw)
	}
	if before, path, found := strings.Cut(raw, ":"); found && strings.Contains(before, "@") {
		host := before[strings.LastIndex(before, "@")+1:]
		return repositoryFromHostAndPath(host, path, raw)
	}
	parts := strings.Split(strings.Trim(raw, "/"), "/")
	if len(parts) == 2 {
		if defaultHost == "" {
			defaultHost = "github.com"
		}
		return repositoryFromHostAndPath(defaultHost, strings.Join(parts, "/"), raw)
	}
	if len(parts) == 3 {
		return repositoryFromHostAndPath(parts[0], strings.Join(parts[1:], "/"), raw)
	}
	return nil, fmt.Errorf("GitHub CLI repository %q must be [HOST/]OWNER/REPO", raw)
}

func repositoryFromHostAndPath(host, path, raw string) (*githubCLIRepository, error) {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) != 2 || strings.TrimSpace(host) == "" || parts[0] == "" || parts[1] == "" {
		return nil, fmt.Errorf("GitHub CLI repository %q must be [HOST/]OWNER/REPO", raw)
	}
	repo := strings.TrimSuffix(parts[1], ".git")
	if repo == "" {
		return nil, fmt.Errorf("GitHub CLI repository %q must include a repository name", raw)
	}
	return &githubCLIRepository{
		host:  strings.ToLower(strings.TrimSpace(host)),
		owner: parts[0],
		repo:  repo,
	}, nil
}

func isolatedGitHubCLIConfigDir(taskID, sessionID string) (string, error) {
	digest := sha256.Sum256([]byte(taskID + "\x00" + sessionID))
	dir, err := os.MkdirTemp(os.TempDir(), fmt.Sprintf("kandev-gh-%x-", digest[:8]))
	if err != nil {
		return "", fmt.Errorf("create isolated gh config directory: %w", err)
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		_ = os.RemoveAll(dir)
		return "", fmt.Errorf("secure isolated gh config directory: %w", err)
	}
	return dir, nil
}

func replaceEnvironment(base []string, replacements map[string]string, remove ...string) []string {
	dropped := make(map[string]struct{}, len(replacements)+len(remove))
	for key := range replacements {
		dropped[strings.ToUpper(key)] = struct{}{}
	}
	for _, key := range remove {
		dropped[strings.ToUpper(key)] = struct{}{}
	}
	result := make([]string, 0, len(base)+len(replacements))
	for _, entry := range base {
		key, _, ok := strings.Cut(entry, "=")
		if _, drop := dropped[strings.ToUpper(key)]; ok && drop {
			continue
		}
		result = append(result, entry)
	}
	for key, value := range replacements {
		result = append(result, key+"="+value)
	}
	return result
}

func pathWithoutDirectory(path, excluded string) string {
	cleanExcluded := filepath.Clean(excluded)
	parts := filepath.SplitList(path)
	filtered := parts[:0]
	for _, part := range parts {
		if filepath.Clean(part) != cleanExcluded {
			filtered = append(filtered, part)
		}
	}
	return strings.Join(filtered, string(os.PathListSeparator))
}

func pathWithoutGitHubCLIShimDirectories(path string) string {
	parts := filepath.SplitList(path)
	filtered := parts[:0]
	for _, part := range parts {
		if !isGitHubCLIShimDir(part) {
			filtered = append(filtered, part)
		}
	}
	return strings.Join(filtered, string(os.PathListSeparator))
}

func githubCLIShimName() string {
	if runtime.GOOS == windowsOS {
		return "gh.exe"
	}
	return "gh"
}

func isGitHubCLIShimInvocation(argv0 string) bool {
	name := filepath.Base(strings.ReplaceAll(argv0, "\\", "/"))
	return strings.EqualFold(name, "gh") || strings.EqualFold(name, "gh.exe")
}

func installGitHubCLIShim(agentctlExecutable, tempRoot string) (string, func(), error) {
	dir, err := os.MkdirTemp(tempRoot, githubCLIShimDirPrefix)
	if err != nil {
		return "", nil, fmt.Errorf("create gh shim directory: %w", err)
	}
	cleanup := func() { _ = os.RemoveAll(dir) }
	shim := filepath.Join(dir, githubCLIShimName())
	if err := linkOrCopyExecutable(agentctlExecutable, shim); err != nil {
		cleanup()
		return "", nil, fmt.Errorf("install gh shim: %w", err)
	}
	if err := linkOrCopyExecutable(agentctlExecutable, filepath.Join(dir, "agentctl")); err != nil {
		cleanup()
		return "", nil, fmt.Errorf("install agentctl helper: %w", err)
	}
	if err := installGitHubCLIBashEnv(dir); err != nil {
		cleanup()
		return "", nil, fmt.Errorf("install Bash environment: %w", err)
	}
	return dir, cleanup, nil
}

func installGitHubCLIBashEnv(shimDir string) error {
	path := filepath.Join(shimDir, githubauth.CLIBashEnvFilename)
	const script = `#!/bin/sh
if [ -n "${KANDEV_GITHUB_PARENT_BASH_ENV:-}" ] &&
   [ "${KANDEV_GITHUB_PARENT_BASH_ENV}" != "${KANDEV_GITHUB_CLI_BASH_ENV:-}" ]; then
  . "$KANDEV_GITHUB_PARENT_BASH_ENV"
fi
if [ -n "${KANDEV_GITHUB_CLI_SHIM_DIR:-}" ]; then
  case ":${PATH:-}:" in
    *:"${KANDEV_GITHUB_CLI_SHIM_DIR}":*) ;;
    *) PATH="${KANDEV_GITHUB_CLI_SHIM_DIR}:${PATH:-}"; export PATH ;;
  esac
fi
`
	return os.WriteFile(path, []byte(script), 0o700)
}

func linkOrCopyExecutable(source, target string) error {
	if err := os.Symlink(source, target); err == nil {
		return nil
	}
	if err := os.Link(source, target); err == nil {
		return nil
	}
	input, err := os.Open(source)
	if err != nil {
		return err
	}
	defer func() { _ = input.Close() }()
	output, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o700)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(output, input)
	closeErr := output.Close()
	if copyErr != nil {
		return copyErr
	}
	return closeErr
}

// lookPathSkippingShims is lookPathIn, except that it never returns a Kandev gh
// shim: neither a candidate inside a githubCLIShimDirPrefix directory nor the
// given executable itself, compared by file identity so links to it are skipped
// too. self may be empty when the running executable is unknown.
func lookPathSkippingShims(self string) githubCLILookPath {
	var selfInfo os.FileInfo
	if self != "" {
		if info, err := os.Stat(self); err == nil {
			selfInfo = info
		}
	}
	return func(file, path string) (string, error) {
		return lookPathMatching(file, path, func(candidate string, info os.FileInfo) bool {
			if isGitHubCLIShimDir(filepath.Dir(candidate)) {
				return false
			}
			return selfInfo == nil || !os.SameFile(info, selfInfo)
		})
	}
}

func isGitHubCLIShimDir(dir string) bool {
	return strings.HasPrefix(filepath.Base(dir), githubCLIShimDirPrefix)
}

func lookPathIn(file, path string) (string, error) {
	return lookPathMatching(file, path, func(string, os.FileInfo) bool { return true })
}

func lookPathMatching(file, path string, accept func(string, os.FileInfo) bool) (string, error) {
	names := []string{file}
	if runtime.GOOS == windowsOS && filepath.Ext(file) == "" {
		names = []string{file + ".exe", file + ".cmd", file + ".bat", file}
	}
	for _, directory := range filepath.SplitList(path) {
		for _, name := range names {
			candidate := filepath.Join(directory, name)
			info, err := os.Stat(candidate)
			if err == nil && !info.IsDir() && (runtime.GOOS == windowsOS || info.Mode()&0o111 != 0) && accept(candidate, info) {
				return candidate, nil
			}
		}
	}
	return "", exec.ErrNotFound
}

func executeGitHubCLI(
	ctx context.Context,
	executable string,
	args []string,
	env []string,
	stdin io.Reader,
	stdout, stderr io.Writer,
) error {
	cmd := exec.CommandContext(ctx, executable, args...)
	cmd.Env = env
	cmd.Stdin = stdin
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	return cmd.Run()
}
