package process

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"go.uber.org/zap"
)

type prProvider string

const (
	prProviderGitHub               prProvider = "github"
	prProviderAzureRepos           prProvider = "azure_repos"
	prCreateSubcommand                        = "create"
	repositoryFlagTitle                       = "--title"
	repositoryFlagBody                        = "--body"
	repositoryFlagDescription                 = "--description"
	repositoryFlagHead                        = "--head"
	redactedLogValue                          = "[REDACTED]"
	azureDevOpsExtensionName                  = "azure-devops"
	errAzureCLIMissing                        = "Azure CLI (az) is not on PATH; install it and run: az extension add --name azure-devops"
	errAzureDevOpsExtensionMissing            = "Azure DevOps CLI extension is not installed; run: az extension add --name azure-devops"
)

type azureRepoInfo struct {
	OrganizationURL string
	Project         string
	Repository      string
}

type azurePRCreateResponse struct {
	PullRequestID int `json:"pullRequestId"`
	Repository    struct {
		RemoteURL string `json:"remoteUrl"`
	} `json:"repository"`
}

func detectPRProvider(remoteURL string) prProvider {
	host := remoteHostFromURL(remoteURL)
	if isAzureReposHost(host) {
		return prProviderAzureRepos
	}
	if isGitHubHost(host) {
		return prProviderGitHub
	}
	return ""
}

func isGitHubHost(host string) bool {
	return host == "github.com" || strings.HasSuffix(host, ".github.com")
}

func isAzureReposHost(host string) bool {
	switch host {
	case "dev.azure.com", "ssh.dev.azure.com":
		return true
	default:
		return strings.HasSuffix(host, ".visualstudio.com")
	}
}

// remoteHostFromURL returns the lowercase hostname from an origin remote URL.
func remoteHostFromURL(remoteURL string) string {
	trimmed := strings.TrimSpace(remoteURL)
	if trimmed == "" {
		return ""
	}

	if strings.Contains(trimmed, "://") {
		parsed, err := url.Parse(trimmed)
		if err == nil && parsed.Host != "" {
			host := parsed.Hostname()
			return strings.ToLower(host)
		}
	}

	rest := strings.TrimPrefix(trimmed, "ssh://")
	if _, after, ok := strings.Cut(rest, "@"); ok {
		rest = after
	}
	hostPort, _, ok := strings.Cut(rest, ":")
	if !ok {
		hostPort, _, _ = strings.Cut(rest, "/")
	}
	host := hostPort
	if idx := strings.Index(host, ":"); idx >= 0 {
		host = host[:idx]
	}
	return strings.ToLower(host)
}

func parseAzurePRCreateResponse(stdoutOutput string) (azurePRCreateResponse, error) {
	trimmed := strings.TrimSpace(stdoutOutput)
	var response azurePRCreateResponse
	if err := json.Unmarshal([]byte(trimmed), &response); err == nil {
		return response, nil
	}

	// az may prefix stdout with status text; decode the first JSON value (incl. pretty-printed).
	if start := strings.Index(trimmed, "{"); start >= 0 {
		dec := json.NewDecoder(strings.NewReader(trimmed[start:]))
		if err := dec.Decode(&response); err == nil {
			return response, nil
		}
	}

	// Some az versions emit single-line JSON after status lines.
	lines := strings.Split(trimmed, "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if !strings.HasPrefix(line, "{") {
			continue
		}
		if err := json.Unmarshal([]byte(line), &response); err == nil {
			return response, nil
		}
	}
	return azurePRCreateResponse{}, fmt.Errorf("no JSON object in output")
}

func parseAzureRepoInfo(remoteURL string) (*azureRepoInfo, error) {
	trimmed := strings.TrimSpace(remoteURL)
	switch {
	case strings.HasPrefix(trimmed, "http://") || strings.HasPrefix(trimmed, "https://"):
		return parseAzureHTTPRemote(trimmed)
	case strings.HasPrefix(trimmed, "ssh://"):
		return parseAzureSSHRemote(trimmed)
	case strings.Contains(trimmed, "@") && strings.Contains(trimmed, ":"):
		return parseAzureSCPRemote(trimmed)
	default:
		return nil, fmt.Errorf("unsupported Azure Repos remote URL: %s", remoteURL)
	}
}

func parseAzureHTTPRemote(remoteURL string) (*azureRepoInfo, error) {
	trimmed := strings.TrimSpace(remoteURL)
	withoutScheme := strings.TrimPrefix(trimmed, "https://")
	withoutScheme = strings.TrimPrefix(withoutScheme, "http://")

	hostAndPath := withoutScheme
	if _, after, ok := strings.Cut(hostAndPath, "@"); ok {
		hostAndPath = after
	}

	host, path, ok := strings.Cut(hostAndPath, "/")
	if !ok {
		return nil, fmt.Errorf("missing repository path in Azure Repos URL: %s", remoteURL)
	}

	segments := splitRemotePath(path)
	if len(segments) < 3 {
		return nil, fmt.Errorf("invalid Azure Repos URL path: %s", remoteURL)
	}

	lowerHost := strings.ToLower(host)
	scheme := "https://"
	switch {
	case strings.Contains(lowerHost, "dev.azure.com"):
		if len(segments) < 4 || segments[2] != "_git" {
			return nil, fmt.Errorf("invalid Azure Repos dev.azure.com URL: %s", remoteURL)
		}
		return &azureRepoInfo{
			OrganizationURL: scheme + host + "/" + segments[0],
			Project:         segments[1],
			Repository:      trimGitSuffix(segments[3]),
		}, nil
	case strings.Contains(lowerHost, "visualstudio.com"):
		if len(segments) < 3 || segments[1] != "_git" {
			return nil, fmt.Errorf("invalid Azure Repos visualstudio.com URL: %s", remoteURL)
		}
		return &azureRepoInfo{
			OrganizationURL: scheme + host,
			Project:         segments[0],
			Repository:      trimGitSuffix(segments[2]),
		}, nil
	default:
		return nil, fmt.Errorf("unsupported Azure Repos host: %s", host)
	}
}

func parseAzureSSHRemote(remoteURL string) (*azureRepoInfo, error) {
	trimmed := strings.TrimPrefix(strings.TrimSpace(remoteURL), "ssh://")
	if _, after, ok := strings.Cut(trimmed, "@"); ok {
		trimmed = after
	}
	hostPort, path, ok := strings.Cut(trimmed, "/")
	if !ok {
		return nil, fmt.Errorf("missing repository path in Azure Repos SSH URL: %s", remoteURL)
	}
	return parseAzureSSHParts(hostPort, path, remoteURL)
}

func parseAzureSCPRemote(remoteURL string) (*azureRepoInfo, error) {
	trimmed := strings.TrimSpace(remoteURL)
	if _, after, ok := strings.Cut(trimmed, "@"); ok {
		trimmed = after
	}
	hostPort, path, ok := strings.Cut(trimmed, ":")
	if !ok {
		return nil, fmt.Errorf("missing repository path in Azure Repos SCP URL: %s", remoteURL)
	}
	return parseAzureSSHParts(hostPort, path, remoteURL)
}

func parseAzureSSHParts(hostPort, path, rawURL string) (*azureRepoInfo, error) {
	host := hostPort
	if idx := strings.Index(host, ":"); idx >= 0 {
		host = host[:idx]
	}
	segments := splitRemotePath(path)
	if len(segments) < 4 || segments[0] != "v3" {
		return nil, fmt.Errorf("invalid Azure Repos SSH path: %s", rawURL)
	}

	org := segments[1]
	project := segments[2]
	repo := trimGitSuffix(segments[3])
	lowerHost := strings.ToLower(host)

	switch {
	case strings.Contains(lowerHost, "ssh.dev.azure.com"):
		return &azureRepoInfo{
			OrganizationURL: "https://dev.azure.com/" + org,
			Project:         project,
			Repository:      repo,
		}, nil
	case strings.Contains(lowerHost, "visualstudio.com"):
		return &azureRepoInfo{
			OrganizationURL: "https://" + org + ".visualstudio.com",
			Project:         project,
			Repository:      repo,
		}, nil
	default:
		return nil, fmt.Errorf("unsupported Azure Repos SSH host: %s", host)
	}
}

func splitRemotePath(path string) []string {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	segments := make([]string, 0, len(parts))
	for _, part := range parts {
		if part == "" {
			continue
		}
		segments = append(segments, part)
	}
	return segments
}

func trimGitSuffix(name string) string {
	return strings.TrimSuffix(name, ".git")
}

func cleanBaseBranch(baseBranch string) string {
	return strings.TrimPrefix(strings.TrimSpace(baseBranch), "origin/")
}

func sanitizeRepositoryArgs(args []string) []string {
	redactedFlags := map[string]struct{}{
		repositoryFlagTitle:       {},
		repositoryFlagBody:        {},
		repositoryFlagDescription: {},
	}

	sanitized := make([]string, 0, len(args))
	redactNext := false
	for _, arg := range args {
		if redactNext {
			sanitized = append(sanitized, redactedLogValue)
			redactNext = false
			continue
		}

		if _, ok := redactedFlags[arg]; ok {
			sanitized = append(sanitized, arg)
			redactNext = true
			continue
		}

		switch {
		case strings.HasPrefix(arg, repositoryFlagTitle+"="):
			sanitized = append(sanitized, repositoryFlagTitle+"="+redactedLogValue)
		case strings.HasPrefix(arg, repositoryFlagBody+"="):
			sanitized = append(sanitized, repositoryFlagBody+"="+redactedLogValue)
		case strings.HasPrefix(arg, repositoryFlagDescription+"="):
			sanitized = append(sanitized, repositoryFlagDescription+"="+redactedLogValue)
		default:
			sanitized = append(sanitized, arg)
		}
	}

	return sanitized
}

func combineCommandOutput(stdout, stderr string) string {
	parts := make([]string, 0, 2)
	if trimmedStdout := strings.TrimSpace(stdout); trimmedStdout != "" {
		parts = append(parts, trimmedStdout)
	}
	if trimmedStderr := strings.TrimSpace(stderr); trimmedStderr != "" {
		parts = append(parts, trimmedStderr)
	}
	return strings.Join(parts, "\n")
}

func redactRemoteURL(remoteURL string) string {
	trimmed := strings.TrimSpace(remoteURL)
	if trimmed == "" {
		return ""
	}

	if strings.Contains(trimmed, "://") {
		parsed, err := url.Parse(trimmed)
		if err == nil {
			parsed.User = nil
			return parsed.String()
		}
	}

	if before, after, ok := strings.Cut(trimmed, "@"); ok && before != "" && strings.Contains(after, ":") {
		return after
	}

	return trimmed
}

func (g *GitOperator) getOriginRemoteURL(ctx context.Context) (string, error) {
	output, err := g.runGitCommand(ctx, "remote", "get-url", "origin")
	if err != nil {
		return "", fmt.Errorf("failed to get origin remote URL: %w", err)
	}
	return strings.TrimSpace(output), nil
}

func (g *GitOperator) runRepositoryCommand(ctx context.Context, name string, args ...string) (string, string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = g.workDir
	cmd.Env = filterGitEnv(os.Environ())

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	g.logger.Debug("executing repository command",
		zap.String("command", name),
		zap.Strings("args", sanitizeRepositoryArgs(args)),
		zap.String("workDir", g.workDir))

	err := cmd.Run()
	stdoutOutput := strings.TrimSpace(stdout.String())
	stderrOutput := strings.TrimSpace(stderr.String())
	if err != nil {
		combined := combineCommandOutput(stdoutOutput, stderrOutput)
		if combined == "" {
			combined = err.Error()
		}
		return stdoutOutput, stderrOutput, fmt.Errorf("%w: %s", err, combined)
	}
	return stdoutOutput, stderrOutput, nil
}

func (g *GitOperator) createGitHubPR(
	ctx context.Context,
	result *PRCreateResult,
	branch, title, body, baseBranch string,
	draft bool,
) (*PRCreateResult, error) {
	args := []string{"pr", prCreateSubcommand, repositoryFlagTitle, title, repositoryFlagBody, body, repositoryFlagHead, branch}
	if cleanBase := cleanBaseBranch(baseBranch); cleanBase != "" {
		args = append(args, "--base", cleanBase)
	}
	if draft {
		args = append(args, "--draft")
	}

	stdoutOutput, stderrOutput, err := g.runRepositoryCommand(ctx, "gh", args...)
	result.Output = combineCommandOutput(stdoutOutput, stderrOutput)
	if err != nil {
		result.Error = err.Error()
		return result, nil
	}

	result.PRURL = strings.TrimSpace(stdoutOutput)
	result.Success = true
	g.logger.Info("PR created", zap.String("url", result.PRURL))
	return result, nil
}

func ensureAzureDevOpsCLI(ctx context.Context) error {
	if _, err := exec.LookPath("az"); err != nil {
		return errors.New(errAzureCLIMissing)
	}

	cmd := exec.CommandContext(ctx, "az", "extension", "show", "--name", azureDevOpsExtensionName)
	cmd.Env = filterGitEnv(os.Environ())
	if err := cmd.Run(); err != nil {
		return errors.New(errAzureDevOpsExtensionMissing)
	}
	return nil
}

func (g *GitOperator) createAzureReposPR(
	ctx context.Context,
	result *PRCreateResult,
	remoteURL, branch, title, body, baseBranch string,
	draft bool,
) (*PRCreateResult, error) {
	if err := ensureAzureDevOpsCLI(ctx); err != nil {
		result.Error = err.Error()
		return result, nil
	}

	info, err := parseAzureRepoInfo(remoteURL)
	if err != nil {
		result.Error = err.Error()
		return result, nil
	}

	args := []string{
		"repos", "pr", prCreateSubcommand,
		"--organization", info.OrganizationURL,
		"--project", info.Project,
		"--repository", info.Repository,
		"--source-branch", branch,
	}
	if cleanBase := cleanBaseBranch(baseBranch); cleanBase != "" {
		args = append(args, "--target-branch", cleanBase)
	}
	args = append(args,
		repositoryFlagTitle, title,
		repositoryFlagDescription, body,
	)
	if draft {
		args = append(args, "--draft", "true")
	}
	args = append(args, "-o", "json")

	stdoutOutput, stderrOutput, err := g.runRepositoryCommand(ctx, "az", args...)
	result.Output = combineCommandOutput(stdoutOutput, stderrOutput)
	if err != nil {
		result.Error = err.Error()
		return result, nil
	}

	response, err := parseAzurePRCreateResponse(stdoutOutput)
	if err != nil {
		result.Error = fmt.Sprintf("failed to parse Azure Repos PR output: %v", err)
		return result, nil
	}
	if response.PullRequestID <= 0 || strings.TrimSpace(response.Repository.RemoteURL) == "" {
		result.Error = "Azure Repos PR output did not include a pull request URL"
		return result, nil
	}

	result.PRURL = strings.TrimRight(response.Repository.RemoteURL, "/") + "/pullrequest/" + strconv.Itoa(response.PullRequestID)
	result.Success = true
	g.logger.Info("PR created", zap.String("url", result.PRURL))
	return result, nil
}
