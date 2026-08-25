package gitlab

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"regexp"
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
// streams and parses the token status line.
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

// glabTokenLinePattern matches a token status line by structure, rather than
// by exact wording. The token label must start the line after any status
// marker, which prevents diagnostic text from becoming a credential.
var glabTokenLinePattern = regexp.MustCompile(`(?i)^[^[:alnum:]_]*token\b([^:\n]*):\s*(\S.*)$`)

func isGlabTokenMetadataLabel(label string) bool {
	fields := strings.Fields(strings.ToLower(label))
	if len(fields) == 0 {
		return false
	}
	switch strings.Trim(fields[0], "():") {
	case "scope", "scopes", "type", "endpoint", "permission", "permissions":
		return true
	default:
		return false
	}
}

// parseGlabToken finds the token line in the combined output of
// `glab auth status -t` and returns the token (or "" if none).
func parseGlabToken(output string) string {
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		m := glabTokenLinePattern.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		if isGlabTokenMetadataLabel(m[1]) {
			continue
		}
		token := strings.TrimSpace(m[2])
		// glab sometimes prefixes lines with ANSI / arrows; strip leading
		// non-alphanumerics until we hit token characters.
		token = strings.TrimLeft(token, " \t-→>")
		// Without -t (or a masked display), the value is a run of asterisks
		// rather than an absent line — reject that instead of "authenticating"
		// with a redacted placeholder.
		if token == "" || token == "<no token>" || strings.Trim(token, "*") == "" {
			return ""
		}
		return token
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
