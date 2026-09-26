package lifecycle

import (
	"fmt"
	"strings"

	"github.com/kandev/kandev/internal/gitconfigenv"
	"github.com/kandev/kandev/internal/githubauth"
	"github.com/kandev/kandev/internal/task/models"
)

const dockerCloneLine = "git clone --depth=1 --branch {{repository.branch}} {{repository.clone_url}} {{workspace.path}}"

func checkoutOptionsPrepareScript(script string, options *models.RepositoryCheckoutOptions) (string, error) {
	options, err := models.NormalizeRepositoryCheckoutOptions(options)
	if err != nil {
		return "", err
	}
	if options == nil || (options.DownloadMode == models.DownloadStandard && len(options.SparseDirectories) == 0) {
		return script, nil
	}
	if script != DefaultPrepareScript("local_docker") {
		return "", fmt.Errorf("checkout options require the built-in preparation script")
	}
	clone := "git clone --no-checkout --branch {{repository.branch}}"
	if options.DownloadMode == models.DownloadOnDemand {
		clone += " --filter=blob:none"
	}
	clone += " -- {{repository.clone_url}} {{workspace.path}}"
	lines := []string{
		`checkout_log=$(mktemp)`,
		`trap 'rm -f "$checkout_log"' EXIT`,
		"if ! " + clone + ` 2>"$checkout_log"; then echo 'Repository clone failed. Check remote access and retry.' >&2; exit 1; fi`,
		`if grep -qi 'filtering not recognized' "$checkout_log"; then echo 'Server does not support on-demand downloads' >&2; exit 1; fi`,
		"cd {{workspace.path}}",
	}
	if len(options.SparseDirectories) > 0 {
		quoted := make([]string, 0, len(options.SparseDirectories))
		for _, directory := range options.SparseDirectories {
			quoted = append(quoted, shellQuote(`"`+strings.ReplaceAll(directory, `"`, `\"`)+`"`))
		}
		lines = append(lines, `printf '%s\n' `+strings.Join(quoted, " ")+` | git sparse-checkout set --cone --stdin`)
	}
	lines = append(lines, "git read-tree -mu HEAD")
	return strings.Replace(script, dockerCloneLine, strings.Join(lines, "\n"), 1), nil
}

func primaryCheckoutOptions(metadata map[string]interface{}) (*models.RepositoryCheckoutOptions, error) {
	return models.GetRepositoryCheckoutOptions(metadata)
}

func (m *Manager) validateLaunchCheckoutOptions(req *LaunchRequest) error {
	for _, spec := range req.RepoSpecs() {
		options, err := models.NormalizeRepositoryCheckoutOptions(spec.CheckoutOptions)
		if err != nil {
			return err
		}
		if options == nil || (options.DownloadMode == models.DownloadStandard && len(options.SparseDirectories) == 0) {
			continue
		}
		demand, sparse := m.SupportsRepositoryCheckoutOptions(req.ExecutorType, req.SetupScript)
		if options.DownloadMode == models.DownloadOnDemand && !demand || len(options.SparseDirectories) > 0 && !sparse {
			return fmt.Errorf("repository checkout options are unavailable for this preparation path")
		}
	}
	return nil
}

func putPrimaryCheckoutOptions(metadata map[string]interface{}, req *LaunchRequest) {
	delete(metadata, models.RepositoryCheckoutOptionsKey)
	specs := req.RepoSpecs()
	if len(specs) > 0 && specs[0].CheckoutOptions != nil {
		metadata[models.RepositoryCheckoutOptionsKey] = specs[0].CheckoutOptions
	}
}

func checkoutOptionsValidationScript(options *models.RepositoryCheckoutOptions) string {
	if options == nil || len(options.SparseDirectories) == 0 {
		return ""
	}
	lines := []string{"\ncd {{workspace.path}}"}
	for _, directory := range options.SparseDirectories {
		lines = append(lines, `case "$(git --literal-pathspecs ls-tree -d HEAD -- `+shellQuote(directory)+` | cut -d ' ' -f 1)" in 040000|160000) ;; *) echo 'Selected folder is unavailable at this revision' >&2; exit 1 ;; esac`)
	}
	return strings.Join(lines, "\n") + "\n"
}

// checkoutCredentialEnvironment excludes profile controls from host Git commands.
func checkoutCredentialEnvironment(env map[string]string) map[string]string {
	result := make(map[string]string)
	for _, key := range append([]string{githubauth.CredentialHelperPathEnv, githubauth.CredentialCLIShimDirEnv}, managedGitCredentialBrokerEnvKeys...) {
		if value, ok := env[key]; ok {
			result[key] = value
		}
	}
	filtered, err := gitconfigenv.Filter(env, func(index int, entries []gitconfigenv.Entry) bool {
		entry := entries[index]
		if isManagedSetupScriptGitHelper(index, entries) {
			return true
		}
		return strings.HasPrefix(entry.Key, "credential.https://") && strings.HasSuffix(entry.Key, ".useHttpPath") && entry.Value == boolStringTrue
	})
	if err == nil {
		gitconfigenv.CopyIndexed(result, filtered)
	}
	return result
}
