package gitlab

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// glabAuthTimeout bounds glab subprocess execution.
const glabAuthTimeout = 10 * time.Second

// GLabClient implements Client by piggy-backing on the user's glab CLI
// configuration. It discovers the host and token via `glab auth status`,
// then delegates every API call to an embedded PATClient.
//
// Versus a pure shell-out client, this approach is simpler (no per-command
// JSON parsing) and stays consistent with the REST surface that
// PATClient exercises in tests.
type GLabClient struct {
	*PATClient
	version string
}

// GLabAvailable checks if the glab CLI is installed and on PATH.
func GLabAvailable() bool {
	_, err := exec.LookPath("glab")
	return err == nil
}

// NewGLabClient discovers glab's configured host and token for the given
// targetHost (or the default if empty), and returns a Client that signs
// requests with that token. Returns an error when glab is not available
// or not authenticated for targetHost.
func NewGLabClient(ctx context.Context, targetHost string) (*GLabClient, error) {
	if !GLabAvailable() {
		return nil, errors.New("glab CLI not installed")
	}
	host := targetHost
	if host == "" {
		host = DefaultHost
	}
	hostname := stripScheme(host)

	token, err := glabReadToken(ctx, hostname)
	if err != nil {
		return nil, err
	}
	if token == "" {
		return nil, fmt.Errorf("glab not authenticated for %s", hostname)
	}
	pat := NewPATClient(host, token)
	return &GLabClient{
		PATClient: pat,
		version:   glabVersion(ctx),
	}, nil
}

// Version reports the glab CLI version (best effort, "" if unavailable).
func (c *GLabClient) Version() string { return c.version }

// RunAuthDiagnostics executes `glab auth status` and captures the raw
// output for troubleshooting. Mirrors GitHub's RunAuthDiagnostics.
func (c *GLabClient) RunAuthDiagnostics(ctx context.Context) *AuthDiagnostics {
	cctx, cancel := context.WithTimeout(ctx, glabAuthTimeout)
	defer cancel()
	cmd := exec.CommandContext(cctx, "glab", "auth", "status", "--hostname", stripScheme(c.Host()))
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	exitCode := 0
	if err := cmd.Run(); err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			exitCode = exitErr.ExitCode()
		} else {
			exitCode = -1
		}
	}
	output := stderr.String()
	if output == "" {
		output = stdout.String()
	}
	return &AuthDiagnostics{
		Command:  "glab auth status --hostname " + stripScheme(c.Host()),
		Output:   output,
		ExitCode: exitCode,
	}
}

// glabReadToken extracts the token glab uses for the given hostname. glab's
// `auth status -t` prints the token to stderr; this function captures both
// streams and parses the line "Token: <value>".
func glabReadToken(ctx context.Context, hostname string) (string, error) {
	cctx, cancel := context.WithTimeout(ctx, glabAuthTimeout)
	defer cancel()
	cmd := exec.CommandContext(cctx, "glab", "auth", "status", "--hostname", hostname, "-t")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		// Even when glab exits non-zero we may still have a token printed —
		// e.g. a different account on the same host triggers a warning. Try
		// to parse before giving up.
		token := parseGlabToken(stdout.String() + "\n" + stderr.String())
		if token != "" {
			return token, nil
		}
		return "", fmt.Errorf("glab auth status: %w (%s)", err, strings.TrimSpace(stderr.String()))
	}
	return parseGlabToken(stdout.String() + "\n" + stderr.String()), nil
}

// parseGlabToken finds a "Token: <value>" line in the combined output of
// `glab auth status -t` and returns the token (or "" if none).
func parseGlabToken(output string) string {
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		idx := strings.Index(strings.ToLower(line), "token:")
		if idx < 0 {
			continue
		}
		token := strings.TrimSpace(line[idx+len("token:"):])
		// glab sometimes prefixes lines with ANSI / arrows; strip leading
		// non-alphanumerics until we hit token characters.
		token = strings.TrimLeft(token, " \t-→>")
		if token != "" && token != "<no token>" {
			return token
		}
	}
	return ""
}

// glabVersion runs `glab --version` and extracts the semver string.
// Returns "" on any failure.
func glabVersion(ctx context.Context) string {
	cctx, cancel := context.WithTimeout(ctx, glabAuthTimeout)
	defer cancel()
	out, err := exec.CommandContext(cctx, "glab", "--version").Output()
	if err != nil {
		return ""
	}
	for _, field := range strings.Fields(string(out)) {
		if strings.Count(field, ".") >= 2 {
			return strings.TrimSpace(field)
		}
	}
	return ""
}

func stripScheme(host string) string {
	host = strings.TrimPrefix(host, "https://")
	host = strings.TrimPrefix(host, "http://")
	return strings.TrimRight(host, "/")
}
