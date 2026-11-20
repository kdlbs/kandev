package sqlite

import (
	"fmt"

	"github.com/kandev/kandev/internal/db"
)

// retiredAzureDevOpsTables are the tables of the removed built-in Azure DevOps
// integration. The integration is now the kandev-plugin-azure-devops runtime
// plugin, which keeps its own state; nothing in the host reads these tables.
var retiredAzureDevOpsTables = []string{
	"azure_devops_work_item_watch_tasks",
	"azure_devops_pull_request_watch_tasks",
	"azure_devops_work_item_watches",
	"azure_devops_pull_request_watches",
	"azure_devops_task_prs",
	"azure_devops_task_work_items",
	"azure_devops_configs",
}

// dropRetiredAzureDevOpsSchema removes the built-in Azure DevOps tables and
// the workspace PATs it stored. It runs after the boot backup and is a no-op
// on replay or on a database that never had the integration. The provider
// host backfill runs separately, after the repositories columns it reads.
func (r *Repository) dropRetiredAzureDevOpsSchema() error {
	ctx := r.migrationContext()
	for _, table := range retiredAzureDevOpsTables {
		if _, err := r.db.ExecContext(ctx, `DROP TABLE IF EXISTS `+table); err != nil {
			return fmt.Errorf("drop retired table %s: %w", table, err)
		}
	}
	_, err := r.db.ExecContext(ctx, `DELETE FROM secrets WHERE id LIKE 'azure_devops:%'`)
	if err != nil && !db.IsMissingTableError(err) {
		return fmt.Errorf("delete retired azure devops secrets: %w", err)
	}
	return nil
}

// retiredAzureDevOpsOrigin is the only origin the built-in integration
// accepted for organizations and clone URLs.
const retiredAzureDevOpsOrigin = "https://dev.azure.com"

// recordRetiredAzureDevOpsProviderHost gives repositories recorded by the
// built-in integration the provider host that managed Git credentials
// require. Only rows whose own HTTPS clone URL is on that origin change, so
// the host is derived from the row and never guessed.
func (r *Repository) recordRetiredAzureDevOpsProviderHost() error {
	_, err := r.db.ExecContext(r.migrationContext(), r.db.Rebind(`UPDATE repositories SET provider_host = ?
		WHERE provider = 'azure_devops' AND COALESCE(provider_host, '') = '' AND remote_url LIKE ?`),
		retiredAzureDevOpsOrigin, retiredAzureDevOpsOrigin+"/%")
	if err != nil {
		return fmt.Errorf("record retired azure devops provider host: %w", err)
	}
	return nil
}
