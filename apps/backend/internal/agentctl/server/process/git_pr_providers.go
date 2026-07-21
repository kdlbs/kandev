package process

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/subproc"
)

var credentialURLPattern = regexp.MustCompile(`(?i)(https?://)[^\s/@]+@`)

type prProvider string

const (
	prProviderGitHub               prProvider = "github"
	prProviderGitLab               prProvider = "gitlab"
	prProviderAzureRepos           prProvider = "azure_repos"
	prCreateSubcommand                        = "create"
	repositoryFlagTitle                       = "--title"
	repositoryFlagBody                        = "--body"
	repositoryFlagDescription                 = "--description"
	repositoryFlagHead                        = "--head"
	redactedLogValue                          = "[REDACTED]"
	azureDevOpsExtensionName                  = "azure-devops"
	errAzureCLIMissing                        = "azure CLI (az) is not on PATH; install it and run: az extension add --name azure-devops"
	errAzureDevOpsExtensionMissing            = "azure DevOps CLI extension is not installed; run: az extension add --name azure-devops"
	gitLabHostEnv                             = "KANDEV_GITLAB_HOST"
	gitLabTokenEnv                            = "GITLAB_TOKEN"
)

const gitLabAPITimeout = 30 * time.Second

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
	return detectPRProviderWithGitLabHost(remoteURL, os.Getenv(gitLabHostEnv))
}

func detectPRProviderWithGitLabHost(remoteURL, configuredGitLabHost string) prProvider {
	host := remoteHostFromURL(remoteURL)
	if isAzureReposHost(host) {
		return prProviderAzureRepos
	}
	if isGitHubHost(host) {
		return prProviderGitHub
	}
	if isGitLabHost(host, configuredGitLabHost) {
		return prProviderGitLab
	}
	return ""
}

func isGitLabHost(host, configuredGitLabHost string) bool {
	if host == "gitlab.com" {
		return true
	}
	configured, err := url.Parse(strings.TrimSpace(configuredGitLabHost))
	return err == nil && configured.Hostname() != "" && strings.EqualFold(host, configured.Hostname())
}

func (g *GitOperator) detectPRProvider(remoteURL string) prProvider {
	return detectPRProviderWithGitLabHost(remoteURL, g.environmentValue(gitLabHostEnv))
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

func sanitizePRFailure(message string, sensitiveValues ...string) string {
	sanitized := credentialURLPattern.ReplaceAllString(message, `${1}`+redactedLogValue+"@")
	values := append([]string{os.Getenv(gitLabTokenEnv)}, sensitiveValues...)
	for _, value := range values {
		if value != "" {
			sanitized = strings.ReplaceAll(sanitized, value, redactedLogValue)
		}
	}
	return sanitized
}

func (g *GitOperator) sanitizePRFailure(message string, sensitiveValues ...string) string {
	values := append([]string{g.environmentValue(gitLabTokenEnv)}, sensitiveValues...)
	return sanitizePRFailure(message, values...)
}

func (g *GitOperator) sanitizeGitPushOutput(output string) string {
	return g.sanitizePRFailure(output)
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
	cmd.Env = filterGitEnv(g.environmentValues())

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	g.logger.Debug("executing repository command",
		zap.String("command", name),
		zap.Strings("args", sanitizeRepositoryArgs(args)),
		zap.String("workDir", g.workDir))

	// Route through the matching subproc throttle so PR-creation execs
	// (gh / git) count against the same process-wide cap as the rest of
	// the codebase. Unknown binaries (e.g. az) skip throttling — those
	// aren't part of the fork-storm pattern.
	var err error
	switch name {
	case "gh":
		err = subproc.RunGH(ctx, cmd)
	case "git":
		err = subproc.RunGit(ctx, cmd)
	default:
		err = cmd.Run()
	}
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

func (g *GitOperator) createGitLabPR(
	ctx context.Context,
	result *PRCreateResult,
	info *gitLabRepoInfo,
	branch, title, body, baseBranch string,
	draft bool,
) (*PRCreateResult, error) {
	targetBranch, err := g.resolveGitLabTargetBranch(ctx, info, baseBranch)
	if err != nil {
		result.Error = err.Error()
		return result, nil
	}

	if _, lookupErr := exec.LookPath("glab"); lookupErr != nil {
		return g.createGitLabPRWithREST(ctx, result, info, branch, title, body, targetBranch, draft)
	}
	if existingURL := g.findExistingGitLabMRWithGLab(ctx, info, branch, targetBranch); existingURL != "" {
		result.PRURL = existingURL
		result.Output = existingURL
		result.Success = true
		return result, nil
	}

	args := []string{"mr", "create", repositoryFlagTitle, title, repositoryFlagDescription, body, "--source-branch", branch}
	args = append(args, "--target-branch", targetBranch)
	if draft {
		args = append(args, "--draft")
	}
	args = append(args, "--yes")

	stdoutOutput, stderrOutput, err := g.runRepositoryCommand(ctx, "glab", args...)
	result.Output = g.sanitizePRFailure(combineCommandOutput(stdoutOutput, stderrOutput), title, body)
	if err != nil {
		if g.environmentValue(gitLabTokenEnv) != "" {
			return g.createGitLabPRWithREST(ctx, result, info, branch, title, body, targetBranch, draft)
		}
		result.Error = "GitLab merge request creation failed; authenticate glab for the configured host"
		return result, nil
	}
	result.PRURL = extractGitLabMRURL(stdoutOutput, info)
	result.Success = result.PRURL != ""
	if !result.Success {
		result.Error = "GitLab did not return a merge request URL"
	}
	return result, nil
}

type gitLabRepoInfo struct {
	Origin      string
	APIHost     string
	ProjectPath string
}

type gitLabRemoteInfo struct {
	Hostname    string
	HTTPOrigin  string
	ProjectPath string
	SSH         bool
}

func parseGitLabRepoInfo(remoteURL, configuredOrigin string) (*gitLabRepoInfo, error) {
	remote, err := parseGitLabRemote(remoteURL)
	if err != nil {
		return nil, err
	}
	origin := strings.TrimSpace(configuredOrigin)
	if origin == "" {
		if !strings.EqualFold(remote.Hostname, "gitlab.com") {
			return nil, errors.New("GitLab host is not configured for this remote")
		}
		origin = "https://gitlab.com"
	}
	parsedOrigin, err := url.Parse(origin)
	if err != nil || (parsedOrigin.Scheme != "http" && parsedOrigin.Scheme != "https") || parsedOrigin.Host == "" || parsedOrigin.User != nil || (parsedOrigin.Path != "" && parsedOrigin.Path != "/") {
		return nil, errors.New("configured GitLab host must be an HTTP(S) origin")
	}
	normalizedOrigin := parsedOrigin.Scheme + "://" + parsedOrigin.Host
	if remote.SSH {
		if !strings.EqualFold(parsedOrigin.Hostname(), remote.Hostname) {
			return nil, errors.New("GitLab remote host does not match the configured workspace host")
		}
	} else if !strings.EqualFold(normalizedOrigin, remote.HTTPOrigin) {
		return nil, errors.New("GitLab remote host does not match the configured workspace host")
	}
	return &gitLabRepoInfo{
		Origin:      normalizedOrigin,
		APIHost:     parsedOrigin.Host,
		ProjectPath: remote.ProjectPath,
	}, nil
}

func parseGitLabRemote(remoteURL string) (*gitLabRemoteInfo, error) {
	trimmed := strings.TrimSpace(remoteURL)
	if strings.Contains(trimmed, "://") {
		parsed, err := url.Parse(trimmed)
		if err != nil || parsed.Hostname() == "" || parsed.RawQuery != "" || parsed.Fragment != "" {
			return nil, errors.New("invalid GitLab remote URL")
		}
		host, projectPath, err := validateGitLabRemoteParts(parsed.Hostname(), parsed.Path)
		if err != nil {
			return nil, err
		}
		switch parsed.Scheme {
		case "http", "https":
			if parsed.User != nil {
				return nil, errors.New("GitLab HTTP remote URL must not contain credentials")
			}
			return &gitLabRemoteInfo{Hostname: host, HTTPOrigin: parsed.Scheme + "://" + parsed.Host, ProjectPath: projectPath}, nil
		case "ssh":
			return &gitLabRemoteInfo{Hostname: host, ProjectPath: projectPath, SSH: true}, nil
		default:
			return nil, errors.New("unsupported GitLab remote URL scheme")
		}
	}
	withoutUser := trimmed
	if _, after, ok := strings.Cut(withoutUser, "@"); ok {
		withoutUser = after
	}
	host, path, ok := strings.Cut(withoutUser, ":")
	if !ok {
		return nil, errors.New("invalid GitLab SSH remote URL")
	}
	host, projectPath, err := validateGitLabRemoteParts(host, path)
	if err != nil {
		return nil, err
	}
	return &gitLabRemoteInfo{Hostname: host, ProjectPath: projectPath, SSH: true}, nil
}

func validateGitLabRemoteParts(host, path string) (string, string, error) {
	projectPath := strings.TrimSuffix(strings.Trim(strings.TrimSpace(path), "/"), ".git")
	if host == "" || projectPath == "" || strings.Contains(projectPath, "..") || strings.Contains(projectPath, "/-/") {
		return "", "", errors.New("invalid GitLab repository path")
	}
	return strings.ToLower(host), projectPath, nil
}

type gitLabMRResponse struct {
	WebURL       string `json:"web_url"`
	TargetBranch string `json:"target_branch"`
}

func (g *GitOperator) findExistingGitLabMRWithGLab(ctx context.Context, info *gitLabRepoInfo, branch, targetBranch string) string {
	args := []string{"mr", "list", "--source-branch", branch, "--target-branch", targetBranch, "--state", "opened", "--output", "json"}
	stdoutOutput, _, err := g.runRepositoryCommand(ctx, "glab", args...)
	if err != nil {
		return ""
	}
	var rows []gitLabMRResponse
	if json.Unmarshal([]byte(stdoutOutput), &rows) != nil {
		return ""
	}
	for _, row := range rows {
		if row.TargetBranch == targetBranch {
			validated, validationErr := validateGitLabMRWebURL(row.WebURL, info)
			if validationErr == nil {
				return validated
			}
		}
	}
	return ""
}

func extractGitLabMRURL(output string, info *gitLabRepoInfo) string {
	for _, field := range strings.Fields(output) {
		candidate := strings.Trim(field, "()[]{}<>,.;\"'")
		if validated, err := validateGitLabMRWebURL(candidate, info); err == nil {
			return validated
		}
	}
	return ""
}

func validateGitLabMRWebURL(raw string, info *gitLabRepoInfo) (string, error) {
	candidate := strings.TrimSpace(raw)
	parsed, err := url.Parse(candidate)
	if err != nil || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", errors.New("invalid GitLab merge request URL")
	}
	if !strings.EqualFold(parsed.Scheme+"://"+parsed.Host, info.Origin) {
		return "", errors.New("GitLab merge request URL does not match the configured workspace origin")
	}
	prefix := "/" + strings.Trim(info.ProjectPath, "/") + "/-/merge_requests/"
	iid := strings.TrimPrefix(parsed.Path, prefix)
	if !strings.HasPrefix(parsed.Path, prefix) || iid == "" || strings.Contains(iid, "/") {
		return "", errors.New("GitLab merge request URL does not match the repository")
	}
	if _, err := strconv.Atoi(iid); err != nil {
		return "", errors.New("GitLab merge request URL has an invalid IID")
	}
	return candidate, nil
}

func (g *GitOperator) resolveGitLabTargetBranch(ctx context.Context, info *gitLabRepoInfo, baseBranch string) (string, error) {
	if target := cleanBaseBranch(baseBranch); target != "" {
		return target, nil
	}
	projectPath := "projects/" + url.PathEscape(info.ProjectPath)
	if _, err := exec.LookPath("glab"); err == nil {
		stdout, _, runErr := g.runRepositoryCommand(ctx, "glab", "api", projectPath, "--hostname", info.APIHost)
		if runErr == nil {
			var project struct {
				DefaultBranch string `json:"default_branch"`
			}
			if json.Unmarshal([]byte(stdout), &project) == nil && strings.TrimSpace(project.DefaultBranch) != "" {
				return strings.TrimSpace(project.DefaultBranch), nil
			}
		}
	}
	token := strings.TrimSpace(g.environmentValue(gitLabTokenEnv))
	if token == "" {
		return "", errors.New("GitLab project default branch is unavailable; authenticate glab for the configured host")
	}
	var project struct {
		DefaultBranch string `json:"default_branch"`
	}
	endpoint := info.Origin + "/api/v4/" + projectPath
	if err := gitLabAPIJSON(ctx, &http.Client{Timeout: gitLabAPITimeout}, token, http.MethodGet, endpoint, nil, &project); err != nil {
		return "", err
	}
	if strings.TrimSpace(project.DefaultBranch) == "" {
		return "", errors.New("GitLab project did not return a default branch")
	}
	return strings.TrimSpace(project.DefaultBranch), nil
}

func (g *GitOperator) createGitLabPRWithREST(
	ctx context.Context,
	result *PRCreateResult,
	info *gitLabRepoInfo,
	branch, title, body, targetBranch string,
	draft bool,
) (*PRCreateResult, error) {
	token := strings.TrimSpace(g.environmentValue(gitLabTokenEnv))
	if token == "" {
		result.Error = "GitLab authentication is unavailable; install and authenticate glab or configure a workspace token"
		return result, nil
	}
	client := &http.Client{Timeout: gitLabAPITimeout}
	projectEndpoint := info.Origin + "/api/v4/projects/" + url.PathEscape(info.ProjectPath)
	existingEndpoint := projectEndpoint + "/merge_requests?state=opened&source_branch=" + url.QueryEscape(branch) + "&target_branch=" + url.QueryEscape(targetBranch)
	var existing []gitLabMRResponse
	if err := gitLabAPIJSON(ctx, client, token, http.MethodGet, existingEndpoint, nil, &existing); err != nil {
		result.Error = err.Error()
		return result, nil
	}
	if len(existing) > 0 && existing[0].WebURL != "" {
		validated, validationErr := validateGitLabMRWebURL(existing[0].WebURL, info)
		if validationErr != nil {
			result.Error = validationErr.Error()
			return result, nil
		}
		result.PRURL = validated
		result.Output = validated
		result.Success = true
		return result, nil
	}

	if draft && !strings.HasPrefix(strings.ToLower(strings.TrimSpace(title)), "draft:") {
		title = "Draft: " + title
	}
	payload := struct {
		SourceBranch string `json:"source_branch"`
		TargetBranch string `json:"target_branch"`
		Title        string `json:"title"`
		Description  string `json:"description"`
	}{SourceBranch: branch, TargetBranch: targetBranch, Title: title, Description: body}
	var created gitLabMRResponse
	if err := gitLabAPIJSON(ctx, client, token, http.MethodPost, projectEndpoint+"/merge_requests", payload, &created); err != nil {
		result.Error = err.Error()
		return result, nil
	}
	if created.WebURL == "" {
		result.Error = "GitLab did not return a merge request URL"
		return result, nil
	}
	validated, validationErr := validateGitLabMRWebURL(created.WebURL, info)
	if validationErr != nil {
		result.Error = validationErr.Error()
		return result, nil
	}
	result.PRURL = validated
	result.Output = validated
	result.Success = true
	return result, nil
}

func gitLabAPIJSON(ctx context.Context, client *http.Client, token, method, endpoint string, requestBody, responseBody any) error {
	var body io.Reader
	if requestBody != nil {
		encoded, err := json.Marshal(requestBody)
		if err != nil {
			return errors.New("encode GitLab API request")
		}
		body = bytes.NewReader(encoded)
	}
	req, err := http.NewRequestWithContext(ctx, method, endpoint, body)
	if err != nil {
		return errors.New("build GitLab API request")
	}
	req.Header.Set("PRIVATE-TOKEN", token)
	if requestBody != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("GitLab API request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<20))
		return fmt.Errorf("GitLab API request failed with status %d", resp.StatusCode)
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(responseBody); err != nil {
		return errors.New("decode GitLab API response")
	}
	return nil
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
