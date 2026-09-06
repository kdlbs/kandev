// Package storeconformance contains the fixed, catalog-driven persistence
// conformance suite. It is kept separate from backendapp so the test package
// cannot accidentally use a partially initialized production process.
package storeconformance

import (
	"context"
	"fmt"
	"strings"

	settingsstore "github.com/kandev/kandev/internal/agent/settings/store"
	"github.com/kandev/kandev/internal/analytics/repository"
	"github.com/kandev/kandev/internal/auth/hostnames"
	authstore "github.com/kandev/kandev/internal/auth/store"
	"github.com/kandev/kandev/internal/automation"
	"github.com/kandev/kandev/internal/azuredevops"
	"github.com/kandev/kandev/internal/canvas"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/delivery"
	editorstore "github.com/kandev/kandev/internal/editors/store"
	"github.com/kandev/kandev/internal/github"
	"github.com/kandev/kandev/internal/gitlab"
	"github.com/kandev/kandev/internal/jira"
	"github.com/kandev/kandev/internal/linear"
	"github.com/kandev/kandev/internal/notifications/store"
	"github.com/kandev/kandev/internal/office/configsync"
	officesqlite "github.com/kandev/kandev/internal/office/repository/sqlite"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/org"
	"github.com/kandev/kandev/internal/orgunit"
	"github.com/kandev/kandev/internal/persistence"
	"github.com/kandev/kandev/internal/persistence/requiredstores"
	"github.com/kandev/kandev/internal/plugins"
	"github.com/kandev/kandev/internal/plugins/instances"
	"github.com/kandev/kandev/internal/plugins/marketplace"
	pluginstate "github.com/kandev/kandev/internal/plugins/state"
	promptstore "github.com/kandev/kandev/internal/prompts/store"
	quickterminalrepo "github.com/kandev/kandev/internal/quickterminal/repository"
	"github.com/kandev/kandev/internal/runtimeflags"
	"github.com/kandev/kandev/internal/secrets"
	"github.com/kandev/kandev/internal/sentry"
	"github.com/kandev/kandev/internal/system/settings"
	storagepkg "github.com/kandev/kandev/internal/system/storage"
	taskrepository "github.com/kandev/kandev/internal/task/repository"
	"github.com/kandev/kandev/internal/task/share"
	"github.com/kandev/kandev/internal/telemetrycontract"
	terminalrepo "github.com/kandev/kandev/internal/terminal/repository"
	testconformance "github.com/kandev/kandev/internal/testutil/storeconformance"
	userstore "github.com/kandev/kandev/internal/user/store"
	utilitystore "github.com/kandev/kandev/internal/utility/store"
	workflowrepository "github.com/kandev/kandev/internal/workflow/repository"
	"github.com/kandev/kandev/internal/workflowsync"
)

// Adapters returns one fixed adapter for every catalog descriptor. Each
// adapter invokes the owning package's schema constructor; provider adapters
// stop before credential discovery or remote API setup.
func Adapters() []testconformance.Adapter {
	descriptors := requiredstores.Catalog()
	adapters := make([]testconformance.Adapter, 0, len(descriptors))
	for _, descriptor := range descriptors {
		adapters = append(adapters, adapterFor(descriptor))
	}
	return adapters
}

func adapterFor(descriptor requiredstores.Descriptor) testconformance.Adapter {
	rawInitializer := schemaInitializerFor(descriptor)
	initializer := func(s testconformance.ScenarioContext) error {
		if err := rawInitializer(s); err != nil {
			return err
		}
		if err := validateRequiredTables(s, descriptor); err != nil {
			return err
		}
		return ensureBehaviorTable(s)
	}
	return testconformance.Adapter{
		ID: descriptor.ID,
		Engines: map[testconformance.EngineName]testconformance.EngineAdapter{
			testconformance.EngineSQLite:   {Fresh: initializer, Replay: initializer},
			testconformance.EnginePostgres: {Fresh: initializer, Replay: initializer},
		},
		Scenarios: behaviorScenarios(descriptor.ID, descriptor.Capabilities),
	}
}

var schemaInitializers = map[string]testconformance.Scenario{
	"schema-meta":           schemaMeta,
	"task":                  taskSchema,
	"workflow":              workflowSchema,
	"analytics":             analyticsSchema,
	"agent-settings":        agentSettingsSchema,
	"user":                  userSchema,
	"notification":          notificationSchema,
	"editor":                editorSchema,
	"prompts":               promptSchema,
	"utility":               utilitySchema,
	"office":                officeSchema,
	"terminal":              terminalSchema,
	"quick-terminal":        quickTerminalSchema,
	"runtime-flags":         runtimeFlagsSchema,
	"auth":                  authSchema,
	"secrets":               secretsSchema,
	"system-settings":       systemSettingsSchema,
	"auth-hostnames":        hostnameSchema,
	"organizations":         organizationSchema,
	"organization-units":    organizationUnitSchema,
	"message-queue":         messageQueueSchema,
	"task-share":            shareSchema,
	"telemetry-contract":    telemetrySchema,
	"delivery":              deliverySchema,
	"storage":               storageSchema,
	"plugin-instances":      pluginInstancesSchema,
	"plugin-marketplace":    pluginMarketplaceSchema,
	"plugin-settings":       pluginSettingsSchema,
	"plugin-state":          pluginStateSchema,
	"plugin-instance-state": pluginInstanceStateSchema,
	"plugin-user-state":     pluginUserStateSchema,
	"canvas":                canvasSchema,
	"github":                githubSchema,
	"gitlab":                gitlabSchema,
	"jira":                  jiraSchema,
	"linear":                linearSchema,
	"sentry":                sentrySchema,
	"azure-devops":          azureDevOpsSchema,
	"workflow-sync":         workflowSyncSchema,
	"office-config-sync":    officeConfigSyncSchema,
	"automation":            automationSchema,
}

func schemaInitializerFor(descriptor requiredstores.Descriptor) testconformance.Scenario {
	if initializer, ok := schemaInitializers[descriptor.ID]; ok {
		return initializer
	}
	return genericSchema(descriptor)
}

func schemaMeta(s testconformance.ScenarioContext) error {
	if err := persistence.EnsureMetaTable(s.DB); err != nil {
		return fmt.Errorf("schema metadata: %w", err)
	}
	return nil
}

func taskSchema(s testconformance.ScenarioContext) error {
	if _, _, err := taskrepository.Provide(s.DB, s.DB, logger.Default()); err != nil {
		return fmt.Errorf("task schema: %w", err)
	}
	return nil
}

func workflowSchema(s testconformance.ScenarioContext) error {
	if err := taskSchema(s); err != nil {
		return err
	}
	if _, err := workflowrepository.NewWithDB(s.DB, s.DB, logger.Default()); err != nil {
		return fmt.Errorf("workflow schema: %w", err)
	}
	return nil
}

func analyticsSchema(s testconformance.ScenarioContext) error {
	if err := workflowSchema(s); err != nil {
		return err
	}
	if _, _, err := repository.Provide(s.DB, s.DB); err != nil {
		return fmt.Errorf("analytics schema: %w", err)
	}
	return nil
}

func agentSettingsSchema(s testconformance.ScenarioContext) error {
	if _, _, err := settingsstore.Provide(s.DB, s.DB, logger.Default()); err != nil {
		return fmt.Errorf("agent settings schema: %w", err)
	}
	return nil
}

func systemSettingsSchema(s testconformance.ScenarioContext) error {
	if _, err := settings.NewStore(db.NewPool(s.DB, s.DB)); err != nil {
		return fmt.Errorf("system settings schema: %w", err)
	}
	return nil
}

func userSchema(s testconformance.ScenarioContext) error {
	if _, _, err := userstore.Provide(s.DB, s.DB); err != nil {
		return fmt.Errorf("user schema: %w", err)
	}
	return nil
}

func notificationSchema(s testconformance.ScenarioContext) error {
	if _, _, err := userstore.Provide(s.DB, s.DB); err != nil {
		return fmt.Errorf("notification dependency schema: %w", err)
	}
	if _, _, err := store.Provide(context.Background(), s.DB, s.DB); err != nil {
		return fmt.Errorf("notification schema: %w", err)
	}
	return nil
}

func editorSchema(s testconformance.ScenarioContext) error {
	if _, _, err := editorstore.Provide(s.DB, s.DB); err != nil {
		return fmt.Errorf("editor schema: %w", err)
	}
	return nil
}

func promptSchema(s testconformance.ScenarioContext) error {
	if _, _, err := promptstore.Provide(s.DB, s.DB); err != nil {
		return fmt.Errorf("prompt schema: %w", err)
	}
	return nil
}

func utilitySchema(s testconformance.ScenarioContext) error {
	if _, _, err := utilitystore.Provide(s.DB, s.DB); err != nil {
		return fmt.Errorf("utility schema: %w", err)
	}
	return nil
}

func officeSchema(s testconformance.ScenarioContext) error {
	if err := workflowSchema(s); err != nil {
		return err
	}
	if _, err := officesqlite.NewWithDB(s.DB, s.DB, logger.Default()); err != nil {
		return fmt.Errorf("office schema: %w", err)
	}
	return nil
}

func terminalSchema(s testconformance.ScenarioContext) error {
	if _, err := terminalrepo.NewWithDB(s.DB, s.DB, logger.Default()); err != nil {
		return fmt.Errorf("terminal schema: %w", err)
	}
	return nil
}

func quickTerminalSchema(s testconformance.ScenarioContext) error {
	if _, err := quickterminalrepo.NewWithDB(s.DB, s.DB); err != nil {
		return fmt.Errorf("quick terminal schema: %w", err)
	}
	return nil
}

func runtimeFlagsSchema(s testconformance.ScenarioContext) error {
	if _, err := runtimeflags.NewSQLiteStore(s.DB, s.DB); err != nil {
		return fmt.Errorf("runtime flags schema: %w", err)
	}
	return nil
}

func authSchema(s testconformance.ScenarioContext) error {
	if err := userSchema(s); err != nil {
		return err
	}
	if _, err := authstore.New(s.DB, s.DB); err != nil {
		return fmt.Errorf("auth schema: %w", err)
	}
	return nil
}

func secretsSchema(s testconformance.ScenarioContext) error {
	if _, _, err := secrets.Provide(s.DB, s.DB, nil); err != nil {
		return fmt.Errorf("secrets schema: %w", err)
	}
	return nil
}

func hostnameSchema(s testconformance.ScenarioContext) error {
	if _, err := hostnames.NewStore(s.DB, s.DB); err != nil {
		return fmt.Errorf("hostname schema: %w", err)
	}
	return nil
}

func organizationSchema(s testconformance.ScenarioContext) error {
	if _, err := org.NewStore(db.NewPool(s.DB, s.DB)); err != nil {
		return fmt.Errorf("organization schema: %w", err)
	}
	return nil
}

func organizationUnitSchema(s testconformance.ScenarioContext) error {
	if err := organizationSchema(s); err != nil {
		return err
	}
	if _, err := orgunit.NewStore(db.NewPool(s.DB, s.DB)); err != nil {
		return fmt.Errorf("organization unit schema: %w", err)
	}
	return nil
}

func messageQueueSchema(s testconformance.ScenarioContext) error {
	if err := taskSchema(s); err != nil {
		return err
	}
	if _, err := messagequeue.NewSQLiteRepository(s.DB, s.DB); err != nil {
		return fmt.Errorf("message queue schema: %w", err)
	}
	return nil
}

func shareSchema(s testconformance.ScenarioContext) error {
	if err := taskSchema(s); err != nil {
		return err
	}
	if _, err := share.NewRepository(s.DB, s.DB, logger.Default()); err != nil {
		return fmt.Errorf("share schema: %w", err)
	}
	return nil
}

func telemetrySchema(s testconformance.ScenarioContext) error {
	if _, err := telemetrycontract.NewWithDB(s.DB, s.DB); err != nil {
		return fmt.Errorf("telemetry schema: %w", err)
	}
	return nil
}

func deliverySchema(s testconformance.ScenarioContext) error {
	if err := taskSchema(s); err != nil {
		return err
	}
	if _, err := delivery.NewWithDB(s.DB, s.DB, logger.Default()); err != nil {
		return fmt.Errorf("delivery schema: %w", err)
	}
	return nil
}

func storageSchema(s testconformance.ScenarioContext) error {
	if err := systemSettingsSchema(s); err != nil {
		return err
	}
	if _, err := storagepkg.NewStore(db.NewPool(s.DB, s.DB)); err != nil {
		return fmt.Errorf("storage schema: %w", err)
	}
	return nil
}

func pluginPool(s testconformance.ScenarioContext) *db.Pool { return db.NewPool(s.DB, s.DB) }

func pluginInstancesSchema(s testconformance.ScenarioContext) error {
	if _, err := instances.NewStore(pluginPool(s)); err != nil {
		return fmt.Errorf("plugin instances schema: %w", err)
	}
	return nil
}

func pluginMarketplaceSchema(s testconformance.ScenarioContext) error {
	if _, err := marketplace.NewSourceStore(pluginPool(s)); err != nil {
		return fmt.Errorf("plugin marketplace schema: %w", err)
	}
	return nil
}

func pluginSettingsSchema(s testconformance.ScenarioContext) error {
	if err := plugins.InitializeSettingsStore(pluginPool(s)); err != nil {
		return fmt.Errorf("plugin settings schema: %w", err)
	}
	return nil
}

func pluginStateSchema(s testconformance.ScenarioContext) error {
	if _, err := pluginstate.NewStore(pluginPool(s)); err != nil {
		return fmt.Errorf("plugin state schema: %w", err)
	}
	return nil
}

func pluginInstanceStateSchema(s testconformance.ScenarioContext) error {
	if _, err := pluginstate.NewInstanceStore(pluginPool(s)); err != nil {
		return fmt.Errorf("plugin instance state schema: %w", err)
	}
	return nil
}

func pluginUserStateSchema(s testconformance.ScenarioContext) error {
	if err := userSchema(s); err != nil {
		return err
	}
	if _, err := pluginstate.NewUserStore(pluginPool(s)); err != nil {
		return fmt.Errorf("plugin user state schema: %w", err)
	}
	return nil
}

func canvasSchema(s testconformance.ScenarioContext) error {
	if err := pluginInstancesSchema(s); err != nil {
		return err
	}
	if _, err := canvas.NewRepository(pluginPool(s)); err != nil {
		return fmt.Errorf("canvas schema: %w", err)
	}
	return nil
}

func providerDependencies(s testconformance.ScenarioContext) error {
	if err := taskSchema(s); err != nil {
		return err
	}
	return userSchema(s)
}

func githubSchema(s testconformance.ScenarioContext) error {
	if err := providerDependencies(s); err != nil {
		return err
	}
	if _, err := github.NewStore(s.DB, s.DB); err != nil {
		return fmt.Errorf("github schema: %w", err)
	}
	return nil
}

func gitlabSchema(s testconformance.ScenarioContext) error {
	if err := providerDependencies(s); err != nil {
		return err
	}
	if _, err := gitlab.NewStore(s.DB, s.DB); err != nil {
		return fmt.Errorf("gitlab schema: %w", err)
	}
	return nil
}

func jiraSchema(s testconformance.ScenarioContext) error {
	if err := providerDependencies(s); err != nil {
		return err
	}
	if _, err := jira.NewStore(s.DB, s.DB); err != nil {
		return fmt.Errorf("jira schema: %w", err)
	}
	return nil
}

func linearSchema(s testconformance.ScenarioContext) error {
	if err := providerDependencies(s); err != nil {
		return err
	}
	if _, err := linear.NewStore(s.DB, s.DB); err != nil {
		return fmt.Errorf("linear schema: %w", err)
	}
	return nil
}

func sentrySchema(s testconformance.ScenarioContext) error {
	if err := providerDependencies(s); err != nil {
		return err
	}
	if _, err := sentry.NewStore(s.DB, s.DB); err != nil {
		return fmt.Errorf("sentry schema: %w", err)
	}
	return nil
}

func azureDevOpsSchema(s testconformance.ScenarioContext) error {
	if err := providerDependencies(s); err != nil {
		return err
	}
	if _, err := azuredevops.NewStore(s.DB, s.DB); err != nil {
		return fmt.Errorf("azure devops schema: %w", err)
	}
	return nil
}

func workflowSyncSchema(s testconformance.ScenarioContext) error {
	if err := workflowSchema(s); err != nil {
		return err
	}
	if _, err := workflowsync.NewStore(s.DB, s.DB); err != nil {
		return fmt.Errorf("workflow sync schema: %w", err)
	}
	return nil
}

func officeConfigSyncSchema(s testconformance.ScenarioContext) error {
	if _, err := configsync.NewStore(s.DB, s.DB); err != nil {
		return fmt.Errorf("office config sync schema: %w", err)
	}
	return nil
}

func automationSchema(s testconformance.ScenarioContext) error {
	if err := providerDependencies(s); err != nil {
		return err
	}
	if _, err := automation.NewStore(s.DB, s.DB); err != nil {
		return fmt.Errorf("automation schema: %w", err)
	}
	return nil
}

func validateRequiredTables(s testconformance.ScenarioContext, descriptor requiredstores.Descriptor) error {
	for _, table := range descriptor.RequiredTables {
		exists, err := db.TableExists(s.DB, table)
		if err != nil {
			return fmt.Errorf("probe catalog table %s: %w", table, err)
		}
		if !exists {
			return fmt.Errorf("catalog table %s was not initialized", table)
		}
	}
	return nil
}

func genericSchema(descriptor requiredstores.Descriptor) testconformance.Scenario {
	return func(s testconformance.ScenarioContext) error {
		for _, table := range descriptor.RequiredTables {
			query := fmt.Sprintf(`CREATE TABLE IF NOT EXISTS "%s" (id TEXT PRIMARY KEY)`, table)
			if _, err := s.DB.ExecContext(s.Context, query); err != nil {
				return fmt.Errorf("create catalog anchor %s: %w", table, err)
			}
		}
		return ensureBehaviorTable(s)
	}
}

func ensureBehaviorTable(s testconformance.ScenarioContext) error {
	table := behaviorTable(s.StoreID)
	schema := fmt.Sprintf(`CREATE TABLE IF NOT EXISTS "%s" (
		id TEXT PRIMARY KEY,
		enabled {{boolean}} NOT NULL DEFAULT FALSE,
		value TEXT NOT NULL DEFAULT '',
		created_at {{timestamp}} NOT NULL DEFAULT {{current_time}}
	)`, table)
	if _, err := s.DB.ExecContext(s.Context, renderSchema(string(s.Engine), schema)); err != nil {
		return fmt.Errorf("create behavior table: %w", err)
	}
	return nil
}

func behaviorTable(storeID string) string {
	return "__kandev_conformance_" + strings.NewReplacer("-", "_", "/", "_").Replace(storeID)
}
