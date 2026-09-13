package executor

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/kandev/kandev/internal/common/subproc"
	"github.com/kandev/kandev/internal/gitconfigenv"
	"github.com/kandev/kandev/internal/gitcredentials"
	"github.com/kandev/kandev/internal/githubauth"
	"github.com/kandev/kandev/internal/task/models"
)

const (
	hostGitHubCredentialProbeTimeout = 5 * time.Second
	hostGitHubCredentialProbeWorkers = 4
)

// hostGitHubCredentialProbe checks whether the host gh account can provide a
// token for one validated GitHub host. It must never return the token itself.
type hostGitHubCredentialProbe func(context.Context, string, string, map[string]string) error

type hostGitHubProbeResult struct {
	index     int
	available bool
}

// SetHostGitHubCredentialProbe replaces the host CLI availability check. The
// seam keeps policy tests deterministic while production uses the real gh CLI.
func (e *Executor) SetHostGitHubCredentialProbe(probe hostGitHubCredentialProbe) {
	if probe == nil {
		e.hostGitHubCredentialProbe = runHostGitHubCredentialProbe
		return
	}
	e.hostGitHubCredentialProbe = probe
}

func runHostGitHubCredentialProbe(ctx context.Context, executable, host string, env map[string]string) error {
	cmd := exec.CommandContext(ctx, executable, "auth", "token", "--hostname", host)
	cmd.Env = hostGitHubCommandEnvironment(env)
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	return subproc.RunGH(ctx, cmd)
}

func (e *Executor) configureHostGitHubCredentialBridgeWithProfileEnv(
	ctx context.Context,
	req *LaunchAgentRequest,
	infos []*repoInfo,
	profileEnvVars []models.ProfileEnvVar,
) error {
	if req == nil || !hostGitHubCredentialBridgeEligible(req.ExecutorType) {
		return nil
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	hosts := resolveHostGitHubHosts(infos)
	if len(hosts) == 0 {
		return nil
	}
	executable, err := resolveHostGitHubExecutable()
	if err != nil {
		return nil
	}
	probe := e.hostGitHubCredentialProbe
	if probe == nil {
		probe = runHostGitHubCredentialProbe
	}
	probeEnv, probeOK := e.hostGitHubProbeEnvironment(ctx, req.Env, profileEnvVars)
	if !probeOK {
		if err := ctx.Err(); err != nil {
			return err
		}
		return nil
	}
	available, err := e.probeHostGitHubHosts(ctx, req, profileEnvVars, probe, executable, hosts, probeEnv)
	if err != nil {
		return err
	}
	if len(available) == 0 {
		return nil
	}
	return appendHostGitHubCredentialHelpers(req, executable, available)
}

func (e *Executor) probeHostGitHubHosts(
	ctx context.Context,
	req *LaunchAgentRequest,
	profileEnvVars []models.ProfileEnvVar,
	probe hostGitHubCredentialProbe,
	executable string,
	hosts []string,
	probeEnv map[string]string,
) ([]string, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	candidates := hostGitHubProbeCandidates(req, profileEnvVars, hosts)
	if len(candidates) == 0 {
		return nil, nil
	}

	workerCount := hostGitHubCredentialProbeWorkers
	if workerCount > len(candidates) {
		workerCount = len(candidates)
	}
	probeCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	jobs := make(chan int, len(candidates))
	results := make(chan hostGitHubProbeResult, len(candidates))
	for _, index := range candidates {
		jobs <- index
	}
	close(jobs)

	var workers sync.WaitGroup
	workers.Add(workerCount)
	for range workerCount {
		go runHostGitHubProbeWorker(probeCtx, &workers, jobs, results, probe, executable, hosts, probeEnv)
	}

	availableByIndex := make([]bool, len(hosts))
	completed := 0
	for completed < len(candidates) {
		select {
		case result := <-results:
			availableByIndex[result.index] = result.available
			completed++
		case <-ctx.Done():
			cancel()
			workers.Wait()
			return nil, ctx.Err()
		}
	}
	cancel()
	workers.Wait()

	available := make([]string, 0, len(candidates))
	for _, index := range candidates {
		if availableByIndex[index] {
			available = append(available, hosts[index])
		}
	}
	return available, nil
}

func hostGitHubProbeCandidates(req *LaunchAgentRequest, profileEnvVars []models.ProfileEnvVar, hosts []string) []int {
	candidates := make([]int, 0, len(hosts))
	for index, host := range hosts {
		if hasExplicitGitHubToken(req.Env, host) || hasExplicitGitHubTokenInProfile(profileEnvVars, host) {
			continue
		}
		candidates = append(candidates, index)
	}
	return candidates
}

func runHostGitHubProbeWorker(
	ctx context.Context,
	workers *sync.WaitGroup,
	jobs <-chan int,
	results chan<- hostGitHubProbeResult,
	probe hostGitHubCredentialProbe,
	executable string,
	hosts []string,
	probeEnv map[string]string,
) {
	defer workers.Done()
	for index := range jobs {
		hostCtx, hostCancel := context.WithTimeout(ctx, hostGitHubCredentialProbeTimeout)
		probeErr := probe(hostCtx, executable, hosts[index], probeEnv)
		timedOut := hostCtx.Err() != nil
		hostCancel()
		select {
		case results <- hostGitHubProbeResult{index: index, available: probeErr == nil && !timedOut}:
		case <-ctx.Done():
			return
		}
	}
}

// resolveHostGitHubBridgeProfileEnv returns profile definitions in the order
// used by the final resolver: the agent profile is the weaker source and the
// executor profile is the authoritative source. A failed agent-profile lookup
// disables only the optional host bridge, because probing the backend account
// could select a different GitHub login from the one the child will use.
func (e *Executor) resolveHostGitHubBridgeProfileEnv(
	ctx context.Context,
	agentProfileID string,
	executorProfileEnv []models.ProfileEnvVar,
) ([]models.ProfileEnvVar, bool) {
	if strings.TrimSpace(agentProfileID) == "" {
		return append([]models.ProfileEnvVar(nil), executorProfileEnv...), true
	}
	if e == nil || e.agentManager == nil {
		return append([]models.ProfileEnvVar(nil), executorProfileEnv...), false
	}
	agentProfile, err := e.agentManager.ResolveAgentProfile(ctx, agentProfileID)
	if err != nil || agentProfile == nil {
		return append([]models.ProfileEnvVar(nil), executorProfileEnv...), false
	}
	profileEnv := make([]models.ProfileEnvVar, 0, len(agentProfile.EnvVars)+len(executorProfileEnv))
	profileEnv = append(profileEnv, agentProfile.EnvVars...)
	profileEnv = append(profileEnv, executorProfileEnv...)
	return profileEnv, true
}

func (e *Executor) hostGitHubProbeEnvironment(
	ctx context.Context,
	requestEnv map[string]string,
	profileEnvVars []models.ProfileEnvVar,
) (map[string]string, bool) {
	if len(profileEnvVars) == 0 {
		return requestEnv, true
	}
	selected := winningGitHubCredentialSelection(profileEnvVars)
	overrides, ok := e.resolveHostGitHubProbeOverrides(ctx, requestEnv, selected)
	if !ok {
		return nil, false
	}
	if len(overrides) == 0 {
		return requestEnv, true
	}
	probeEnv := cloneStringMap(requestEnv)
	if probeEnv == nil {
		probeEnv = make(map[string]string, len(overrides))
	}
	for key, value := range overrides {
		probeEnv[key] = value
	}
	return probeEnv, true
}

func winningGitHubCredentialSelection(profileEnvVars []models.ProfileEnvVar) map[string]models.ProfileEnvVar {
	selected := make(map[string]models.ProfileEnvVar, len(profileEnvVars))
	for _, envVar := range profileEnvVars {
		if !isGitHubCredentialSelectionEnv(envVar.Key) ||
			(envVar.SecretID == "" && strings.TrimSpace(envVar.Value) == "") {
			continue
		}
		// The effective profile is ordered from weaker agent values to stronger
		// executor values. Resolve the winning definition before revealing a
		// secret so a stale weaker reference cannot disable a valid override.
		selected[envVar.Key] = envVar
	}
	return selected
}

func (e *Executor) resolveHostGitHubProbeOverrides(
	ctx context.Context,
	requestEnv map[string]string,
	selected map[string]models.ProfileEnvVar,
) (map[string]string, bool) {
	overrides := make(map[string]string, len(selected))
	keys := make([]string, 0, len(selected))
	for key := range selected {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		envVar := selected[key]
		// Request values are the effective managed source. Do not reveal a
		// profile-backed directory that cannot affect the selected account.
		if nonEmptyEnvValue(requestEnv, envVar.Key) {
			continue
		}
		if envVar.SecretID == "" {
			if envVar.Value != "" {
				overrides[envVar.Key] = envVar.Value
			}
			continue
		}
		if e == nil || e.secretStore == nil {
			return nil, false
		}
		value, err := e.revealGlobalSecret(ctx, envVar.SecretID)
		if err != nil {
			return nil, false
		}
		overrides[envVar.Key] = value
	}
	return overrides, true
}

func isGitHubCredentialSelectionEnv(key string) bool {
	switch key {
	case "HOME", "GH_CONFIG_DIR", "XDG_CONFIG_HOME":
		return true
	default:
		return false
	}
}

func hasExplicitGitHubTokenInProfile(profileEnvVars []models.ProfileEnvVar, host string) bool {
	for _, envVar := range profileEnvVars {
		if envVar.SecretID == "" && strings.TrimSpace(envVar.Value) == "" {
			continue
		}
		switch envVar.Key {
		case envGHToken, envGitHubToken:
			if strings.EqualFold(host, defaultGitHubHost) {
				return true
			}
		case "GH_ENTERPRISE_TOKEN", "GITHUB_ENTERPRISE_TOKEN":
			if !strings.EqualFold(host, defaultGitHubHost) {
				return true
			}
		}
	}
	return false
}

func hostGitHubCredentialBridgeEligible(executorType string) bool {
	switch models.ExecutorType(executorType) {
	case models.ExecutorTypeLocal, models.ExecutorTypeWorktree:
		return true
	default:
		return false
	}
}

func resolveHostGitHubHosts(infos []*repoInfo) []string {
	hosts := make([]string, 0, len(infos))
	seen := make(map[string]struct{}, len(infos))
	for _, info := range infos {
		if info == nil || info.Repository == nil || !hostGitHubRepository(info.Repository) {
			continue
		}
		identity, err := gitcredentials.ResolveRepositoryIdentity(gitcredentials.RepositoryIdentityInput{
			RepositoryID:  info.RepositoryID,
			Provider:      info.Repository.Provider,
			ProviderHost:  info.Repository.ProviderHost,
			ProviderOwner: info.Repository.ProviderOwner,
			ProviderName:  info.Repository.ProviderName,
			RemoteURL:     info.Repository.RemoteURL,
		})
		if err != nil || !hostGitHubIdentityAllowed(info.Repository, identity.Host) {
			continue
		}
		host := strings.TrimSpace(identity.Host)
		key := strings.ToLower(host)
		if host == "" {
			continue
		}
		if _, found := seen[key]; found {
			continue
		}
		seen[key] = struct{}{}
		hosts = append(hosts, host)
	}
	return hosts
}

func hostGitHubRepository(repository *models.Repository) bool {
	if repository == nil {
		return false
	}
	provider := strings.ToLower(strings.TrimSpace(repository.Provider))
	if provider == gitHubProviderID {
		return true
	}
	// Empty provider rows retain the public GitHub compatibility fallback for
	// managed remote repositories, but a local checkout needs explicit metadata.
	return provider == "" && repository.SourceType != sourceTypeLocal
}

func hostGitHubIdentityAllowed(repository *models.Repository, host string) bool {
	if strings.EqualFold(strings.TrimSpace(repository.Provider), gitHubProviderID) {
		return true
	}
	return strings.EqualFold(host, defaultGitHubHost)
}

func hasExplicitGitHubToken(env map[string]string, host string) bool {
	if strings.EqualFold(host, defaultGitHubHost) {
		return nonEmptyEnvValue(env, envGHToken) || nonEmptyEnvValue(env, envGitHubToken)
	}
	return nonEmptyEnvValue(env, "GH_ENTERPRISE_TOKEN") ||
		nonEmptyEnvValue(env, "GITHUB_ENTERPRISE_TOKEN")
}

func nonEmptyEnvValue(env map[string]string, key string) bool {
	return strings.TrimSpace(env[key]) != ""
}

func resolveHostGitHubExecutable() (string, error) {
	path, err := exec.LookPath("gh")
	if err != nil {
		return "", err
	}
	if filepath.IsAbs(path) {
		return filepath.Clean(path), nil
	}
	return filepath.Abs(path)
}

func appendHostGitHubCredentialHelpers(req *LaunchAgentRequest, executable string, hosts []string) error {
	overlay := make(map[string]string, len(hosts)*2)
	helper := hostGitHubCredentialHelper(executable)
	for _, host := range hosts {
		appendGitConfig(overlay, "credential.https://"+host+".helper", helper)
	}
	merged, err := gitconfigenv.Merge(req.Env, overlay)
	if err != nil {
		return fmt.Errorf("compose host GitHub credential helper: %w", err)
	}
	req.Env = merged
	return nil
}

func removeHostGitHubCredentialHelpers(req *LaunchAgentRequest) error {
	if req == nil || req.Env == nil {
		return nil
	}
	filtered, err := gitconfigenv.Filter(req.Env, func(index int, entries []gitconfigenv.Entry) bool {
		return !isHostGitHubCredentialHelperEntry(entries[index])
	})
	if err != nil {
		return fmt.Errorf("remove host GitHub credential helper: %w", err)
	}
	req.Env = filtered
	return nil
}

func isHostGitHubCredentialHelperEntry(entry gitconfigenv.Entry) bool {
	return githubauth.IsHostGitHubCredentialHelperEntry(entry.Key, entry.Value)
}

func hostGitHubCredentialHelper(executable string) string {
	return "!f() { : " + githubauth.HostGitHubCredentialHelperMarker + "; " + shellQuote(executable) + " auth git-credential \"$@\"; }; f"
}

func isHostGitHubCredentialHelper(value string) bool {
	return githubauth.IsHostGitHubCredentialHelper(value)
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}

func hostGitHubCommandEnvironment(overrides map[string]string) []string {
	envMap := make(map[string]string, len(overrides)+4)
	for _, entry := range os.Environ() {
		key, value, found := strings.Cut(entry, "=")
		if found && isHostGitHubCommandEnvironmentKey(key) {
			envMap[key] = value
		}
	}
	for key, value := range overrides {
		if isHostGitHubCommandEnvironmentKey(key) {
			envMap[key] = value
		}
	}
	keys := make([]string, 0, len(envMap))
	for key := range envMap {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	env := make([]string, 0, len(keys))
	for _, key := range keys {
		env = append(env, key+"="+envMap[key])
	}
	return env
}

func isHostGitHubCommandEnvironmentKey(key string) bool {
	switch key {
	case "PATH", "HOME", "GH_CONFIG_DIR", "XDG_CONFIG_HOME":
		return true
	default:
		return false
	}
}
