package lifecycle

import (
	"fmt"
	"strings"

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
		"if ! " + clone + ` 2>"$checkout_log"; then cat "$checkout_log" >&2; exit 1; fi`,
		`cat "$checkout_log" >&2`,
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
		lines = append(lines, `[ "$(git cat-file -t `+shellQuote("HEAD:"+directory)+`)" = tree ] || { echo 'Selected folder is unavailable at this revision' >&2; exit 1; }`)
	}
	return strings.Join(lines, "\n") + "\n"
}
