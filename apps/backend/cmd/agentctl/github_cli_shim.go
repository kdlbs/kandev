package main

import (
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

const envGitHubCLIShimDir = "KANDEV_GITHUB_CLI_SHIM_DIR"

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
	client, err := newGitHubCredentialBrokerClient(getenv, httpClient)
	if err != nil {
		return err
	}
	credential, err := client.resolve(ctx)
	if err != nil {
		return err
	}
	realPath := pathWithoutDirectory(getenv("PATH"), shimDir)
	executable, err := lookPath("gh", realPath)
	if err != nil {
		return fmt.Errorf("find real gh CLI: %w", err)
	}
	configDir, err := isolatedGitHubCLIConfigDir(client.request.TaskID, client.request.SessionID)
	if err != nil {
		return err
	}
	childEnv := replaceEnvironment(environ(), map[string]string{
		"GH_TOKEN":      credential.Password,
		"GH_CONFIG_DIR": configDir,
		"PATH":          realPath,
	}, "GITHUB_TOKEN")
	return runner(ctx, executable, args, childEnv, stdin, stdout, stderr)
}

func isolatedGitHubCLIConfigDir(taskID, sessionID string) (string, error) {
	digest := sha256.Sum256([]byte(taskID + "\x00" + sessionID))
	dir := filepath.Join(os.TempDir(), fmt.Sprintf("kandev-gh-%x", digest[:8]))
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", fmt.Errorf("create isolated gh config directory: %w", err)
	}
	if err := os.Chmod(dir, 0o700); err != nil {
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
	dir, err := os.MkdirTemp(tempRoot, "kandev-github-cli-")
	if err != nil {
		return "", nil, fmt.Errorf("create gh shim directory: %w", err)
	}
	cleanup := func() { _ = os.RemoveAll(dir) }
	shim := filepath.Join(dir, githubCLIShimName())
	if err := linkOrCopyExecutable(agentctlExecutable, shim); err != nil {
		cleanup()
		return "", nil, fmt.Errorf("install gh shim: %w", err)
	}
	return dir, cleanup, nil
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

func lookPathIn(file, path string) (string, error) {
	names := []string{file}
	if runtime.GOOS == windowsOS && filepath.Ext(file) == "" {
		names = []string{file + ".exe", file + ".cmd", file + ".bat", file}
	}
	for _, directory := range filepath.SplitList(path) {
		for _, name := range names {
			candidate := filepath.Join(directory, name)
			info, err := os.Stat(candidate)
			if err == nil && !info.IsDir() && (runtime.GOOS == windowsOS || info.Mode()&0o111 != 0) {
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
