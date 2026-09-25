// Package requiredstores defines the inventory and lifecycle contract for
// Kandev's built-in SQL schema owners.
package requiredstores

import (
	"fmt"
	"strings"

	"github.com/kandev/kandev/internal/startup"
)

// Capability identifies a SQL behavior that a store conformance adapter must
// exercise on every supported database engine.
type Capability string

const (
	CapabilityBoolean     Capability = "boolean"
	CapabilityTimestamp   Capability = "timestamp"
	CapabilityConflict    Capability = "conflict"
	CapabilityTransaction Capability = "transaction"
)

var validCapabilities = map[Capability]struct{}{
	CapabilityBoolean:     {},
	CapabilityTimestamp:   {},
	CapabilityConflict:    {},
	CapabilityTransaction: {},
}

// Store IDs referenced from multiple catalog entries, either as their own ID
// or as another entry's DependsOn, kept as constants so the two never drift.
const (
	storeIDSchemaMeta      = "schema-meta"
	storeIDTask            = "task"
	storeIDWorkflow        = "workflow"
	storeIDAgentSettings   = "agent-settings"
	storeIDUser            = "user"
	storeIDPluginInstances = "plugin-instances"

	pkgPluginsState = "internal/plugins/state"
)

// Descriptor is the immutable metadata for one built-in SQL schema owner.
// RequiredTables are the table names used by runtime persistence probes.
type Descriptor struct {
	ID             string
	OwnerPackage   string
	RequiredTables []string
	DependsOn      []string
	Capabilities   []Capability

	// Sweep names which store-admission startup step (stores.repositories or
	// stores.services) claims this entry's `recordRequiredStore` call. Runtime
	// admission order decides the owner, not file or definition line, so this
	// is the single declared source both sweeps' totals read at BeginStep and
	// what the catalog/registry completeness check asserts is total and
	// non-overlapping.
	Sweep startup.StepID
}

// catalog is ordered in the same dependency order used by backend startup.
// Keep entries for stores whose feature is disabled: their schema remains part
// of the database contract and must be initialized before readiness.
var catalog = []Descriptor{
	{ID: storeIDSchemaMeta, OwnerPackage: "internal/persistence", RequiredTables: []string{"kandev_meta"}, Sweep: startup.StepStoresRepositories},
	{ID: storeIDTask, OwnerPackage: "internal/task/repository/sqlite", RequiredTables: []string{"workspaces", "tasks", "task_workflow_session_bindings", "task_environment_kubernetes", "task_exact_profile_assignments"}, DependsOn: []string{storeIDSchemaMeta}, Capabilities: []Capability{CapabilityBoolean, CapabilityTimestamp, CapabilityConflict, CapabilityTransaction}, Sweep: startup.StepStoresRepositories},
	{ID: storeIDWorkflow, OwnerPackage: "internal/workflow/repository", RequiredTables: []string{"workflow_templates", "workflow_steps"}, DependsOn: []string{storeIDTask}, Capabilities: []Capability{CapabilityTimestamp, CapabilityConflict, CapabilityTransaction}, Sweep: startup.StepStoresRepositories},
	{ID: "analytics", OwnerPackage: "internal/analytics/repository", RequiredTables: []string{"tasks"}, DependsOn: []string{storeIDTask, storeIDWorkflow}, Capabilities: []Capability{CapabilityTimestamp}, Sweep: startup.StepStoresRepositories},
	{ID: storeIDAgentSettings, OwnerPackage: "internal/agent/settings/store", RequiredTables: []string{"agents", "agent_profiles"}, DependsOn: []string{storeIDSchemaMeta}, Capabilities: []Capability{CapabilityBoolean, CapabilityTimestamp, CapabilityConflict}, Sweep: startup.StepStoresRepositories},
	{ID: storeIDUser, OwnerPackage: "internal/user/store", RequiredTables: []string{"users"}, DependsOn: []string{storeIDSchemaMeta}, Capabilities: []Capability{CapabilityBoolean, CapabilityTimestamp, CapabilityConflict}, Sweep: startup.StepStoresRepositories},
	{ID: "notification", OwnerPackage: "internal/notifications/store", RequiredTables: []string{"notification_providers"}, DependsOn: []string{storeIDUser}, Capabilities: []Capability{CapabilityBoolean, CapabilityTimestamp, CapabilityConflict}, Sweep: startup.StepStoresRepositories},
	{ID: "editor", OwnerPackage: "internal/editors/store", RequiredTables: []string{"editors"}, DependsOn: []string{storeIDUser}, Capabilities: []Capability{CapabilityTimestamp, CapabilityConflict}, Sweep: startup.StepStoresRepositories},
	{ID: "prompts", OwnerPackage: "internal/prompts/store", RequiredTables: []string{"custom_prompts"}, DependsOn: []string{storeIDUser}, Capabilities: []Capability{CapabilityTimestamp, CapabilityConflict}, Sweep: startup.StepStoresRepositories},
	{ID: "utility", OwnerPackage: "internal/utility/store", RequiredTables: []string{"utility_agents"}, DependsOn: []string{storeIDAgentSettings}, Capabilities: []Capability{CapabilityBoolean, CapabilityTimestamp, CapabilityConflict}, Sweep: startup.StepStoresRepositories},
	{ID: "office", OwnerPackage: "internal/office/repository/sqlite", RequiredTables: []string{"office_projects", "runs"}, DependsOn: []string{storeIDTask, storeIDAgentSettings}, Capabilities: []Capability{CapabilityBoolean, CapabilityTimestamp, CapabilityConflict, CapabilityTransaction}, Sweep: startup.StepStoresRepositories},
	{ID: "terminal", OwnerPackage: "internal/terminal/repository", RequiredTables: []string{"user_terminals"}, DependsOn: []string{storeIDUser}, Capabilities: []Capability{CapabilityBoolean, CapabilityTimestamp, CapabilityConflict}, Sweep: startup.StepStoresRepositories},
	{ID: "quick-terminal", OwnerPackage: "internal/quickterminal/repository", RequiredTables: []string{"quick_terminal_tabs"}, DependsOn: []string{storeIDUser}, Capabilities: []Capability{CapabilityTimestamp, CapabilityConflict}, Sweep: startup.StepStoresRepositories},
	{ID: "runtime-flags", OwnerPackage: "internal/runtimeflags", RequiredTables: []string{"runtime_flag_overrides"}, DependsOn: []string{storeIDSchemaMeta}, Capabilities: []Capability{CapabilityBoolean, CapabilityTimestamp, CapabilityConflict}, Sweep: startup.StepStoresRepositories},
	{ID: "auth", OwnerPackage: "internal/auth/store", RequiredTables: []string{"auth_identities", "auth_sessions"}, DependsOn: []string{storeIDUser}, Capabilities: []Capability{CapabilityBoolean, CapabilityTimestamp, CapabilityConflict, CapabilityTransaction}, Sweep: startup.StepStoresRepositories},
	{ID: "secrets", OwnerPackage: "internal/secrets", RequiredTables: []string{"secrets"}, DependsOn: []string{storeIDTask}, Capabilities: []Capability{CapabilityTimestamp, CapabilityConflict, CapabilityTransaction}, Sweep: startup.StepStoresRepositories},
	{ID: "system-settings", OwnerPackage: "internal/system/settings", RequiredTables: []string{"settings"}, DependsOn: []string{storeIDSchemaMeta}, Capabilities: []Capability{CapabilityTimestamp, CapabilityConflict}, Sweep: startup.StepStoresRepositories},
	{ID: "auth-hostnames", OwnerPackage: "internal/auth/hostnames", RequiredTables: []string{"auth_hostname_cache"}, DependsOn: []string{"auth"}, Capabilities: []Capability{CapabilityTimestamp, CapabilityConflict}, Sweep: startup.StepStoresRepositories},
	{ID: "organizations", OwnerPackage: "internal/org", RequiredTables: []string{"orgs"}, DependsOn: []string{storeIDUser}, Capabilities: []Capability{CapabilityBoolean, CapabilityTimestamp, CapabilityConflict}, Sweep: startup.StepStoresServices},
	{ID: "organization-units", OwnerPackage: "internal/orgunit", RequiredTables: []string{"org_units"}, DependsOn: []string{"organizations"}, Capabilities: []Capability{CapabilityTimestamp, CapabilityConflict}, Sweep: startup.StepStoresServices},
	{ID: "message-queue", OwnerPackage: "internal/orchestrator/messagequeue", RequiredTables: []string{"queued_messages", "queue_admission_receipts"}, DependsOn: []string{storeIDTask}, Capabilities: []Capability{CapabilityBoolean, CapabilityTimestamp, CapabilityConflict, CapabilityTransaction}, Sweep: startup.StepStoresServices},
	{ID: "task-share", OwnerPackage: "internal/task/share", RequiredTables: []string{"task_shares"}, DependsOn: []string{storeIDTask}, Capabilities: []Capability{CapabilityTimestamp, CapabilityConflict}, Sweep: startup.StepStoresServices},
	{ID: "telemetry-contract", OwnerPackage: "internal/telemetrycontract", RequiredTables: []string{"telemetry_activations"}, DependsOn: []string{storeIDSchemaMeta}, Capabilities: []Capability{CapabilityTimestamp, CapabilityConflict}, Sweep: startup.StepStoresRepositories},
	{ID: "delivery", OwnerPackage: "internal/delivery", RequiredTables: []string{"task_delivery_ledger"}, DependsOn: []string{storeIDTask}, Capabilities: []Capability{CapabilityTimestamp, CapabilityConflict, CapabilityTransaction}, Sweep: startup.StepStoresServices},
	{ID: "storage", OwnerPackage: "internal/system/storage", RequiredTables: []string{"storage_maintenance_runs", "storage_quarantine_entries"}, DependsOn: []string{storeIDSchemaMeta}, Capabilities: []Capability{CapabilityTimestamp, CapabilityConflict, CapabilityTransaction}, Sweep: startup.StepStoresServices},
	{ID: storeIDPluginInstances, OwnerPackage: "internal/plugins/instances", RequiredTables: []string{"plugin_instances"}, DependsOn: []string{storeIDSchemaMeta}, Capabilities: []Capability{CapabilityBoolean, CapabilityTimestamp, CapabilityConflict}, Sweep: startup.StepStoresServices},
	{ID: "plugin-marketplace", OwnerPackage: "internal/plugins/marketplace", RequiredTables: []string{"plugin_marketplace_source"}, DependsOn: []string{storeIDSchemaMeta}, Capabilities: []Capability{CapabilityBoolean, CapabilityTimestamp, CapabilityConflict}, Sweep: startup.StepStoresServices},
	{ID: "plugin-settings", OwnerPackage: "internal/plugins", RequiredTables: []string{"plugin_settings"}, DependsOn: []string{storeIDSchemaMeta}, Capabilities: []Capability{CapabilityBoolean, CapabilityTimestamp, CapabilityConflict}, Sweep: startup.StepStoresServices},
	{ID: "plugin-state", OwnerPackage: pkgPluginsState, RequiredTables: []string{"plugin_state"}, DependsOn: []string{storeIDSchemaMeta}, Capabilities: []Capability{CapabilityBoolean, CapabilityTimestamp, CapabilityConflict}, Sweep: startup.StepStoresServices},
	{ID: "plugin-instance-state", OwnerPackage: pkgPluginsState, RequiredTables: []string{"plugin_instance_state"}, DependsOn: []string{storeIDPluginInstances}, Capabilities: []Capability{CapabilityBoolean, CapabilityTimestamp, CapabilityConflict}, Sweep: startup.StepStoresServices},
	{ID: "plugin-user-state", OwnerPackage: pkgPluginsState, RequiredTables: []string{"plugin_user_state"}, DependsOn: []string{storeIDUser}, Capabilities: []Capability{CapabilityBoolean, CapabilityTimestamp, CapabilityConflict}, Sweep: startup.StepStoresServices},
	{ID: "canvas", OwnerPackage: "internal/canvas", RequiredTables: []string{"canvas_lifecycle_metadata", "canvas_creation_authority", "canvas_install_receipts"}, DependsOn: []string{storeIDPluginInstances}, Capabilities: []Capability{CapabilityBoolean, CapabilityTimestamp, CapabilityConflict, CapabilityTransaction}, Sweep: startup.StepStoresServices},
	{ID: "github", OwnerPackage: "internal/github", RequiredTables: []string{"github_pr_watches"}, DependsOn: []string{storeIDTask, storeIDUser}, Capabilities: []Capability{CapabilityBoolean, CapabilityTimestamp, CapabilityConflict}, Sweep: startup.StepStoresServices},
	{ID: "gitlab", OwnerPackage: "internal/gitlab", RequiredTables: []string{"gitlab_configs"}, DependsOn: []string{storeIDTask, storeIDUser}, Capabilities: []Capability{CapabilityBoolean, CapabilityTimestamp, CapabilityConflict}, Sweep: startup.StepStoresServices},
	{ID: "jira", OwnerPackage: "internal/jira", RequiredTables: []string{"jira_configs"}, DependsOn: []string{storeIDTask, storeIDUser}, Capabilities: []Capability{CapabilityBoolean, CapabilityTimestamp, CapabilityConflict}, Sweep: startup.StepStoresServices},
	{ID: "linear", OwnerPackage: "internal/linear", RequiredTables: []string{"linear_configs"}, DependsOn: []string{storeIDTask, storeIDUser}, Capabilities: []Capability{CapabilityBoolean, CapabilityTimestamp, CapabilityConflict}, Sweep: startup.StepStoresServices},
	{ID: "sentry", OwnerPackage: "internal/sentry", RequiredTables: []string{"sentry_configs"}, DependsOn: []string{storeIDTask, storeIDUser}, Capabilities: []Capability{CapabilityBoolean, CapabilityTimestamp, CapabilityConflict}, Sweep: startup.StepStoresServices},
	{ID: "azure-devops", OwnerPackage: "internal/azuredevops", RequiredTables: []string{"azure_devops_configs"}, DependsOn: []string{storeIDTask, storeIDUser}, Capabilities: []Capability{CapabilityBoolean, CapabilityTimestamp, CapabilityConflict}, Sweep: startup.StepStoresServices},
	{ID: "workflow-sync", OwnerPackage: "internal/workflowsync", RequiredTables: []string{"workflow_sync_configs"}, DependsOn: []string{storeIDWorkflow}, Capabilities: []Capability{CapabilityBoolean, CapabilityTimestamp, CapabilityConflict}, Sweep: startup.StepStoresServices},
	{ID: "office-config-sync", OwnerPackage: "internal/office/configsync", RequiredTables: []string{"office_config_sync_configs"}, DependsOn: []string{"office"}, Capabilities: []Capability{CapabilityBoolean, CapabilityTimestamp, CapabilityConflict}, Sweep: startup.StepStoresServices},
	{ID: "automation", OwnerPackage: "internal/automation", RequiredTables: []string{"automations", "automation_runs"}, DependsOn: []string{storeIDTask, storeIDUser}, Capabilities: []Capability{CapabilityBoolean, CapabilityTimestamp, CapabilityConflict, CapabilityTransaction}, Sweep: startup.StepStoresServices},
}

// Catalog returns a deep copy of the authoritative store catalog.
func Catalog() []Descriptor {
	return cloneDescriptors(catalog)
}

// Descriptors is an alias for Catalog for callers that prefer collection
// terminology.
func Descriptors() []Descriptor {
	return Catalog()
}

// ValidateCatalog checks descriptor identity, dependencies, and capabilities.
func ValidateCatalog(descriptors []Descriptor) error {
	if len(descriptors) == 0 {
		return fmt.Errorf("catalog is empty")
	}
	positions := make(map[string]int, len(descriptors))
	for index, descriptor := range descriptors {
		if !validIdentifier(descriptor.ID) {
			return fmt.Errorf("catalog entry %q has invalid ID", descriptor.ID)
		}
		if _, exists := positions[descriptor.ID]; exists {
			return fmt.Errorf("catalog contains duplicate ID %q", descriptor.ID)
		}
		positions[descriptor.ID] = index
		if strings.TrimSpace(descriptor.OwnerPackage) == "" {
			return fmt.Errorf("catalog entry %q has no owner package", descriptor.ID)
		}
		if len(descriptor.RequiredTables) == 0 {
			return fmt.Errorf("catalog entry %q has no required tables", descriptor.ID)
		}
		if descriptor.Sweep != startup.StepStoresRepositories && descriptor.Sweep != startup.StepStoresServices {
			return fmt.Errorf("catalog entry %q has invalid sweep %q", descriptor.ID, descriptor.Sweep)
		}
		if err := validateNames(descriptor); err != nil {
			return err
		}
	}

	for index, descriptor := range descriptors {
		seenDependencies := make(map[string]struct{}, len(descriptor.DependsOn))
		for _, dependency := range descriptor.DependsOn {
			if dependency == descriptor.ID {
				return fmt.Errorf("catalog entry %q depends on itself", descriptor.ID)
			}
			if _, duplicate := seenDependencies[dependency]; duplicate {
				return fmt.Errorf("catalog entry %q lists duplicate dependency %q", descriptor.ID, dependency)
			}
			seenDependencies[dependency] = struct{}{}
			dependencyIndex, exists := positions[dependency]
			if !exists {
				return fmt.Errorf("catalog entry %q depends on unknown store %q", descriptor.ID, dependency)
			}
			if dependencyIndex >= index {
				return fmt.Errorf("catalog entry %q dependency %q is out of order", descriptor.ID, dependency)
			}
		}
	}

	if err := validateCycles(descriptors); err != nil {
		return err
	}
	return nil
}

func validateNames(descriptor Descriptor) error {
	seen := make(map[string]struct{}, len(descriptor.RequiredTables))
	for _, table := range descriptor.RequiredTables {
		if !validIdentifier(table) {
			return fmt.Errorf("catalog entry %q has invalid required table %q", descriptor.ID, table)
		}
		if _, exists := seen[table]; exists {
			return fmt.Errorf("catalog entry %q lists duplicate table %q", descriptor.ID, table)
		}
		seen[table] = struct{}{}
	}
	for _, capability := range descriptor.Capabilities {
		if _, ok := validCapabilities[capability]; !ok {
			return fmt.Errorf("catalog entry %q has invalid capability %q", descriptor.ID, capability)
		}
	}
	return nil
}

func validateCycles(descriptors []Descriptor) error {
	dependencies := make(map[string][]string, len(descriptors))
	for _, descriptor := range descriptors {
		dependencies[descriptor.ID] = descriptor.DependsOn
	}
	visiting := make(map[string]bool, len(descriptors))
	visited := make(map[string]bool, len(descriptors))
	var visit func(string) error
	visit = func(id string) error {
		if visiting[id] {
			return fmt.Errorf("catalog dependency cycle includes %q", id)
		}
		if visited[id] {
			return nil
		}
		visiting[id] = true
		for _, dependency := range dependencies[id] {
			if err := visit(dependency); err != nil {
				return err
			}
		}
		visiting[id] = false
		visited[id] = true
		return nil
	}
	for _, descriptor := range descriptors {
		if err := visit(descriptor.ID); err != nil {
			return err
		}
	}
	return nil
}

func validIdentifier(value string) bool {
	if value == "" || strings.Contains(value, "--") {
		return false
	}
	for index, character := range value {
		valid := character == '_' || character == '-' || character >= 'a' && character <= 'z' || character >= '0' && character <= '9'
		if !valid || index == 0 && (character == '-' || character == '_') {
			return false
		}
	}
	return !strings.HasSuffix(value, "-") && !strings.HasSuffix(value, "_")
}

func cloneDescriptors(descriptors []Descriptor) []Descriptor {
	cloned := make([]Descriptor, len(descriptors))
	for index, descriptor := range descriptors {
		cloned[index] = descriptor
		cloned[index].RequiredTables = append([]string(nil), descriptor.RequiredTables...)
		cloned[index].DependsOn = append([]string(nil), descriptor.DependsOn...)
		cloned[index].Capabilities = append([]Capability(nil), descriptor.Capabilities...)
	}
	return cloned
}
