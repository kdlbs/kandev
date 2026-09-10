//nolint:revive // Each catalog owner has an explicit API adapter in this test contract.
package storeconformance

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"time"

	analyticsrepository "github.com/kandev/kandev/internal/analytics/repository"
	"github.com/kandev/kandev/internal/auth/hostnames"
	authstore "github.com/kandev/kandev/internal/auth/store"
	"github.com/kandev/kandev/internal/automation"
	"github.com/kandev/kandev/internal/azuredevops"
	"github.com/kandev/kandev/internal/canvas"
	"github.com/kandev/kandev/internal/common/authcircuit"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/delivery"
	"github.com/kandev/kandev/internal/github"
	"github.com/kandev/kandev/internal/gitlab"
	"github.com/kandev/kandev/internal/jira"
	"github.com/kandev/kandev/internal/linear"
	"github.com/kandev/kandev/internal/office/configsync"
	officemodels "github.com/kandev/kandev/internal/office/models"
	officesqlite "github.com/kandev/kandev/internal/office/repository/sqlite"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/org"
	"github.com/kandev/kandev/internal/orgunit"
	"github.com/kandev/kandev/internal/persistence"
	"github.com/kandev/kandev/internal/plugins"
	"github.com/kandev/kandev/internal/plugins/instances"
	"github.com/kandev/kandev/internal/plugins/marketplace"
	pluginstate "github.com/kandev/kandev/internal/plugins/state"
	quickterminalrepo "github.com/kandev/kandev/internal/quickterminal/repository"
	"github.com/kandev/kandev/internal/runtimeflags"
	"github.com/kandev/kandev/internal/secrets"
	"github.com/kandev/kandev/internal/sentry"
	"github.com/kandev/kandev/internal/system/settings"
	storagepkg "github.com/kandev/kandev/internal/system/storage"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/share"
	"github.com/kandev/kandev/internal/telemetrycontract"
	terminalrepo "github.com/kandev/kandev/internal/terminal/repository"
	testconformance "github.com/kandev/kandev/internal/testutil/storeconformance"
	userstore "github.com/kandev/kandev/internal/user/store"
	"github.com/kandev/kandev/internal/workflowsync"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

const defaultUserID = userstore.DefaultUserID

type metaBehaviorRecord struct {
	ID        string
	Value     string
	CreatedAt time.Time
	UpdatedAt time.Time
}

//nolint:funlen // The catalog-to-owner mapping is deliberately explicit.
func buildOwnerBehaviors() map[string]ownerBehavior {
	behaviors := map[string]ownerBehavior{
		"schema-meta": {actions: []apiAction{metaAction()}},
		"task": {actions: []apiAction{
			standardAction(reflectedSpec{
				name: "workspaces", factory: taskFactory,
				createMethod: "CreateWorkspace", readMethod: "GetWorkspace",
				updateMethod: "UpdateWorkspace", deleteMethod: "DeleteWorkspace",
			}),
			taskAction(),
		}},
		"workflow": {actions: []apiAction{
			standardAction(reflectedSpec{
				name: "workflow_templates", factory: workflowFactory,
				createMethod: "CreateTemplate", readMethod: "GetTemplate",
				updateMethod: "UpdateTemplate", deleteMethod: "DeleteTemplate",
			}),
		}},
		"agent-settings": {actions: []apiAction{
			standardAction(reflectedSpec{
				name: "agents", factory: settingsFactory,
				createMethod: "CreateAgent", readMethod: "GetAgent",
				updateMethod: "UpdateAgent", deleteMethod: "DeleteAgent",
				booleanFields: []string{"SupportsMCP"},
				updateMutation: func(record any, _ string) {
					value := reflectValue(record)
					setBoolField(value, "SupportsMCP", true)
				},
				updateAssertion: func(_ any, after any, _ string) error {
					enabled, ok := recordBoolean(after, []string{"SupportsMCP"})
					if !ok || !enabled {
						return fmt.Errorf("agent SupportsMCP = %t, want true", enabled)
					}
					return nil
				},
			}),
		}},
		"user": {actions: []apiAction{userAction()}},
		"notification": {actions: []apiAction{
			standardAction(reflectedSpec{
				name: "notification_providers", factory: notificationFactory,
				createMethod: "CreateProvider", readMethod: "GetProvider",
				updateMethod: "UpdateProvider", deleteMethod: "DeleteProvider",
				readArgs:      func(id string) []any { return []any{defaultUserID, id} },
				updateArgs:    func(_ string, record any) []any { return []any{record} },
				deleteArgs:    func(id string) []any { return []any{defaultUserID, id} },
				booleanFields: []string{"Enabled"},
				updateMutation: func(record any, id string) {
					setStringField(reflectValue(record), "Name", "updated-"+id)
				},
			}),
		}},
		"editor": {actions: []apiAction{
			standardAction(reflectedSpec{
				name: "editors", factory: editorFactory,
				createMethod: "CreateEditor", readMethod: "GetEditorByID",
				updateMethod: "UpdateEditor", deleteMethod: "DeleteEditor",
				populate: func(record any, id string) {
					populateRecord(record, id)
					setStringField(reflectValue(record), "Type", "conformance-editor-"+id)
				},
				booleanFields: []string{"Enabled", "Installed"},
			}),
		}},
		"prompts": {actions: []apiAction{
			standardAction(reflectedSpec{
				name: "custom_prompts", factory: promptFactory,
				createMethod: "CreatePrompt", readMethod: "GetPromptByID",
				updateMethod: "UpdatePrompt", deleteMethod: "DeletePrompt",
			}),
		}},
		"utility": {actions: []apiAction{
			standardAction(reflectedSpec{
				name: "utility_agents", factory: utilityFactory,
				createMethod: "CreateAgent", readMethod: "GetAgentByID",
				updateMethod: "UpdateAgent", deleteMethod: "DeleteAgent",
				booleanFields: []string{"Enabled", "Builtin"},
			}),
		}},
	}

	behaviors["office"] = ownerBehavior{actions: []apiAction{officeAction()}}
	behaviors["terminal"] = ownerBehavior{actions: []apiAction{terminalAction()}}
	behaviors["quick-terminal"] = ownerBehavior{actions: []apiAction{quickTerminalAction()}}
	behaviors["runtime-flags"] = ownerBehavior{actions: []apiAction{runtimeFlagsAction()}}
	behaviors["auth"] = ownerBehavior{actions: []apiAction{authIdentityAction()}}
	behaviors["secrets"] = ownerBehavior{actions: []apiAction{secretAction()}}
	behaviors["system-settings"] = ownerBehavior{actions: []apiAction{systemSettingsAction()}}
	behaviors["auth-hostnames"] = ownerBehavior{actions: []apiAction{hostnameAction()}}
	behaviors["organizations"] = ownerBehavior{actions: []apiAction{organizationAction()}}
	behaviors["organization-units"] = ownerBehavior{actions: []apiAction{organizationUnitAction()}}
	behaviors["message-queue"] = ownerBehavior{actions: []apiAction{messageQueueAction()}}
	behaviors["task-share"] = ownerBehavior{actions: []apiAction{shareAction()}}
	behaviors["telemetry-contract"] = ownerBehavior{actions: []apiAction{telemetryAction()}}
	behaviors["delivery"] = ownerBehavior{actions: []apiAction{deliveryAction()}}
	behaviors["storage"] = ownerBehavior{actions: []apiAction{storageAction()}}
	behaviors["plugin-instances"] = ownerBehavior{actions: []apiAction{pluginInstanceAction()}}
	behaviors["plugin-marketplace"] = ownerBehavior{actions: []apiAction{marketplaceAction()}}
	behaviors["plugin-settings"] = ownerBehavior{actions: []apiAction{pluginSettingsAction()}}
	behaviors["plugin-state"] = ownerBehavior{actions: []apiAction{pluginStateAction()}}
	behaviors["plugin-instance-state"] = ownerBehavior{actions: []apiAction{pluginInstanceStateAction()}}
	behaviors["plugin-user-state"] = ownerBehavior{actions: []apiAction{pluginUserStateAction()}}
	behaviors["canvas"] = ownerBehavior{actions: []apiAction{canvasAction()}}
	behaviors["github"] = ownerBehavior{actions: []apiAction{githubAction()}}
	behaviors["gitlab"] = ownerBehavior{actions: []apiAction{gitlabAction()}}
	behaviors["jira"] = ownerBehavior{actions: []apiAction{jiraAction()}}
	behaviors["linear"] = ownerBehavior{actions: []apiAction{linearAction()}}
	behaviors["sentry"] = ownerBehavior{actions: []apiAction{sentryAction()}}
	behaviors["azure-devops"] = ownerBehavior{actions: []apiAction{azureDevOpsAction()}}
	behaviors["workflow-sync"] = ownerBehavior{actions: []apiAction{workflowSyncAction()}}
	behaviors["office-config-sync"] = ownerBehavior{actions: []apiAction{officeConfigSyncAction()}}
	behaviors["automation"] = ownerBehavior{actions: []apiAction{automationAction()}}

	analytics := standardAction(reflectedSpec{
		name: "analytics-task-view", factory: taskFactory,
		createMethod: "CreateWorkspace", readMethod: "GetWorkspace",
		updateMethod: "UpdateWorkspace", deleteMethod: "DeleteWorkspace",
	})
	baseAnalyticsRead := analytics.read
	analytics.read = func(s testconformance.ScenarioContext, id string) (any, error) {
		stats, err := analyticsRepository(s)
		if err != nil {
			return nil, err
		}
		if _, err := invoke(stats, "GetGlobalStats", s.Context, id, nil); err != nil {
			return nil, fmt.Errorf("analytics read: %w", err)
		}
		return baseAnalyticsRead(s, id)
	}
	behaviors["analytics"] = ownerBehavior{actions: []apiAction{analytics}}

	return behaviors
}

func metaAction() apiAction {
	return apiAction{
		name: "kandev_meta",
		create: func(s testconformance.ScenarioContext, id string) (any, error) {
			if err := persistence.WriteVersion(s.DB, id); err != nil {
				return nil, err
			}
			now := time.Now().UTC()
			return &metaBehaviorRecord{ID: id, Value: id, CreatedAt: now, UpdatedAt: now}, nil
		},
		read: func(s testconformance.ScenarioContext, _ string) (any, error) {
			value, err := persistence.ReadMetaKey(s.DB, "kandev_version")
			if err != nil {
				return nil, err
			}
			if value == "" {
				return nil, fmt.Errorf("meta version is empty")
			}
			return &metaBehaviorRecord{ID: value, Value: value, CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()}, nil
		},
		update: func(s testconformance.ScenarioContext, id string, _ any) error {
			return persistence.WriteVersion(s.DB, id+"-updated")
		},
		delete: func(s testconformance.ScenarioContext, _ string) error {
			return persistence.DeleteKeys(s.DB, "kandev_version")
		},
	}
}

func officeFactory(s testconformance.ScenarioContext) (any, error) {
	return officesqlite.NewWithDB(s.DB, s.DB, logger.Default())
}

func terminalFactory(s testconformance.ScenarioContext) (any, error) {
	return terminalrepo.NewWithDB(s.DB, s.DB, logger.Default())
}

func quickTerminalFactory(s testconformance.ScenarioContext) (any, error) {
	return quickterminalrepo.NewWithDB(s.DB, s.DB)
}

func runtimeFlagsFactory(s testconformance.ScenarioContext) (any, error) {
	return runtimeflags.NewSQLiteStore(s.DB, s.DB)
}

func authFactory(s testconformance.ScenarioContext) (any, error) {
	return authstore.New(s.DB, s.DB)
}

func secretFactory(s testconformance.ScenarioContext) (any, error) {
	keyDir := filepath.Join(os.TempDir(), "kandev-store-conformance", string(s.Engine))
	key, err := secrets.NewMasterKeyProvider(keyDir)
	if err != nil {
		return nil, err
	}
	store, _, err := secrets.Provide(s.DB, s.DB, key)
	return store, err
}

func systemSettingsFactory(s testconformance.ScenarioContext) (any, error) {
	return settings.NewStore(ownerPool(s))
}

func hostnameFactory(s testconformance.ScenarioContext) (any, error) {
	return hostnames.NewStore(s.DB, s.DB)
}

func organizationFactory(s testconformance.ScenarioContext) (any, error) {
	return org.NewStore(ownerPool(s))
}

func organizationUnitFactory(s testconformance.ScenarioContext) (any, error) {
	return orgunit.NewStore(ownerPool(s))
}

func messageQueueFactory(s testconformance.ScenarioContext) (any, error) {
	return messagequeue.NewSQLiteRepository(s.DB, s.DB)
}

func shareFactory(s testconformance.ScenarioContext) (any, error) {
	return share.NewRepository(s.DB, s.DB, logger.Default())
}

func telemetryFactory(s testconformance.ScenarioContext) (any, error) {
	return telemetrycontract.NewWithDB(s.DB, s.DB)
}

func deliveryFactory(s testconformance.ScenarioContext) (any, error) {
	return delivery.NewWithDB(s.DB, s.DB, logger.Default())
}

func storageFactory(s testconformance.ScenarioContext) (any, error) {
	return storagepkg.NewStore(ownerPool(s))
}

func pluginInstancesFactory(s testconformance.ScenarioContext) (any, error) {
	return instances.NewStore(ownerPool(s))
}

func marketplaceFactory(s testconformance.ScenarioContext) (any, error) {
	return marketplace.NewSourceStore(ownerPool(s))
}

func pluginSettingsFactory(s testconformance.ScenarioContext) (any, error) {
	return plugins.NewSettingsStore(ownerPool(s))
}

func pluginStateFactory(s testconformance.ScenarioContext) (any, error) {
	return pluginstate.NewStore(ownerPool(s))
}

func pluginInstanceStateFactory(s testconformance.ScenarioContext) (any, error) {
	return pluginstate.NewInstanceStore(ownerPool(s))
}

func pluginUserStateFactory(s testconformance.ScenarioContext) (any, error) {
	return pluginstate.NewUserStore(ownerPool(s))
}

func canvasFactory(s testconformance.ScenarioContext) (any, error) {
	return canvas.NewRepository(ownerPool(s))
}

func githubFactory(s testconformance.ScenarioContext) (any, error) {
	return github.NewStore(s.DB, s.DB)
}

func gitlabFactory(s testconformance.ScenarioContext) (any, error) {
	return gitlab.NewStore(s.DB, s.DB)
}

func jiraFactory(s testconformance.ScenarioContext) (any, error) {
	return jira.NewStore(s.DB, s.DB)
}

func linearFactory(s testconformance.ScenarioContext) (any, error) {
	return linear.NewStore(s.DB, s.DB)
}

func sentryFactory(s testconformance.ScenarioContext) (any, error) {
	return sentry.NewStore(s.DB, s.DB)
}

func azureDevOpsFactory(s testconformance.ScenarioContext) (any, error) {
	return azuredevops.NewStore(s.DB, s.DB)
}

func workflowSyncFactory(s testconformance.ScenarioContext) (any, error) {
	return workflowsync.NewStore(s.DB, s.DB)
}

func officeConfigSyncFactory(s testconformance.ScenarioContext) (any, error) {
	return configsync.NewStore(s.DB, s.DB)
}

func automationFactory(s testconformance.ScenarioContext) (any, error) {
	return automation.NewStore(s.DB, s.DB)
}

func userAction() apiAction {
	action := standardAction(reflectedSpec{
		name: "users", factory: userFactory,
		createMethod: "CreateUser", readMethod: "GetUser",
		updateMethod: "UpdateUserProfile", deleteMethod: "DeleteUser",
		updateArgs: func(id string, _ any) []any {
			return []any{id, id + "-updated@example.test", "Updated User", "member"}
		},
	})
	action.assertUpdated = func(_ any, after any, _ string) error {
		name, nameOK := recordStringField(after, "DisplayName")
		role, roleOK := recordStringField(after, "Role")
		if !nameOK || !roleOK || name != "Updated User" || role != "member" {
			return fmt.Errorf("updated user = %#v, want member/Updated User", after)
		}
		return nil
	}
	action.setBoolean = func(s testconformance.ScenarioContext, id string, _ any, enabled bool) (any, error) {
		store, err := userFactory(s)
		if err != nil {
			return nil, err
		}
		if _, err := invoke(store, "SetOperator", s.Context, id, enabled); err != nil {
			return nil, err
		}
		return action.read(s, id)
	}
	action.readBoolean = func(record any) (bool, bool) {
		return recordBoolean(record, []string{"IsOperator"})
	}
	return action
}

//nolint:cyclop,gocognit,funlen // This adapter asserts the complete task repository contract.
func taskAction() apiAction {
	action := apiAction{name: "tasks", key: recordIdentifier}
	createTask := func(s testconformance.ScenarioContext, id string, ephemeral bool) (any, error) {
		store, err := taskFactory(s)
		if err != nil {
			return nil, err
		}
		workspaceID, err := conformanceWorkspaceID(s)
		if err != nil {
			return nil, err
		}
		task := &taskmodels.Task{
			ID: id, WorkspaceID: workspaceID, Title: "Conformance task",
			State: v1.TaskStateTODO, Priority: "medium", Labels: "[]",
			Metadata: map[string]interface{}{}, IsEphemeral: ephemeral,
		}
		if err := invokeError(store, "CreateTask", s.Context, task); err != nil {
			return nil, err
		}
		return task, nil
	}
	action.create = func(s testconformance.ScenarioContext, id string) (any, error) {
		return createTask(s, id, false)
	}
	action.read = func(s testconformance.ScenarioContext, id string) (any, error) {
		store, err := taskFactory(s)
		if err != nil {
			return nil, err
		}
		return invokeRecord(store, "GetTask", s.Context, id)
	}
	action.update = func(s testconformance.ScenarioContext, id string, _ any) error {
		store, err := taskFactory(s)
		if err != nil {
			return err
		}
		value, err := invokeRecord(store, "GetTask", s.Context, id)
		if err != nil {
			return err
		}
		task, ok := value.(*taskmodels.Task)
		if !ok {
			return fmt.Errorf("GetTask returned %T", value)
		}
		task.Title = "Updated " + id
		return invokeError(store, "UpdateTask", s.Context, task)
	}
	action.assertUpdated = func(_ any, after any, id string) error {
		task, ok := after.(*taskmodels.Task)
		if !ok || task == nil {
			return fmt.Errorf("GetTask returned %T", after)
		}
		if task.Title != "Updated "+id {
			return fmt.Errorf("task title = %q, want %q", task.Title, "Updated "+id)
		}
		return nil
	}
	action.delete = func(s testconformance.ScenarioContext, id string) error {
		store, err := taskFactory(s)
		if err != nil {
			return err
		}
		return invokeError(store, "DeleteTask", s.Context, id)
	}
	action.setBoolean = func(s testconformance.ScenarioContext, id string, _ any, enabled bool) (any, error) {
		store, err := taskFactory(s)
		if err != nil {
			return nil, err
		}
		// IsEphemeral is immutable after creation. Recreate the valid task through
		// the owning repository so both persisted boolean values are exercised.
		if err := invokeError(store, "DeleteTask", s.Context, id); err != nil {
			return nil, err
		}
		if _, err := createTask(s, id, enabled); err != nil {
			return nil, err
		}
		return invokeRecord(store, "GetTask", s.Context, id)
	}
	action.readBoolean = func(record any) (bool, bool) {
		task, ok := record.(*taskmodels.Task)
		return ok && task.IsEphemeral, ok
	}
	action.conflict = func(s testconformance.ScenarioContext, id string, _ any) error {
		store, err := taskFactory(s)
		if err != nil {
			return err
		}
		workspaceID, err := conformanceWorkspaceID(s)
		if err != nil {
			return err
		}
		duplicate := &taskmodels.Task{ID: id, WorkspaceID: workspaceID, Title: "Duplicate", State: v1.TaskStateTODO, Priority: "medium", Labels: "[]", Metadata: map[string]interface{}{}}
		if err := invokeError(store, "CreateTask", s.Context, duplicate); err == nil {
			return fmt.Errorf("duplicate task succeeded")
		}
		return nil
	}
	action.transaction = func(s testconformance.ScenarioContext, id string) error {
		return transactionAPICheck(action, s, id)
	}
	return action
}

func officeAction() apiAction {
	action := standardAction(reflectedSpec{
		name: "office_projects", factory: officeFactory,
		createMethod: "CreateProject", readMethod: "GetProject",
		updateMethod: "UpdateProject", deleteMethod: "DeleteProject",
	})
	action.setBoolean = func(s testconformance.ScenarioContext, id string, _ any, active bool) (any, error) {
		store, err := officeFactory(s)
		if err != nil {
			return nil, err
		}
		value, err := invokeRecord(store, "GetProject", s.Context, id)
		if err != nil {
			return nil, err
		}
		project, ok := value.(*officemodels.Project)
		if !ok {
			return nil, fmt.Errorf("GetProject returned %T", value)
		}
		if active {
			project.Status = officemodels.ProjectStatusActive
		} else {
			project.Status = officemodels.ProjectStatusArchived
		}
		if err := action.update(s, id, project); err != nil {
			return nil, err
		}
		return action.read(s, id)
	}
	action.readBoolean = func(record any) (bool, bool) {
		project, ok := record.(*officemodels.Project)
		return ok && project.Status == officemodels.ProjectStatusActive, ok
	}
	return action
}

func terminalAction() apiAction {
	action := standardAction(reflectedSpec{
		name:         "user_terminals",
		factory:      terminalFactory,
		createMethod: "Create",
		readMethod:   "Get",
		updateMethod: "Rename",
		deleteMethod: "Delete",
		createArgs: func(id string, _ any) []any {
			return []any{"conformance-task", "conformance-environment", id, "echo conformance"}
		},
		updateArgs: func(id string, _ any) []any {
			name := "updated terminal"
			return []any{id, &name}
		},
	})
	action.assertUpdated = func(_ any, after any, _ string) error {
		name, ok := recordStringField(after, "CustomName")
		if !ok || name != "updated terminal" {
			return fmt.Errorf("terminal custom name = %q, want %q", name, "updated terminal")
		}
		return nil
	}
	action.setBoolean = func(s testconformance.ScenarioContext, id string, _ any, open bool) (any, error) {
		state := "parked"
		if open {
			state = "open"
		}
		store, err := terminalFactory(s)
		if err != nil {
			return nil, err
		}
		if _, err := invoke(store, "SetState", s.Context, id, state); err != nil {
			return nil, err
		}
		return action.read(s, id)
	}
	action.readBoolean = func(record any) (bool, bool) {
		value := reflect.ValueOf(record)
		if value.Kind() == reflect.Pointer && !value.IsNil() {
			value = value.Elem()
		}
		field := value.FieldByName("State")
		if !field.IsValid() || field.Kind() != reflect.String {
			return false, false
		}
		return field.String() == "open", true
	}
	return action
}

func quickTerminalAction() apiAction {
	action := standardAction(reflectedSpec{
		name:         "quick_terminal_tabs",
		factory:      quickTerminalFactory,
		createMethod: "Create",
		readMethod:   "Get",
		updateMethod: "UpdateLifecycle",
		deleteMethod: "Delete",
		createArgs: func(id string, _ any) []any {
			return []any{defaultUserID, "conformance-workspace", id}
		},
		readArgs: func(id string) []any {
			return []any{defaultUserID, id}
		},
		updateArgs: func(id string, _ any) []any {
			return []any{defaultUserID, id, "conformance-session", "closed", nil, ""}
		},
		deleteArgs: func(id string) []any {
			return []any{defaultUserID, id}
		},
	})
	action.conflict = func(s testconformance.ScenarioContext, id string, _ any) error {
		store, err := quickTerminalFactory(s)
		if err != nil {
			return err
		}
		if _, err := invoke(store, "Create", s.Context, "another-user", "conformance-workspace", id); err == nil {
			return fmt.Errorf("duplicate quick terminal tab succeeded")
		}
		return nil
	}
	return action
}

type runtimeFlagRecord struct {
	ID        string
	Enabled   bool
	CreatedAt time.Time
	UpdatedAt time.Time
}

//nolint:funlen // This adapter enumerates every runtime-flag API assertion.
func runtimeFlagsAction() apiAction {
	action := apiAction{name: "runtime_flag_overrides", key: recordIdentifier}
	action.create = func(s testconformance.ScenarioContext, id string) (any, error) {
		store, err := runtimeFlagsFactory(s)
		if err != nil {
			return nil, err
		}
		if err := store.(*runtimeflags.SQLiteStore).SetOverride(s.Context, id, false); err != nil {
			return nil, err
		}
		override, err := store.(*runtimeflags.SQLiteStore).GetOverride(s.Context, id)
		if err != nil {
			return nil, err
		}
		return &runtimeFlagRecord{ID: override.Key, Enabled: override.Value, CreatedAt: override.CreatedAt, UpdatedAt: override.UpdatedAt}, nil
	}
	action.read = func(s testconformance.ScenarioContext, id string) (any, error) {
		store, err := runtimeFlagsFactory(s)
		if err != nil {
			return nil, err
		}
		override, err := store.(*runtimeflags.SQLiteStore).GetOverride(s.Context, id)
		if err != nil {
			return nil, err
		}
		return &runtimeFlagRecord{ID: override.Key, Enabled: override.Value, CreatedAt: override.CreatedAt, UpdatedAt: override.UpdatedAt}, nil
	}
	action.update = func(s testconformance.ScenarioContext, id string, record any) error {
		enabled, _ := recordBoolean(record, nil)
		return runtimeFlagsFactorySet(s, id, !enabled)
	}
	action.delete = func(s testconformance.ScenarioContext, id string) error {
		store, err := runtimeFlagsFactory(s)
		if err != nil {
			return err
		}
		return store.(*runtimeflags.SQLiteStore).DeleteOverride(s.Context, id)
	}
	action.assertDeleted = func(s testconformance.ScenarioContext, id string, _ any) error {
		store, err := runtimeFlagsFactory(s)
		if err != nil {
			return err
		}
		overrides, err := store.(*runtimeflags.SQLiteStore).ListOverrides(s.Context)
		if err != nil {
			return err
		}
		if _, ok := overrides[id]; ok {
			return fmt.Errorf("runtime flag %q remains after delete", id)
		}
		return nil
	}
	action.setBoolean = func(s testconformance.ScenarioContext, id string, _ any, enabled bool) (any, error) {
		if err := runtimeFlagsFactorySet(s, id, enabled); err != nil {
			return nil, err
		}
		return action.read(s, id)
	}
	action.readBoolean = func(record any) (bool, bool) {
		value, ok := record.(*runtimeFlagRecord)
		if !ok {
			return false, false
		}
		return value.Enabled, true
	}
	action.conflict = func(s testconformance.ScenarioContext, id string, _ any) error {
		if err := runtimeFlagsFactorySet(s, id, true); err != nil {
			return err
		}
		read, err := action.read(s, id)
		if err != nil {
			return err
		}
		if enabled, _ := action.readBoolean(read); !enabled {
			return fmt.Errorf("runtime flag upsert did not resolve conflict")
		}
		return nil
	}
	return action
}

func runtimeFlagsFactorySet(s testconformance.ScenarioContext, id string, enabled bool) error {
	store, err := runtimeFlagsFactory(s)
	if err != nil {
		return err
	}
	return store.(*runtimeflags.SQLiteStore).SetOverride(s.Context, id, enabled)
}

func authIdentityAction() apiAction {
	action := apiAction{name: "auth_identities"}
	action.create = func(s testconformance.ScenarioContext, id string) (any, error) {
		store, err := authFactory(s)
		if err != nil {
			return nil, err
		}
		identity := &authstore.LoginIdentity{
			ID: id, UserID: defaultUserID, Provider: authstore.ProviderLocal,
			Subject: id, PasswordHash: "initial-hash",
		}
		if err := store.(*authstore.Store).CreateIdentity(s.Context, identity); err != nil {
			return nil, err
		}
		return identity, nil
	}
	action.read = func(s testconformance.ScenarioContext, _ string) (any, error) {
		store, err := authFactory(s)
		if err != nil {
			return nil, err
		}
		return store.(*authstore.Store).GetLocalIdentity(s.Context, defaultUserID)
	}
	action.update = func(s testconformance.ScenarioContext, _ string, _ any) error {
		store, err := authFactory(s)
		if err != nil {
			return err
		}
		return store.(*authstore.Store).UpdatePasswordHash(s.Context, defaultUserID, "updated-hash")
	}
	action.delete = func(s testconformance.ScenarioContext, id string) error {
		store, err := authFactory(s)
		if err != nil {
			return err
		}
		return store.(*authstore.Store).DeleteIdentity(s.Context, id)
	}
	action.setBoolean = func(s testconformance.ScenarioContext, _ string, _ any, enabled bool) (any, error) {
		store, err := authFactory(s)
		if err != nil {
			return nil, err
		}
		hash := ""
		if enabled {
			hash = "enabled-hash"
		}
		if err := store.(*authstore.Store).UpdatePasswordHash(s.Context, defaultUserID, hash); err != nil {
			return nil, err
		}
		return action.read(s, "")
	}
	action.readBoolean = func(record any) (bool, bool) {
		identity, ok := record.(*authstore.LoginIdentity)
		if !ok {
			return false, false
		}
		return identity.PasswordHash != "", true
	}
	action.conflict = func(s testconformance.ScenarioContext, id string, _ any) error {
		store, err := authFactory(s)
		if err != nil {
			return err
		}
		duplicate := &authstore.LoginIdentity{ID: id, UserID: defaultUserID, Provider: authstore.ProviderLocal, Subject: id, PasswordHash: "duplicate-hash"}
		if err := store.(*authstore.Store).CreateIdentity(s.Context, duplicate); err == nil {
			return fmt.Errorf("duplicate auth identity succeeded")
		}
		return nil
	}
	action.transaction = func(s testconformance.ScenarioContext, id string) error {
		return transactionAPICheck(action, s, id)
	}
	return action
}

func secretAction() apiAction {
	action := apiAction{name: "secrets"}
	action.create = func(s testconformance.ScenarioContext, id string) (any, error) {
		store, err := secretFactory(s)
		if err != nil {
			return nil, err
		}
		secret := &secrets.SecretWithValue{
			Secret: secrets.Secret{ID: id, Name: "secret-" + id, Scope: secrets.ScopeGlobal},
			Value:  "conformance-secret-value",
		}
		if err := store.(secrets.SecretStore).Create(s.Context, secret); err != nil {
			return nil, err
		}
		return secret, nil
	}
	action.read = func(s testconformance.ScenarioContext, id string) (any, error) {
		store, err := secretFactory(s)
		if err != nil {
			return nil, err
		}
		return store.(secrets.SecretStore).Get(s.Context, id)
	}
	action.update = func(s testconformance.ScenarioContext, id string, _ any) error {
		store, err := secretFactory(s)
		if err != nil {
			return err
		}
		name := "updated-secret-" + id
		return store.(secrets.SecretStore).Update(s.Context, id, &secrets.UpdateSecretRequest{Name: &name})
	}
	action.delete = func(s testconformance.ScenarioContext, id string) error {
		store, err := secretFactory(s)
		if err != nil {
			return err
		}
		return store.(secrets.SecretStore).Delete(s.Context, id)
	}
	action.conflict = func(s testconformance.ScenarioContext, id string, _ any) error {
		store, err := secretFactory(s)
		if err != nil {
			return err
		}
		duplicate := &secrets.SecretWithValue{
			Secret: secrets.Secret{ID: id, Name: "duplicate-" + id, Scope: secrets.ScopeGlobal},
			Value:  "duplicate-value",
		}
		if err := store.(secrets.SecretStore).Create(s.Context, duplicate); err == nil {
			return fmt.Errorf("duplicate secret succeeded")
		}
		return nil
	}
	action.transaction = func(s testconformance.ScenarioContext, id string) error {
		return transactionAPICheck(action, s, id)
	}
	return action
}

type settingBehaviorRecord struct {
	ID        string
	Value     []byte
	CreatedAt time.Time
	UpdatedAt time.Time
}

func systemSettingsAction() apiAction {
	action := apiAction{name: "settings"}
	action.create = func(s testconformance.ScenarioContext, id string) (any, error) {
		store, err := systemSettingsFactory(s)
		if err != nil {
			return nil, err
		}
		value := []byte("initial-" + id)
		if err := store.(*settings.Store).Save(s.Context, id, value); err != nil {
			return nil, err
		}
		entry, found, err := store.(*settings.Store).GetEntry(s.Context, id)
		if err != nil {
			return nil, err
		}
		if !found {
			return nil, fmt.Errorf("setting %q was not persisted", id)
		}
		return &settingBehaviorRecord{ID: entry.Key, Value: entry.Value, CreatedAt: entry.UpdatedAt, UpdatedAt: entry.UpdatedAt}, nil
	}
	action.read = func(s testconformance.ScenarioContext, id string) (any, error) {
		store, err := systemSettingsFactory(s)
		if err != nil {
			return nil, err
		}
		entry, found, err := store.(*settings.Store).GetEntry(s.Context, id)
		if err != nil {
			return nil, err
		}
		if !found {
			return nil, fmt.Errorf("setting %q not found", id)
		}
		return &settingBehaviorRecord{ID: entry.Key, Value: entry.Value, CreatedAt: entry.UpdatedAt, UpdatedAt: entry.UpdatedAt}, nil
	}
	action.update = func(s testconformance.ScenarioContext, id string, _ any) error {
		store, err := systemSettingsFactory(s)
		if err != nil {
			return err
		}
		return store.(*settings.Store).Save(s.Context, id, []byte("updated-"+id))
	}
	action.delete = func(s testconformance.ScenarioContext, id string) error {
		store, err := systemSettingsFactory(s)
		if err != nil {
			return err
		}
		return store.(*settings.Store).Delete(s.Context, id)
	}
	action.assertDeleted = func(s testconformance.ScenarioContext, id string, _ any) error {
		store, err := systemSettingsFactory(s)
		if err != nil {
			return err
		}
		_, found, err := store.(*settings.Store).Get(s.Context, id)
		if err != nil {
			return err
		}
		if found {
			return fmt.Errorf("setting %q remains after delete", id)
		}
		return nil
	}
	action.conflict = func(s testconformance.ScenarioContext, id string, _ any) error {
		return action.update(s, id, nil)
	}
	return action
}

func analyticsRepository(s testconformance.ScenarioContext) (any, error) {
	// The analytics owner is intentionally read-only. Its table is task-owned,
	// so the adapter performs the write lifecycle through the task repository
	// and proves the analytics repository can read the resulting task scope.
	return analyticsFactory(s)
}

func analyticsFactory(s testconformance.ScenarioContext) (any, error) {
	repo, _, err := analyticsrepository.Provide(s.DB, s.DB)
	return repo, err
}

type hostnameBehaviorRecord struct {
	ID         string
	Hostname   string
	ResolvedAt time.Time
}

func hostnameAction() apiAction {
	action := apiAction{name: "hostname_cache", key: func(record any, fallback string) string {
		if value, ok := record.(*hostnameBehaviorRecord); ok {
			return value.ID
		}
		return fallback
	}}
	action.create = func(s testconformance.ScenarioContext, id string) (any, error) {
		store, err := hostnameFactory(s)
		if err != nil {
			return nil, err
		}
		at := time.Date(2024, 1, 2, 3, 4, 5, 0, time.UTC)
		if err := store.(*hostnames.Store).Set(s.Context, id, "host-"+id, at); err != nil {
			return nil, err
		}
		return &hostnameBehaviorRecord{ID: id, Hostname: "host-" + id, ResolvedAt: at}, nil
	}
	action.read = func(s testconformance.ScenarioContext, id string) (any, error) {
		store, err := hostnameFactory(s)
		if err != nil {
			return nil, err
		}
		entry, err := store.(*hostnames.Store).Get(s.Context, id)
		if err != nil {
			return nil, err
		}
		if entry.ResolvedAt == nil {
			return nil, fmt.Errorf("hostname %q has no resolved timestamp", id)
		}
		return &hostnameBehaviorRecord{ID: id, Hostname: entry.Hostname, ResolvedAt: entry.ResolvedAt.UTC()}, nil
	}
	action.update = func(s testconformance.ScenarioContext, id string, _ any) error {
		store, err := hostnameFactory(s)
		if err != nil {
			return err
		}
		return store.(*hostnames.Store).Set(s.Context, id, "updated-"+id, time.Date(2024, 1, 2, 4, 4, 5, 0, time.UTC))
	}
	action.delete = func(s testconformance.ScenarioContext, id string) error {
		store, err := hostnameFactory(s)
		if err != nil {
			return err
		}
		return store.(*hostnames.Store).Delete(s.Context, id)
	}
	action.conflict = func(s testconformance.ScenarioContext, id string, _ any) error {
		return action.update(s, id, nil)
	}
	return action
}

func organizationAction() apiAction {
	action := apiAction{name: "organizations", key: func(record any, fallback string) string {
		if value, ok := record.(*org.Org); ok {
			return value.ID
		}
		return fallback
	}}
	action.create = func(s testconformance.ScenarioContext, id string) (any, error) {
		store, err := organizationFactory(s)
		if err != nil {
			return nil, err
		}
		return store.(*org.Store).Create(s.Context, "Conformance "+id, id, false)
	}
	action.read = func(s testconformance.ScenarioContext, id string) (any, error) {
		store, err := organizationFactory(s)
		if err != nil {
			return nil, err
		}
		return store.(*org.Store).Get(s.Context, id)
	}
	action.update = func(s testconformance.ScenarioContext, id string, _ any) error {
		store, err := organizationFactory(s)
		if err != nil {
			return err
		}
		_, err = store.(*org.Store).UpdateNameStatus(s.Context, id, "Updated "+id, string(org.StatusActive))
		return err
	}
	action.delete = func(s testconformance.ScenarioContext, id string) error {
		store, err := organizationFactory(s)
		if err != nil {
			return err
		}
		return store.(*org.Store).Delete(s.Context, id)
	}
	action.setBoolean = func(s testconformance.ScenarioContext, id string, _ any, active bool) (any, error) {
		store, err := organizationFactory(s)
		if err != nil {
			return nil, err
		}
		status := string(org.StatusSuspended)
		if active {
			status = string(org.StatusActive)
		}
		if _, err := store.(*org.Store).UpdateNameStatus(s.Context, id, "Boolean "+id, status); err != nil {
			return nil, err
		}
		return action.read(s, id)
	}
	action.readBoolean = func(record any) (bool, bool) {
		value, ok := record.(*org.Org)
		return ok && value.Active(), ok
	}
	action.conflict = func(s testconformance.ScenarioContext, id string, record any) error {
		store, err := organizationFactory(s)
		if err != nil {
			return err
		}
		slug := id
		if value, ok := record.(*org.Org); ok {
			slug = value.Slug
		}
		if _, err := store.(*org.Store).Create(s.Context, "Duplicate "+id, slug, false); err == nil {
			return fmt.Errorf("duplicate organization slug succeeded")
		}
		return nil
	}
	return action
}

func organizationUnitAction() apiAction {
	action := apiAction{name: "organization_units", key: func(record any, fallback string) string {
		if value, ok := record.(*orgunit.Unit); ok {
			return value.ID
		}
		return fallback
	}}
	createOrg := func(s testconformance.ScenarioContext, id string) (*org.Org, error) {
		store, err := organizationFactory(s)
		if err != nil {
			return nil, err
		}
		return store.(*org.Store).Create(s.Context, "Unit Org "+id, "unit-org-"+id, false)
	}
	action.create = func(s testconformance.ScenarioContext, id string) (any, error) {
		organization, err := createOrg(s, id)
		if err != nil {
			return nil, err
		}
		store, err := organizationUnitFactory(s)
		if err != nil {
			return nil, err
		}
		unit, err := store.(*orgunit.Store).Insert(s.Context, &orgunit.Unit{
			ID: id, OrgID: organization.ID, Kind: orgunit.KindStandard, Name: "Unit " + id,
		})
		if err != nil {
			return nil, err
		}
		return unit, nil
	}
	action.read = func(s testconformance.ScenarioContext, id string) (any, error) {
		store, err := organizationUnitFactory(s)
		if err != nil {
			return nil, err
		}
		return store.(*orgunit.Store).Get(s.Context, id)
	}
	action.update = func(s testconformance.ScenarioContext, id string, _ any) error {
		store, err := organizationUnitFactory(s)
		if err != nil {
			return err
		}
		return store.(*orgunit.Store).Rename(s.Context, id, "Updated "+id)
	}
	action.delete = func(s testconformance.ScenarioContext, id string) error {
		store, err := organizationUnitFactory(s)
		if err != nil {
			return err
		}
		return store.(*orgunit.Store).Delete(s.Context, id)
	}
	action.conflict = func(s testconformance.ScenarioContext, id string, _ any) error {
		store, err := organizationUnitFactory(s)
		if err != nil {
			return err
		}
		if _, err := store.(*orgunit.Store).Insert(s.Context, &orgunit.Unit{ID: id, OrgID: "duplicate", Kind: orgunit.KindStandard, Name: "Duplicate"}); err == nil {
			return fmt.Errorf("duplicate organization unit succeeded")
		}
		return nil
	}
	return action
}

func conformanceWorkspaceID(s testconformance.ScenarioContext) (string, error) {
	repo, err := taskFactory(s)
	if err != nil {
		return "", err
	}
	outputs, err := invoke(repo, "ListWorkspaces", s.Context)
	if err != nil {
		return "", err
	}
	if len(outputs) == 0 {
		return "", fmt.Errorf("ListWorkspaces returned no values")
	}
	value := outputs[0].Interface()
	items := reflect.ValueOf(value)
	if items.Kind() != reflect.Slice || items.Len() == 0 {
		return "", fmt.Errorf("task schema has no workspace")
	}
	return recordIdentifier(items.Index(0).Interface(), ""), nil
}

func ensureConformanceTaskAndSession(s testconformance.ScenarioContext, id string) (string, string, error) {
	workspaceID, err := conformanceWorkspaceID(s)
	if err != nil {
		return "", "", err
	}
	repo, err := taskFactory(s)
	if err != nil {
		return "", "", err
	}
	taskID := id + "-task"
	if _, err := invoke(repo, "CreateTask", s.Context, &taskmodels.Task{
		ID: taskID, WorkspaceID: workspaceID, Title: "Conformance task", IsEphemeral: true,
	}); err != nil {
		return "", "", err
	}
	sessionID := id + "-session"
	now := time.Now().UTC()
	if _, err := invoke(repo, "CreateTaskSession", s.Context, &taskmodels.TaskSession{
		ID: sessionID, TaskID: taskID, State: taskmodels.TaskSessionStateCreated, StartedAt: now, UpdatedAt: now,
	}); err != nil {
		return "", "", err
	}
	return taskID, sessionID, nil
}

//nolint:cyclop,gocognit,funlen // This adapter asserts the complete message-queue contract.
func messageQueueAction() apiAction {
	type queueRecord struct {
		ID        string
		SessionID string
		TaskID    string
		Content   string
		QueuedAt  time.Time
		PlanMode  bool
	}
	action := apiAction{name: "queued_messages", key: func(record any, fallback string) string {
		if value, ok := record.(*queueRecord); ok {
			return value.ID
		}
		return fallback
	}}
	action.create = func(s testconformance.ScenarioContext, id string) (any, error) {
		taskID, sessionID, err := ensureConformanceTaskAndSession(s, id)
		if err != nil {
			return nil, err
		}
		store, err := messageQueueFactory(s)
		if err != nil {
			return nil, err
		}
		msg := &messagequeue.QueuedMessage{ID: id, SessionID: sessionID, TaskID: taskID, Content: "initial", Model: "model", QueuedBy: "conformance", Metadata: map[string]interface{}{}}
		if err := store.(messagequeue.Repository).Insert(s.Context, msg, 10); err != nil {
			return nil, err
		}
		return &queueRecord{ID: msg.ID, SessionID: msg.SessionID, TaskID: msg.TaskID, Content: msg.Content, QueuedAt: msg.QueuedAt, PlanMode: msg.PlanMode}, nil
	}
	findQueueMessage := func(s testconformance.ScenarioContext, id string) (*messagequeue.QueuedMessage, error) {
		store, err := messageQueueFactory(s)
		if err != nil {
			return nil, err
		}
		sessionID := id + "-session"
		entries, err := store.(messagequeue.Repository).ListBySession(s.Context, sessionID)
		if err != nil {
			return nil, err
		}
		for index := range entries {
			if entries[index].ID == id {
				return &entries[index], nil
			}
		}
		return nil, fmt.Errorf("queued message %q not found", id)
	}
	action.read = func(s testconformance.ScenarioContext, id string) (any, error) {
		msg, err := findQueueMessage(s, id)
		if err != nil {
			return nil, err
		}
		return &queueRecord{ID: msg.ID, SessionID: msg.SessionID, TaskID: msg.TaskID, Content: msg.Content, QueuedAt: msg.QueuedAt, PlanMode: msg.PlanMode}, nil
	}
	action.update = func(s testconformance.ScenarioContext, id string, _ any) error {
		store, err := messageQueueFactory(s)
		if err != nil {
			return err
		}
		return store.(messagequeue.Repository).UpdateContent(s.Context, id+"-session", id, "updated", nil, "conformance")
	}
	action.assertUpdated = func(_ any, after any, _ string) error {
		message, ok := after.(*queueRecord)
		if !ok || message == nil || message.Content != "updated" {
			return fmt.Errorf("queued message after update = %#v, want content updated", after)
		}
		return nil
	}
	action.delete = func(s testconformance.ScenarioContext, id string) error {
		store, err := messageQueueFactory(s)
		if err != nil {
			return err
		}
		return store.(messagequeue.Repository).DeleteByID(s.Context, id+"-session", id)
	}
	action.setBoolean = func(s testconformance.ScenarioContext, id string, _ any, enabled bool) (any, error) {
		store, err := messageQueueFactory(s)
		if err != nil {
			return nil, err
		}
		if err := store.(messagequeue.Repository).SetAutoRun(s.Context, id+"-session", enabled); err != nil {
			return nil, err
		}
		return &queueRecord{ID: id, SessionID: id + "-session", PlanMode: enabled}, nil
	}
	action.readBoolean = func(record any) (bool, bool) {
		value, ok := record.(*queueRecord)
		return value != nil && value.PlanMode, ok
	}
	action.conflict = func(s testconformance.ScenarioContext, id string, _ any) error {
		store, err := messageQueueFactory(s)
		if err != nil {
			return err
		}
		msg := &messagequeue.QueuedMessage{ID: id, SessionID: id + "-session", TaskID: id + "-task", Content: "duplicate", QueuedBy: "conformance"}
		if err := store.(messagequeue.Repository).Insert(s.Context, msg, 10); err == nil {
			return fmt.Errorf("duplicate queued message succeeded")
		}
		return nil
	}
	action.transaction = func(s testconformance.ScenarioContext, id string) error {
		return transactionAPICheck(action, s, id)
	}
	return action
}

func shareAction() apiAction {
	action := apiAction{name: "task_shares", nullableTimestampUpdates: []string{"RevokedAt"}, key: recordIdentifier}
	action.create = func(s testconformance.ScenarioContext, id string) (any, error) {
		store, err := shareFactory(s)
		if err != nil {
			return nil, err
		}
		value := &share.Share{ID: id, TaskSessionID: id + "-session", Backend: share.BackendGitHubGist, ExternalID: id + "-external", ExternalURL: "https://example.test/share/" + id, SnapshotSizeBytes: 12}
		if err := store.(*share.Repository).Create(s.Context, value); err != nil {
			return nil, err
		}
		return value, nil
	}
	action.read = func(s testconformance.ScenarioContext, id string) (any, error) {
		store, err := shareFactory(s)
		if err != nil {
			return nil, err
		}
		return store.(*share.Repository).GetByID(s.Context, id)
	}
	action.update = func(s testconformance.ScenarioContext, id string, _ any) error {
		store, err := shareFactory(s)
		if err != nil {
			return err
		}
		return store.(*share.Repository).MarkRevoked(s.Context, id, time.Now().UTC())
	}
	action.delete = func(s testconformance.ScenarioContext, id string) error {
		return action.update(s, id, nil)
	}
	action.assertDeleted = func(s testconformance.ScenarioContext, id string, _ any) error {
		value, err := action.read(s, id)
		if err != nil {
			return err
		}
		if !value.(*share.Share).IsRevoked() {
			return fmt.Errorf("share %q is not revoked", id)
		}
		return nil
	}
	action.conflict = func(s testconformance.ScenarioContext, id string, _ any) error {
		store, err := shareFactory(s)
		if err != nil {
			return err
		}
		if err := store.(*share.Repository).Create(s.Context, &share.Share{ID: id, TaskSessionID: id + "-session", Backend: share.BackendGitHubGist, ExternalID: "duplicate", ExternalURL: "https://example.test/duplicate"}); err == nil {
			return fmt.Errorf("duplicate share succeeded")
		}
		return nil
	}
	action.transaction = func(s testconformance.ScenarioContext, id string) error {
		return transactionAPICheck(action, s, id)
	}
	return action
}

func telemetryAction() apiAction {
	contract := telemetrycontract.Registry()[0]
	action := apiAction{name: "telemetry_activations", key: func(_ any, _ string) string { return contract.Key }}
	action.create = func(s testconformance.ScenarioContext, _ string) (any, error) {
		store, err := telemetryFactory(s)
		if err != nil {
			return nil, err
		}
		if err := store.(*telemetrycontract.Store).Activate(s.Context); err != nil {
			return nil, err
		}
		return activationRecord(s, contract)
	}
	action.read = func(s testconformance.ScenarioContext, _ string) (any, error) {
		return activationRecord(s, contract)
	}
	action.update = func(s testconformance.ScenarioContext, _ string, _ any) error {
		store, err := telemetryFactory(s)
		if err != nil {
			return err
		}
		if err := store.(*telemetrycontract.Store).DeleteActivation(s.Context, contract.Key, contract.Version); err != nil {
			return err
		}
		return store.(*telemetrycontract.Store).Activate(s.Context)
	}
	action.assertUpdated = func(before, after any, _ string) error {
		beforeRecord, beforeOK := before.(*activationBehaviorRecord)
		afterRecord, afterOK := after.(*activationBehaviorRecord)
		if !beforeOK || !afterOK || beforeRecord == nil || afterRecord == nil {
			return fmt.Errorf("activation records have unexpected types: before=%T after=%T", before, after)
		}
		if !afterRecord.CreatedAt.After(beforeRecord.CreatedAt) {
			return fmt.Errorf("activation timestamp = %v, want later than %v", afterRecord.CreatedAt, beforeRecord.CreatedAt)
		}
		return nil
	}
	action.delete = func(s testconformance.ScenarioContext, _ string) error {
		store, err := telemetryFactory(s)
		if err != nil {
			return err
		}
		return store.(*telemetrycontract.Store).DeleteActivation(s.Context, contract.Key, contract.Version)
	}
	action.assertDeleted = func(s testconformance.ScenarioContext, _ string, _ any) error {
		_, err := activationRecord(s, contract)
		if err == nil {
			return fmt.Errorf("telemetry activation remains after delete")
		}
		return nil
	}
	action.conflict = func(s testconformance.ScenarioContext, _ string, _ any) error {
		store, err := telemetryFactory(s)
		if err != nil {
			return err
		}
		return store.(*telemetrycontract.Store).Activate(s.Context)
	}
	return action
}

type activationBehaviorRecord struct {
	ID        string
	CreatedAt time.Time
	UpdatedAt time.Time
}

func activationRecord(s testconformance.ScenarioContext, contract telemetrycontract.Contract) (any, error) {
	store, err := telemetryFactory(s)
	if err != nil {
		return nil, err
	}
	activation, err := store.(*telemetrycontract.Store).GetActivation(s.Context, contract.Key, contract.Version)
	if err != nil {
		return nil, err
	}
	return &activationBehaviorRecord{ID: activation.Key, CreatedAt: activation.ActivatedAt.UTC(), UpdatedAt: activation.ActivatedAt.UTC()}, nil
}

//nolint:cyclop,gocognit,funlen // This adapter asserts the complete delivery contract.
func deliveryAction() apiAction {
	type deliveryRecord struct {
		ID           string
		TaskID       string
		RepositoryID string
		CreatedAt    time.Time
		UpdatedAt    time.Time
	}
	action := apiAction{name: "task_delivery_ledger", key: func(record any, fallback string) string {
		if value, ok := record.(*deliveryRecord); ok {
			return value.ID
		}
		return fallback
	}}
	prepare := func(s testconformance.ScenarioContext, id string) (string, string, string, error) {
		workspaceID, err := conformanceWorkspaceID(s)
		if err != nil {
			return "", "", "", err
		}
		repo, err := taskFactory(s)
		if err != nil {
			return "", "", "", err
		}
		taskID, repositoryID := id+"-task", id+"-repository"
		if _, err := invoke(repo, "CreateTask", s.Context, &taskmodels.Task{ID: taskID, WorkspaceID: workspaceID, Title: "Delivery task", IsEphemeral: true}); err != nil {
			return "", "", "", err
		}
		if _, err := invoke(repo, "CreateRepository", s.Context, &taskmodels.Repository{ID: repositoryID, WorkspaceID: workspaceID, Name: "Delivery repository", SourceType: "local", DefaultBranch: "main"}); err != nil {
			return "", "", "", err
		}
		if _, err := invoke(repo, "CreateTaskRepository", s.Context, &taskmodels.TaskRepository{ID: id + "-link", TaskID: taskID, RepositoryID: repositoryID, BaseBranch: "main"}); err != nil {
			return "", "", "", err
		}
		return workspaceID, taskID, repositoryID, nil
	}
	action.create = func(s testconformance.ScenarioContext, id string) (any, error) {
		workspaceID, taskID, repositoryID, err := prepare(s, id)
		if err != nil {
			return nil, err
		}
		store, err := deliveryFactory(s)
		if err != nil {
			return nil, err
		}
		at := time.Date(2024, 1, 3, 4, 5, 6, 0, time.UTC)
		_, err = store.(*delivery.Repository).Upsert(s.Context, delivery.UpsertInput{TaskID: taskID, RepositoryID: repositoryID, WorkspaceID: workspaceID, Classification: delivery.Classification{Outcome: delivery.OutcomeDirectCommit, Basis: delivery.BasisDefaultBranchCommit, Rank: 7}, EvaluatedAt: at})
		if err != nil {
			return nil, err
		}
		row, err := store.(*delivery.Repository).Get(s.Context, taskID, repositoryID)
		if err != nil {
			return nil, err
		}
		return &deliveryRecord{ID: id, TaskID: taskID, RepositoryID: repositoryID, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt}, nil
	}
	action.read = func(s testconformance.ScenarioContext, id string) (any, error) {
		store, err := deliveryFactory(s)
		if err != nil {
			return nil, err
		}
		row, err := store.(*delivery.Repository).Get(s.Context, id+"-task", id+"-repository")
		if err != nil {
			return nil, err
		}
		return &deliveryRecord{ID: id, TaskID: row.TaskID, RepositoryID: row.RepositoryID, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt}, nil
	}
	action.update = func(s testconformance.ScenarioContext, id string, _ any) error {
		store, err := deliveryFactory(s)
		if err != nil {
			return err
		}
		_, err = store.(*delivery.Repository).Upsert(s.Context, delivery.UpsertInput{TaskID: id + "-task", RepositoryID: id + "-repository", WorkspaceID: "", Classification: delivery.Classification{Outcome: delivery.OutcomePRMerge, Basis: delivery.BasisProviderPRMerged, Rank: 8}, EvaluatedAt: time.Date(2024, 1, 3, 5, 5, 6, 0, time.UTC)})
		return err
	}
	action.delete = func(s testconformance.ScenarioContext, id string) error {
		store, err := deliveryFactory(s)
		if err != nil {
			return err
		}
		return store.(*delivery.Repository).Delete(s.Context, id+"-task", id+"-repository")
	}
	action.conflict = func(s testconformance.ScenarioContext, id string, _ any) error {
		return action.update(s, id, nil)
	}
	action.transaction = func(s testconformance.ScenarioContext, id string) error {
		store, err := deliveryFactory(s)
		if err != nil {
			return err
		}
		repository := store.(*delivery.Repository)
		before, err := repository.Get(s.Context, id+"-task", id+"-repository")
		if err != nil {
			return fmt.Errorf("read before transaction: %w", err)
		}
		// A failed upsert must leave the existing pair untouched. The invalid
		// task reference reaches the repository's atomic INSERT ... ON CONFLICT
		// statement and exercises its rollback path.
		if _, err := repository.Upsert(s.Context, delivery.UpsertInput{
			TaskID: id + "-missing-task", RepositoryID: id + "-repository",
			WorkspaceID:    before.WorkspaceID,
			Classification: delivery.Classification{Outcome: delivery.OutcomeDirectCommit, Basis: delivery.BasisDefaultBranchCommit, Rank: 9},
			EvaluatedAt:    time.Date(2024, 1, 3, 6, 5, 6, 0, time.UTC),
		}); err == nil {
			return fmt.Errorf("invalid delivery upsert succeeded")
		}
		retained, err := repository.Get(s.Context, id+"-task", id+"-repository")
		if err != nil {
			return fmt.Errorf("read after rolled-back upsert: %w", err)
		}
		if retained.EvidenceRank != before.EvidenceRank || retained.LastEvaluatedAt.UTC() != before.LastEvaluatedAt.UTC() {
			return fmt.Errorf("rolled-back upsert changed the committed ledger row")
		}
		if _, err := repository.Upsert(s.Context, delivery.UpsertInput{
			TaskID: id + "-task", RepositoryID: id + "-repository", WorkspaceID: before.WorkspaceID,
			Classification: delivery.Classification{Outcome: delivery.OutcomePRMerge, Basis: delivery.BasisProviderPRMerged, Rank: 9},
			EvaluatedAt:    time.Date(2024, 1, 3, 6, 5, 6, 0, time.UTC),
		}); err != nil {
			return fmt.Errorf("committed delivery upsert: %w", err)
		}
		committed, err := repository.Get(s.Context, id+"-task", id+"-repository")
		if err != nil {
			return fmt.Errorf("read committed delivery upsert: %w", err)
		}
		if committed.EvidenceRank != 9 {
			return fmt.Errorf("committed delivery rank = %d, want 9", committed.EvidenceRank)
		}
		return nil
	}
	return action
}

func storageAction() apiAction {
	action := apiAction{name: "storage_temp_artifacts", nullableTimestampUpdates: []string{"LastHeartbeatAt"}, key: func(record any, fallback string) string {
		if value, ok := record.(*storagepkg.TemporaryArtifact); ok {
			return value.ID
		}
		return fallback
	}}
	action.create = func(s testconformance.ScenarioContext, id string) (any, error) {
		store, err := storageFactory(s)
		if err != nil {
			return nil, err
		}
		artifact := &storagepkg.TemporaryArtifact{ID: id, Kind: storagepkg.TemporaryArtifactKindImproveBundle, Path: "/tmp/" + id, MarkerToken: id + "-marker", State: storagepkg.TemporaryArtifactStateActive, OwnerPID: 1, Metadata: json.RawMessage(`{}`)}
		if err := store.(*storagepkg.Store).CreateTemporaryArtifact(s.Context, artifact); err != nil {
			return nil, err
		}
		return artifact, nil
	}
	action.read = func(s testconformance.ScenarioContext, id string) (any, error) {
		store, err := storageFactory(s)
		if err != nil {
			return nil, err
		}
		artifact, err := store.(*storagepkg.Store).GetTemporaryArtifact(s.Context, id)
		if err != nil {
			return nil, err
		}
		return &artifact, nil
	}
	action.update = func(s testconformance.ScenarioContext, id string, _ any) error {
		store, err := storageFactory(s)
		if err != nil {
			return err
		}
		return store.(*storagepkg.Store).HeartbeatTemporaryArtifact(s.Context, id, time.Now().UTC())
	}
	action.delete = func(s testconformance.ScenarioContext, id string) error {
		store, err := storageFactory(s)
		if err != nil {
			return err
		}
		return store.(*storagepkg.Store).TransitionTemporaryArtifact(s.Context, id, storagepkg.TemporaryArtifactStateDeleted, "", time.Now().UTC())
	}
	action.assertDeleted = func(s testconformance.ScenarioContext, id string, _ any) error {
		value, err := action.read(s, id)
		if err != nil {
			return err
		}
		if value.(*storagepkg.TemporaryArtifact).State != storagepkg.TemporaryArtifactStateDeleted {
			return fmt.Errorf("temporary artifact %q was not deleted", id)
		}
		return nil
	}
	action.conflict = func(s testconformance.ScenarioContext, id string, _ any) error {
		store, err := storageFactory(s)
		if err != nil {
			return err
		}
		if err := store.(*storagepkg.Store).CreateTemporaryArtifact(s.Context, &storagepkg.TemporaryArtifact{ID: id, Kind: storagepkg.TemporaryArtifactKindImproveBundle, Path: "/tmp/duplicate", MarkerToken: "duplicate", State: storagepkg.TemporaryArtifactStateActive}); err == nil {
			return fmt.Errorf("duplicate temporary artifact succeeded")
		}
		return nil
	}
	action.transaction = func(s testconformance.ScenarioContext, id string) error {
		return transactionAPICheck(action, s, id)
	}
	return action
}

type pluginStateBehaviorRecord struct {
	ID        string
	Enabled   bool
	CreatedAt time.Time
	UpdatedAt time.Time
}

//nolint:cyclop,gocognit,funlen // This adapter asserts the complete plugin-state contract.
func pluginStateAction() apiAction {
	const scope = "instance"
	action := apiAction{name: "plugin_state", key: func(record any, fallback string) string {
		if value, ok := record.(*pluginStateBehaviorRecord); ok {
			return value.ID
		}
		return fallback
	}}
	pluginID := func(id string) string { return "plugin-" + id }
	readEntry := func(s testconformance.ScenarioContext, id string) (*pluginStateBehaviorRecord, error) {
		store, err := pluginStateFactory(s)
		if err != nil {
			return nil, err
		}
		value, found, err := store.(*pluginstate.Store).Get(s.Context, pluginID(id), scope, "", "value")
		if err != nil {
			return nil, err
		}
		if !found {
			return nil, fmt.Errorf("plugin state %q not found", id)
		}
		var enabled bool
		if err := json.Unmarshal(value, &enabled); err != nil {
			return nil, err
		}
		entries, err := store.(*pluginstate.Store).List(s.Context, pluginID(id), scope, "")
		if err != nil {
			return nil, err
		}
		if len(entries) != 1 {
			return nil, fmt.Errorf("plugin state list returned %d entries", len(entries))
		}
		return &pluginStateBehaviorRecord{ID: id, Enabled: enabled, CreatedAt: entries[0].UpdatedAt, UpdatedAt: entries[0].UpdatedAt}, nil
	}
	action.create = func(s testconformance.ScenarioContext, id string) (any, error) {
		store, err := pluginStateFactory(s)
		if err != nil {
			return nil, err
		}
		if err := store.(*pluginstate.Store).Set(s.Context, pluginID(id), scope, "", "value", json.RawMessage(`false`)); err != nil {
			return nil, err
		}
		return readEntry(s, id)
	}
	action.read = func(s testconformance.ScenarioContext, id string) (any, error) {
		return readEntry(s, id)
	}
	action.update = func(s testconformance.ScenarioContext, id string, _ any) error {
		store, err := pluginStateFactory(s)
		if err != nil {
			return err
		}
		return store.(*pluginstate.Store).Set(s.Context, pluginID(id), scope, "", "value", json.RawMessage(`true`))
	}
	action.delete = func(s testconformance.ScenarioContext, id string) error {
		store, err := pluginStateFactory(s)
		if err != nil {
			return err
		}
		return store.(*pluginstate.Store).Delete(s.Context, pluginID(id), scope, "", "value")
	}
	action.assertDeleted = func(s testconformance.ScenarioContext, id string, _ any) error {
		store, err := pluginStateFactory(s)
		if err != nil {
			return err
		}
		_, found, err := store.(*pluginstate.Store).Get(s.Context, pluginID(id), scope, "", "value")
		if err != nil {
			return err
		}
		if found {
			return fmt.Errorf("plugin state %q remains after delete", id)
		}
		return nil
	}
	action.setBoolean = func(s testconformance.ScenarioContext, id string, _ any, enabled bool) (any, error) {
		store, err := pluginStateFactory(s)
		if err != nil {
			return nil, err
		}
		value, err := json.Marshal(enabled)
		if err != nil {
			return nil, err
		}
		if err := store.(*pluginstate.Store).Set(s.Context, pluginID(id), scope, "", "value", value); err != nil {
			return nil, err
		}
		return readEntry(s, id)
	}
	action.readBoolean = func(record any) (bool, bool) {
		value, ok := record.(*pluginStateBehaviorRecord)
		return value != nil && value.Enabled, ok
	}
	action.conflict = func(s testconformance.ScenarioContext, id string, _ any) error {
		return action.update(s, id, nil)
	}
	return action
}

//nolint:cyclop,gocognit,funlen // This adapter asserts the complete plugin-instance-state contract.
func pluginInstanceStateAction() apiAction {
	action := apiAction{name: "plugin_instance_state", key: func(record any, fallback string) string {
		if value, ok := record.(*pluginStateBehaviorRecord); ok {
			return value.ID
		}
		return fallback
	}}
	instanceID := func(id string) string { return "instance-" + id }
	readEntry := func(s testconformance.ScenarioContext, id string) (*pluginStateBehaviorRecord, error) {
		store, err := pluginInstanceStateFactory(s)
		if err != nil {
			return nil, err
		}
		entry, found, err := store.(*pluginstate.InstanceStore).Get(s.Context, instanceID(id), "value")
		if err != nil {
			return nil, err
		}
		if !found {
			return nil, fmt.Errorf("plugin instance state %q not found", id)
		}
		var enabled bool
		if err := json.Unmarshal(entry.Value, &enabled); err != nil {
			return nil, err
		}
		return &pluginStateBehaviorRecord{ID: id, Enabled: enabled, CreatedAt: entry.UpdatedAt, UpdatedAt: entry.UpdatedAt}, nil
	}
	action.create = func(s testconformance.ScenarioContext, id string) (any, error) {
		store, err := pluginInstanceStateFactory(s)
		if err != nil {
			return nil, err
		}
		if _, err := store.(*pluginstate.InstanceStore).Set(s.Context, instanceID(id), "value", json.RawMessage(`false`), nil, "conformance"); err != nil {
			return nil, err
		}
		return readEntry(s, id)
	}
	action.read = func(s testconformance.ScenarioContext, id string) (any, error) { return readEntry(s, id) }
	action.update = func(s testconformance.ScenarioContext, id string, _ any) error {
		store, err := pluginInstanceStateFactory(s)
		if err != nil {
			return err
		}
		_, err = store.(*pluginstate.InstanceStore).Set(s.Context, instanceID(id), "value", json.RawMessage(`true`), nil, "conformance")
		return err
	}
	action.delete = func(s testconformance.ScenarioContext, id string) error {
		store, err := pluginInstanceStateFactory(s)
		if err != nil {
			return err
		}
		_, err = store.(*pluginstate.InstanceStore).Delete(s.Context, instanceID(id), "value", nil, "conformance")
		return err
	}
	action.assertDeleted = func(s testconformance.ScenarioContext, id string, _ any) error {
		store, err := pluginInstanceStateFactory(s)
		if err != nil {
			return err
		}
		_, found, err := store.(*pluginstate.InstanceStore).Get(s.Context, instanceID(id), "value")
		if err != nil {
			return err
		}
		if found {
			return fmt.Errorf("plugin instance state %q remains after delete", id)
		}
		return nil
	}
	action.setBoolean = func(s testconformance.ScenarioContext, id string, _ any, enabled bool) (any, error) {
		store, err := pluginInstanceStateFactory(s)
		if err != nil {
			return nil, err
		}
		value, err := json.Marshal(enabled)
		if err != nil {
			return nil, err
		}
		if _, err := store.(*pluginstate.InstanceStore).Set(s.Context, instanceID(id), "value", value, nil, "conformance"); err != nil {
			return nil, err
		}
		return readEntry(s, id)
	}
	action.readBoolean = func(record any) (bool, bool) {
		value, ok := record.(*pluginStateBehaviorRecord)
		return value != nil && value.Enabled, ok
	}
	action.conflict = func(s testconformance.ScenarioContext, id string, _ any) error {
		store, err := pluginInstanceStateFactory(s)
		if err != nil {
			return err
		}
		stale := int64(0)
		if _, err := store.(*pluginstate.InstanceStore).Set(s.Context, instanceID(id), "value", json.RawMessage(`true`), &stale, "conformance"); err == nil {
			return fmt.Errorf("stale plugin instance state write succeeded")
		}
		return nil
	}
	return action
}

//nolint:cyclop,gocognit,funlen // This adapter asserts the complete plugin-user-state contract.
func pluginUserStateAction() apiAction {
	const scope = "instance"
	action := apiAction{name: "plugin_user_state", key: func(record any, fallback string) string {
		if value, ok := record.(*pluginStateBehaviorRecord); ok {
			return value.ID
		}
		return fallback
	}}
	pluginID := func(id string) string { return "user-plugin-" + id }
	readEntry := func(s testconformance.ScenarioContext, id string) (*pluginStateBehaviorRecord, error) {
		store, err := pluginUserStateFactory(s)
		if err != nil {
			return nil, err
		}
		value, updatedAt, found, err := store.(*pluginstate.UserStore).Get(s.Context, pluginID(id), defaultUserID, scope, "", "value")
		if err != nil {
			return nil, err
		}
		if !found {
			return nil, fmt.Errorf("plugin user state %q not found", id)
		}
		var enabled bool
		if err := json.Unmarshal(value, &enabled); err != nil {
			return nil, err
		}
		return &pluginStateBehaviorRecord{ID: id, Enabled: enabled, CreatedAt: updatedAt, UpdatedAt: updatedAt}, nil
	}
	action.create = func(s testconformance.ScenarioContext, id string) (any, error) {
		store, err := pluginUserStateFactory(s)
		if err != nil {
			return nil, err
		}
		if _, err := store.(*pluginstate.UserStore).Set(s.Context, pluginID(id), defaultUserID, scope, "", "value", json.RawMessage(`false`), nil); err != nil {
			return nil, err
		}
		return readEntry(s, id)
	}
	action.read = func(s testconformance.ScenarioContext, id string) (any, error) { return readEntry(s, id) }
	action.update = func(s testconformance.ScenarioContext, id string, _ any) error {
		store, err := pluginUserStateFactory(s)
		if err != nil {
			return err
		}
		_, err = store.(*pluginstate.UserStore).Set(s.Context, pluginID(id), defaultUserID, scope, "", "value", json.RawMessage(`true`), nil)
		return err
	}
	action.delete = func(s testconformance.ScenarioContext, id string) error {
		store, err := pluginUserStateFactory(s)
		if err != nil {
			return err
		}
		return store.(*pluginstate.UserStore).Delete(s.Context, pluginID(id), defaultUserID, scope, "", "value")
	}
	action.assertDeleted = func(s testconformance.ScenarioContext, id string, _ any) error {
		store, err := pluginUserStateFactory(s)
		if err != nil {
			return err
		}
		_, _, found, err := store.(*pluginstate.UserStore).Get(s.Context, pluginID(id), defaultUserID, scope, "", "value")
		if err != nil {
			return err
		}
		if found {
			return fmt.Errorf("plugin user state %q remains after delete", id)
		}
		return nil
	}
	action.setBoolean = func(s testconformance.ScenarioContext, id string, _ any, enabled bool) (any, error) {
		store, err := pluginUserStateFactory(s)
		if err != nil {
			return nil, err
		}
		value, err := json.Marshal(enabled)
		if err != nil {
			return nil, err
		}
		if _, err := store.(*pluginstate.UserStore).Set(s.Context, pluginID(id), defaultUserID, scope, "", "value", value, nil); err != nil {
			return nil, err
		}
		return readEntry(s, id)
	}
	action.readBoolean = func(record any) (bool, bool) {
		value, ok := record.(*pluginStateBehaviorRecord)
		return value != nil && value.Enabled, ok
	}
	action.conflict = func(s testconformance.ScenarioContext, id string, _ any) error {
		store, err := pluginUserStateFactory(s)
		if err != nil {
			return err
		}
		old, _, found, err := store.(*pluginstate.UserStore).Get(s.Context, pluginID(id), defaultUserID, scope, "", "value")
		if err != nil || !found {
			return fmt.Errorf("read user state for conflict: %w", err)
		}
		_ = old
		stale := time.Unix(0, 0).UTC()
		if _, err := store.(*pluginstate.UserStore).Set(s.Context, pluginID(id), defaultUserID, scope, "", "value", json.RawMessage(`true`), &stale); err == nil {
			return fmt.Errorf("stale plugin user state write succeeded")
		}
		return nil
	}
	return action
}

func pluginSettingsAction() apiAction {
	action := apiAction{name: "plugin_settings", key: func(_ any, fallback string) string { return fallback }}
	action.create = func(s testconformance.ScenarioContext, id string) (any, error) {
		store, err := pluginSettingsFactory(s)
		if err != nil {
			return nil, err
		}
		owner := store.(plugins.SettingsStore)
		if err := owner.SetAutoUpdateDefault(false); err != nil {
			return nil, err
		}
		return pluginSettingsRecord(owner, id)
	}
	action.read = func(s testconformance.ScenarioContext, id string) (any, error) {
		store, err := pluginSettingsFactory(s)
		if err != nil {
			return nil, err
		}
		return pluginSettingsRecord(store.(plugins.SettingsStore), id)
	}
	action.update = func(s testconformance.ScenarioContext, _ string, _ any) error {
		store, err := pluginSettingsFactory(s)
		if err != nil {
			return err
		}
		return store.(plugins.SettingsStore).SetAutoUpdateDefault(true)
	}
	action.delete = func(s testconformance.ScenarioContext, _ string) error {
		store, err := pluginSettingsFactory(s)
		if err != nil {
			return err
		}
		return store.(plugins.SettingsStore).Delete()
	}
	action.assertDeleted = func(s testconformance.ScenarioContext, _ string, _ any) error {
		store, err := pluginSettingsFactory(s)
		if err != nil {
			return err
		}
		_, found, err := store.(plugins.SettingsStore).Snapshot()
		if err != nil {
			return err
		}
		if found {
			return fmt.Errorf("plugin settings remains after delete")
		}
		return nil
	}
	action.setBoolean = func(s testconformance.ScenarioContext, id string, _ any, enabled bool) (any, error) {
		store, err := pluginSettingsFactory(s)
		if err != nil {
			return nil, err
		}
		if err := store.(plugins.SettingsStore).SetAutoUpdateDefault(enabled); err != nil {
			return nil, err
		}
		return pluginSettingsRecord(store.(plugins.SettingsStore), id)
	}
	action.readBoolean = func(record any) (bool, bool) {
		value, ok := record.(*pluginStateBehaviorRecord)
		return value != nil && value.Enabled, ok
	}
	action.conflict = func(s testconformance.ScenarioContext, _ string, _ any) error {
		store, err := pluginSettingsFactory(s)
		if err != nil {
			return err
		}
		return store.(plugins.SettingsStore).SetAutoUpdateDefault(false)
	}
	return action
}

func pluginSettingsRecord(store plugins.SettingsStore, id string) (any, error) {
	snapshot, found, err := store.Snapshot()
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, fmt.Errorf("plugin settings row is absent")
	}
	return &pluginStateBehaviorRecord{ID: id, Enabled: snapshot.AutoUpdateDefault, CreatedAt: snapshot.UpdatedAt, UpdatedAt: snapshot.UpdatedAt}, nil
}

//nolint:cyclop,gocognit,funlen // This adapter asserts the complete plugin-instance contract.
func pluginInstanceAction() apiAction {
	action := apiAction{name: "plugin_instances", key: func(record any, fallback string) string {
		if value, ok := record.(*instances.Instance); ok {
			return value.ID
		}
		if value, ok := record.(instances.Instance); ok {
			return value.ID
		}
		return fallback
	}}
	action.create = func(s testconformance.ScenarioContext, id string) (any, error) {
		store, err := pluginInstancesFactory(s)
		if err != nil {
			return nil, err
		}
		value := instances.Instance{ID: id, PluginID: "plugin-" + id, SourceKind: instances.SourceInstalled, ScopeKind: instances.ScopeInstance, Status: instances.StatusActive}
		if err := store.(*instances.Store).Create(s.Context, value); err != nil {
			return nil, err
		}
		read, err := store.(*instances.Store).Get(s.Context, id)
		if err != nil {
			return nil, err
		}
		return &read, nil
	}
	action.read = func(s testconformance.ScenarioContext, id string) (any, error) {
		store, err := pluginInstancesFactory(s)
		if err != nil {
			return nil, err
		}
		read, err := store.(*instances.Store).Get(s.Context, id)
		if err != nil {
			return nil, err
		}
		return &read, nil
	}
	action.update = func(s testconformance.ScenarioContext, id string, _ any) error {
		store, err := pluginInstancesFactory(s)
		if err != nil {
			return err
		}
		return store.(*instances.Store).SetPluginID(s.Context, id, "updated-plugin-"+id)
	}
	action.delete = func(s testconformance.ScenarioContext, id string) error {
		store, err := pluginInstancesFactory(s)
		if err != nil {
			return err
		}
		return store.(*instances.Store).RemoveInstance(s.Context, id)
	}
	action.assertDeleted = func(s testconformance.ScenarioContext, id string, _ any) error {
		value, err := action.read(s, id)
		if err != nil {
			return err
		}
		if value.(*instances.Instance).Status != instances.StatusRemoved {
			return fmt.Errorf("plugin instance %q was not removed", id)
		}
		return nil
	}
	action.setBoolean = func(s testconformance.ScenarioContext, id string, _ any, active bool) (any, error) {
		store, err := pluginInstancesFactory(s)
		if err != nil {
			return nil, err
		}
		if active {
			err = store.(*instances.Store).Restore(s.Context, id)
		} else {
			err = store.(*instances.Store).Archive(s.Context, id)
		}
		if err != nil {
			return nil, err
		}
		return action.read(s, id)
	}
	action.readBoolean = func(record any) (bool, bool) {
		value, ok := record.(*instances.Instance)
		return ok && value.Status == instances.StatusActive, ok
	}
	action.conflict = func(s testconformance.ScenarioContext, id string, _ any) error {
		store, err := pluginInstancesFactory(s)
		if err != nil {
			return err
		}
		if err := store.(*instances.Store).Create(s.Context, instances.Instance{ID: id, PluginID: "duplicate", SourceKind: instances.SourceInstalled, ScopeKind: instances.ScopeInstance, Status: instances.StatusActive}); err == nil {
			return fmt.Errorf("duplicate plugin instance succeeded")
		}
		return nil
	}
	return action
}

func marketplaceAction() apiAction {
	action := apiAction{name: "plugin_marketplace_source", key: func(record any, fallback string) string {
		if value, ok := record.(*marketplace.SourceRecord); ok {
			return value.ID
		}
		return fallback
	}}
	action.create = func(s testconformance.ScenarioContext, id string) (any, error) {
		store, err := marketplaceFactory(s)
		if err != nil {
			return nil, err
		}
		return store.(*marketplace.SourceStore).Add("Conformance "+id, "https://example.test/marketplace/"+id)
	}
	action.read = func(s testconformance.ScenarioContext, id string) (any, error) {
		store, err := marketplaceFactory(s)
		if err != nil {
			return nil, err
		}
		return store.(*marketplace.SourceStore).Get(id)
	}
	action.update = func(s testconformance.ScenarioContext, id string, _ any) error {
		store, err := marketplaceFactory(s)
		if err != nil {
			return err
		}
		_, err = store.(*marketplace.SourceStore).Update(id, nil, boolPointer(false))
		return err
	}
	action.delete = func(s testconformance.ScenarioContext, id string) error {
		store, err := marketplaceFactory(s)
		if err != nil {
			return err
		}
		return store.(*marketplace.SourceStore).Delete(id)
	}
	action.setBoolean = func(s testconformance.ScenarioContext, id string, _ any, enabled bool) (any, error) {
		store, err := marketplaceFactory(s)
		if err != nil {
			return nil, err
		}
		value, err := store.(*marketplace.SourceStore).Update(id, nil, boolPointer(enabled))
		return value, err
	}
	action.readBoolean = func(record any) (bool, bool) {
		value, ok := record.(*marketplace.SourceRecord)
		return ok && value.Enabled, ok
	}
	action.conflict = func(s testconformance.ScenarioContext, id string, record any) error {
		store, err := marketplaceFactory(s)
		if err != nil {
			return err
		}
		url := "https://example.test/marketplace/" + id
		if value, ok := record.(*marketplace.SourceRecord); ok {
			url = value.URL
		}
		if _, err := store.(*marketplace.SourceStore).Add("Duplicate", url); err == nil {
			return fmt.Errorf("duplicate marketplace source succeeded")
		}
		return nil
	}
	return action
}

func boolPointer(value bool) *bool { return &value }

//nolint:cyclop,gocognit,funlen // This adapter asserts the complete canvas contract.
func canvasAction() apiAction {
	action := apiAction{name: "canvas_lifecycle_metadata", nullableTimestampUpdates: []string{"PromotedAt"}, key: func(record any, fallback string) string {
		if value, ok := record.(*canvas.CanvasMetadata); ok {
			return value.ID
		}
		if value, ok := record.(canvas.CanvasMetadata); ok {
			return value.ID
		}
		return fallback
	}}
	action.create = func(s testconformance.ScenarioContext, id string) (any, error) {
		store, err := canvasFactory(s)
		if err != nil {
			return nil, err
		}
		value := canvas.CanvasMetadata{ID: id, PluginInstanceID: "instance-" + id, WorkspaceID: "workspace-" + id, TaskID: "task-" + id, Title: "Conformance canvas", CreatedBySessionID: "session-" + id}
		if err := store.(*canvas.Repository).Create(s.Context, value); err != nil {
			return nil, err
		}
		read, err := store.(*canvas.Repository).Get(s.Context, id)
		if err != nil {
			return nil, err
		}
		return &read, nil
	}
	action.read = func(s testconformance.ScenarioContext, id string) (any, error) {
		store, err := canvasFactory(s)
		if err != nil {
			return nil, err
		}
		read, err := store.(*canvas.Repository).Get(s.Context, id)
		if err != nil {
			return nil, err
		}
		return &read, nil
	}
	action.update = func(s testconformance.ScenarioContext, id string, _ any) error {
		store, err := canvasFactory(s)
		if err != nil {
			return err
		}
		return store.(*canvas.Repository).Promote(s.Context, id, defaultUserID, time.Now().UTC())
	}
	action.delete = func(s testconformance.ScenarioContext, id string) error {
		store, err := canvasFactory(s)
		if err != nil {
			return err
		}
		return store.(*canvas.Repository).Delete(s.Context, id)
	}
	action.setBoolean = func(s testconformance.ScenarioContext, id string, _ any, promoted bool) (any, error) {
		store, err := canvasFactory(s)
		if err != nil {
			return nil, err
		}
		if promoted {
			err = store.(*canvas.Repository).Promote(s.Context, id, defaultUserID, time.Now().UTC())
		} else {
			// New metadata starts task-owned, so the false case is asserted by
			// reading that real initial value. Promotion is the owning API's
			// reversible boolean transition exposed by this repository.
			return action.read(s, id)
		}
		if err != nil {
			return nil, err
		}
		return action.read(s, id)
	}
	action.readBoolean = func(record any) (bool, bool) {
		value, ok := record.(*canvas.CanvasMetadata)
		return ok && value.PromotedAt != nil, ok
	}
	action.conflict = func(s testconformance.ScenarioContext, id string, _ any) error {
		store, err := canvasFactory(s)
		if err != nil {
			return err
		}
		if err := store.(*canvas.Repository).Create(s.Context, canvas.CanvasMetadata{ID: id, PluginInstanceID: "duplicate", WorkspaceID: "duplicate", TaskID: "duplicate", Title: "Duplicate"}); err == nil {
			return fmt.Errorf("duplicate canvas metadata succeeded")
		}
		return nil
	}
	action.assertDeleted = func(s testconformance.ScenarioContext, id string, _ any) error {
		store, err := canvasFactory(s)
		if err != nil {
			return err
		}
		if _, err := store.(*canvas.Repository).Get(s.Context, id); err == nil {
			return fmt.Errorf("canvas metadata %q remains after delete", id)
		}
		return nil
	}
	action.transaction = func(s testconformance.ScenarioContext, id string) error {
		return transactionAPICheck(action, s, id)
	}
	return action
}

//nolint:cyclop,gocognit,funlen // This adapter asserts the complete GitHub store contract.
func githubAction() apiAction {
	action := apiAction{name: "github_pr_watches", nullableTimestampUpdates: []string{"LastCheckedAt"}, key: func(record any, fallback string) string {
		if value, ok := record.(*github.PRWatch); ok {
			return value.ID
		}
		return fallback
	}}
	action.create = func(s testconformance.ScenarioContext, id string) (any, error) {
		store, err := githubFactory(s)
		if err != nil {
			return nil, err
		}
		ws, err := conformanceWorkspaceID(s)
		if err != nil {
			return nil, err
		}
		taskID, sessionID, err := ensureConformanceTaskAndSession(s, id)
		if err != nil {
			return nil, err
		}
		value := &github.PRWatch{ID: id, WorkspaceID: ws, SessionID: sessionID, TaskID: taskID, Owner: "owner", Repo: "repo", PRNumber: 1, Branch: "main", LastCheckStatus: "pending", LastReviewState: "pending"}
		if err := store.(*github.Store).CreatePRWatch(s.Context, value); err != nil {
			return nil, err
		}
		return value, nil
	}
	action.read = func(s testconformance.ScenarioContext, id string) (any, error) {
		store, err := githubFactory(s)
		if err != nil {
			return nil, err
		}
		return store.(*github.Store).GetPRWatch(s.Context, id)
	}
	action.update = func(s testconformance.ScenarioContext, id string, _ any) error {
		store, err := githubFactory(s)
		if err != nil {
			return err
		}
		return store.(*github.Store).UpdatePRWatchTimestamps(s.Context, id, time.Now().UTC(), nil, "success", "approved")
	}
	action.delete = func(s testconformance.ScenarioContext, id string) error {
		store, err := githubFactory(s)
		if err != nil {
			return err
		}
		return store.(*github.Store).DeletePRWatch(s.Context, id)
	}
	action.assertDeleted = func(s testconformance.ScenarioContext, id string, _ any) error {
		store, err := githubFactory(s)
		if err != nil {
			return err
		}
		value, err := store.(*github.Store).GetPRWatch(s.Context, id)
		if err != nil {
			return err
		}
		if value != nil {
			return fmt.Errorf("GitHub PR watch %q remains after delete", id)
		}
		return nil
	}
	action.setBoolean = func(s testconformance.ScenarioContext, id string, _ any, enabled bool) (any, error) {
		store, err := githubFactory(s)
		if err != nil {
			return nil, err
		}
		status := "disabled"
		if enabled {
			status = "success"
		}
		if err := store.(*github.Store).UpdatePRWatchTimestamps(s.Context, id, time.Now().UTC(), nil, status, ""); err != nil {
			return nil, err
		}
		read, err := action.read(s, id)
		if err != nil {
			return nil, err
		}
		if value, ok := read.(*github.PRWatch); !ok || value == nil {
			return nil, fmt.Errorf("read after boolean update returned no GitHub PR watch (status=%q, id=%q)", status, id)
		}
		return read, nil
	}
	action.readBoolean = func(record any) (bool, bool) {
		value, ok := record.(*github.PRWatch)
		if !ok || value == nil {
			return false, false
		}
		return value.LastCheckStatus != "disabled", true
	}
	action.conflict = func(s testconformance.ScenarioContext, id string, _ any) error {
		store, err := githubFactory(s)
		if err != nil {
			return err
		}
		duplicate := &github.PRWatch{ID: id, WorkspaceID: "duplicate", SessionID: "duplicate", TaskID: "duplicate", Owner: "owner", Repo: "repo", PRNumber: 2, Branch: "main", LastCheckStatus: "pending", LastReviewState: "pending"}
		if err := store.(*github.Store).CreatePRWatch(s.Context, duplicate); err == nil {
			return fmt.Errorf("duplicate GitHub PR watch succeeded")
		}
		return nil
	}
	return action
}

func providerConfigKey(record any, fallback string) string {
	value := reflect.ValueOf(record)
	if value.Kind() == reflect.Pointer {
		if value.IsNil() {
			return fallback
		}
		value = value.Elem()
	}
	if value.IsValid() && value.Kind() == reflect.Struct {
		for _, fieldName := range []string{"WorkspaceID", "ID"} {
			field := value.FieldByName(fieldName)
			if field.IsValid() && field.Kind() == reflect.String && field.String() != "" {
				return field.String()
			}
		}
	}
	return fallback
}

func providerBoolean(record any) (bool, bool) {
	value := reflect.ValueOf(record)
	if value.Kind() == reflect.Pointer {
		if value.IsNil() {
			return false, false
		}
		value = value.Elem()
	}
	if !value.IsValid() || value.Kind() != reflect.Struct {
		return false, false
	}
	for _, fieldName := range []string{"LastOK", "LastOk", "Enabled"} {
		field := value.FieldByName(fieldName)
		if field.IsValid() && field.Kind() == reflect.Bool {
			return field.Bool(), true
		}
	}
	return false, false
}

func requireProviderConfig[T any](cfg *T, owner string) (*T, error) {
	if cfg == nil {
		return nil, fmt.Errorf("%s config is absent", owner)
	}
	return cfg, nil
}

//nolint:funlen // This adapter enumerates every GitLab store assertion.
func gitlabAction() apiAction {
	action := apiAction{name: "gitlab_configs", key: providerConfigKey}
	workspace := conformanceWorkspaceID
	action.create = func(s testconformance.ScenarioContext, _ string) (any, error) {
		store, err := gitlabFactory(s)
		if err != nil {
			return nil, err
		}
		workspaceID, err := workspace(s)
		if err != nil {
			return nil, err
		}
		cfg := &gitlab.GitLabConfig{WorkspaceID: workspaceID, Host: "https://gitlab.example.test", AuthMethod: "pat", Username: "conformance"}
		if err := store.(*gitlab.Store).UpsertConfigForWorkspace(s.Context, workspaceID, cfg); err != nil {
			return nil, err
		}
		return gitlabConfigRead(s, store.(*gitlab.Store), workspaceID)
	}
	action.read = func(s testconformance.ScenarioContext, id string) (any, error) {
		store, err := gitlabFactory(s)
		if err != nil {
			return nil, err
		}
		return gitlabConfigRead(s, store.(*gitlab.Store), id)
	}
	action.update = func(s testconformance.ScenarioContext, id string, _ any) error {
		store, err := gitlabFactory(s)
		if err != nil {
			return err
		}
		cfg, err := gitlabConfigRead(s, store.(*gitlab.Store), id)
		if err != nil {
			return err
		}
		cfg.Host = "https://updated.gitlab.example.test"
		return store.(*gitlab.Store).UpsertConfigForWorkspace(s.Context, id, cfg)
	}
	action.delete = func(s testconformance.ScenarioContext, id string) error {
		store, err := gitlabFactory(s)
		if err != nil {
			return err
		}
		return store.(*gitlab.Store).DeleteConfigForWorkspace(s.Context, id)
	}
	action.assertDeleted = func(s testconformance.ScenarioContext, id string, _ any) error {
		store, err := gitlabFactory(s)
		if err != nil {
			return err
		}
		cfg, err := store.(*gitlab.Store).GetConfigForWorkspace(s.Context, id)
		if err != nil {
			return err
		}
		if cfg != nil {
			return fmt.Errorf("gitlab config %q remains after delete", id)
		}
		return nil
	}
	action.setBoolean = func(s testconformance.ScenarioContext, id string, _ any, enabled bool) (any, error) {
		store, err := gitlabFactory(s)
		if err != nil {
			return nil, err
		}
		cfg, err := gitlabConfigRead(s, store.(*gitlab.Store), id)
		if err != nil {
			return nil, err
		}
		updated, err := store.(*gitlab.Store).UpdateConfigHealthForRevision(s.Context, id, cfg.Username, enabled, "", time.Now().UTC(), cfg.Revision)
		if err != nil {
			return nil, err
		}
		if !updated {
			return nil, fmt.Errorf("GitLab health update did not affect config %q", id)
		}
		return gitlabConfigRead(s, store.(*gitlab.Store), id)
	}
	action.readBoolean = providerBoolean
	action.conflict = action.update
	return action
}

func gitlabConfigRead(s testconformance.ScenarioContext, store *gitlab.Store, workspaceID string) (*gitlab.GitLabConfig, error) {
	cfg, err := store.GetConfigForWorkspace(s.Context, workspaceID)
	if err != nil {
		return nil, err
	}
	return requireProviderConfig(cfg, "GitLab")
}

//nolint:funlen // This adapter enumerates every Jira store assertion.
func jiraAction() apiAction {
	action := apiAction{name: "jira_configs", key: providerConfigKey}
	action.create = func(s testconformance.ScenarioContext, _ string) (any, error) {
		store, err := jiraFactory(s)
		if err != nil {
			return nil, err
		}
		workspaceID, err := conformanceWorkspaceID(s)
		if err != nil {
			return nil, err
		}
		cfg := &jira.JiraConfig{WorkspaceID: workspaceID, SiteURL: "https://jira.example.test", Email: "conformance@example.test", AuthMethod: jira.AuthMethodAPIToken, InstanceType: jira.InstanceTypeCloud, DefaultProjectKey: "CONF"}
		if err := store.(*jira.Store).UpsertConfigForWorkspace(s.Context, workspaceID, cfg); err != nil {
			return nil, err
		}
		return jiraConfigRead(s, store.(*jira.Store), workspaceID)
	}
	action.read = func(s testconformance.ScenarioContext, id string) (any, error) {
		store, err := jiraFactory(s)
		if err != nil {
			return nil, err
		}
		return jiraConfigRead(s, store.(*jira.Store), id)
	}
	action.update = func(s testconformance.ScenarioContext, id string, _ any) error {
		store, err := jiraFactory(s)
		if err != nil {
			return err
		}
		cfg, err := jiraConfigRead(s, store.(*jira.Store), id)
		if err != nil {
			return err
		}
		cfg.SiteURL = "https://updated.jira.example.test"
		return store.(*jira.Store).UpsertConfigForWorkspace(s.Context, id, cfg)
	}
	action.delete = func(s testconformance.ScenarioContext, id string) error {
		store, err := jiraFactory(s)
		if err != nil {
			return err
		}
		return store.(*jira.Store).DeleteConfigForWorkspace(s.Context, id)
	}
	action.assertDeleted = func(s testconformance.ScenarioContext, id string, _ any) error {
		store, err := jiraFactory(s)
		if err != nil {
			return err
		}
		cfg, err := store.(*jira.Store).GetConfigForWorkspace(s.Context, id)
		if err != nil {
			return err
		}
		if cfg != nil {
			return fmt.Errorf("jira config %q remains after delete", id)
		}
		return nil
	}
	action.setBoolean = func(s testconformance.ScenarioContext, id string, _ any, enabled bool) (any, error) {
		store, err := jiraFactory(s)
		if err != nil {
			return nil, err
		}
		if err := store.(*jira.Store).UpdateAuthHealthForWorkspace(s.Context, id, enabled, "", time.Now().UTC()); err != nil {
			return nil, err
		}
		return jiraConfigRead(s, store.(*jira.Store), id)
	}
	action.readBoolean = providerBoolean
	action.conflict = action.update
	return action
}

func jiraConfigRead(s testconformance.ScenarioContext, store *jira.Store, workspaceID string) (*jira.JiraConfig, error) {
	cfg, err := store.GetConfigForWorkspace(s.Context, workspaceID)
	if err != nil {
		return nil, err
	}
	return requireProviderConfig(cfg, "Jira")
}

//nolint:funlen // This adapter enumerates every Linear store assertion.
func linearAction() apiAction {
	action := apiAction{name: "linear_configs", key: providerConfigKey}
	action.create = func(s testconformance.ScenarioContext, _ string) (any, error) {
		store, err := linearFactory(s)
		if err != nil {
			return nil, err
		}
		workspaceID, err := conformanceWorkspaceID(s)
		if err != nil {
			return nil, err
		}
		cfg := &linear.LinearConfig{WorkspaceID: workspaceID, AuthMethod: linear.AuthMethodAPIKey, DefaultTeamKey: "CONF"}
		if err := store.(*linear.Store).UpsertConfigForWorkspace(s.Context, workspaceID, cfg); err != nil {
			return nil, err
		}
		return linearConfigRead(s, store.(*linear.Store), workspaceID)
	}
	action.read = func(s testconformance.ScenarioContext, id string) (any, error) {
		store, err := linearFactory(s)
		if err != nil {
			return nil, err
		}
		return linearConfigRead(s, store.(*linear.Store), id)
	}
	action.update = func(s testconformance.ScenarioContext, id string, _ any) error {
		store, err := linearFactory(s)
		if err != nil {
			return err
		}
		cfg, err := linearConfigRead(s, store.(*linear.Store), id)
		if err != nil {
			return err
		}
		cfg.DefaultTeamKey = "UPDATED"
		return store.(*linear.Store).UpsertConfigForWorkspace(s.Context, id, cfg)
	}
	action.delete = func(s testconformance.ScenarioContext, id string) error {
		store, err := linearFactory(s)
		if err != nil {
			return err
		}
		return store.(*linear.Store).DeleteConfigForWorkspace(s.Context, id)
	}
	action.assertDeleted = func(s testconformance.ScenarioContext, id string, _ any) error {
		store, err := linearFactory(s)
		if err != nil {
			return err
		}
		cfg, err := store.(*linear.Store).GetConfigForWorkspace(s.Context, id)
		if err != nil {
			return err
		}
		if cfg != nil {
			return fmt.Errorf("linear config %q remains after delete", id)
		}
		return nil
	}
	action.setBoolean = func(s testconformance.ScenarioContext, id string, _ any, enabled bool) (any, error) {
		store, err := linearFactory(s)
		if err != nil {
			return nil, err
		}
		if err := store.(*linear.Store).UpdateAuthHealthForWorkspace(s.Context, id, enabled, "", "", time.Now().UTC()); err != nil {
			return nil, err
		}
		return linearConfigRead(s, store.(*linear.Store), id)
	}
	action.readBoolean = providerBoolean
	action.conflict = action.update
	return action
}

func linearConfigRead(s testconformance.ScenarioContext, store *linear.Store, workspaceID string) (*linear.LinearConfig, error) {
	cfg, err := store.GetConfigForWorkspace(s.Context, workspaceID)
	if err != nil {
		return nil, err
	}
	return requireProviderConfig(cfg, "Linear")
}

//nolint:cyclop,gocognit,funlen // This adapter asserts the complete Sentry store contract.
func sentryAction() apiAction {
	action := apiAction{name: "sentry_configs", key: func(record any, fallback string) string {
		if value, ok := record.(*sentry.SentryConfig); ok && value.ID != "" {
			return value.ID
		}
		return fallback
	}}
	action.create = func(s testconformance.ScenarioContext, id string) (any, error) {
		store, err := sentryFactory(s)
		if err != nil {
			return nil, err
		}
		workspaceID, err := conformanceWorkspaceID(s)
		if err != nil {
			return nil, err
		}
		cfg := &sentry.SentryConfig{ID: id, WorkspaceID: workspaceID, Name: "Conformance " + id, AuthMethod: sentry.AuthMethodAuthToken, URL: "https://sentry.example.test"}
		if err := store.(*sentry.Store).CreateInstance(s.Context, cfg); err != nil {
			return nil, err
		}
		return sentryConfigRead(s, store.(*sentry.Store), id)
	}
	action.read = func(s testconformance.ScenarioContext, id string) (any, error) {
		store, err := sentryFactory(s)
		if err != nil {
			return nil, err
		}
		return sentryConfigRead(s, store.(*sentry.Store), id)
	}
	action.update = func(s testconformance.ScenarioContext, id string, _ any) error {
		store, err := sentryFactory(s)
		if err != nil {
			return err
		}
		cfg, err := sentryConfigRead(s, store.(*sentry.Store), id)
		if err != nil {
			return err
		}
		cfg.Name = "Updated " + id
		return store.(*sentry.Store).UpdateInstance(s.Context, cfg)
	}
	action.delete = func(s testconformance.ScenarioContext, id string) error {
		store, err := sentryFactory(s)
		if err != nil {
			return err
		}
		return store.(*sentry.Store).DeleteInstance(s.Context, id)
	}
	action.assertDeleted = func(s testconformance.ScenarioContext, id string, _ any) error {
		store, err := sentryFactory(s)
		if err != nil {
			return err
		}
		cfg, err := store.(*sentry.Store).GetInstance(s.Context, id)
		if err != nil {
			return err
		}
		if cfg != nil {
			return fmt.Errorf("sentry config %q remains after delete", id)
		}
		return nil
	}
	action.setBoolean = func(s testconformance.ScenarioContext, id string, _ any, enabled bool) (any, error) {
		store, err := sentryFactory(s)
		if err != nil {
			return nil, err
		}
		if err := store.(*sentry.Store).UpdateAuthHealthForInstance(s.Context, id, enabled, "", time.Now().UTC()); err != nil {
			return nil, err
		}
		return sentryConfigRead(s, store.(*sentry.Store), id)
	}
	action.readBoolean = providerBoolean
	action.conflict = func(s testconformance.ScenarioContext, id string, _ any) error {
		store, err := sentryFactory(s)
		if err != nil {
			return err
		}
		if err := store.(*sentry.Store).CreateInstance(s.Context, &sentry.SentryConfig{ID: id, WorkspaceID: "duplicate", Name: "Duplicate", AuthMethod: sentry.AuthMethodAuthToken, URL: "https://duplicate.example.test"}); err == nil {
			return fmt.Errorf("duplicate Sentry instance succeeded")
		}
		return nil
	}
	return action
}

func sentryConfigRead(s testconformance.ScenarioContext, store *sentry.Store, id string) (*sentry.SentryConfig, error) {
	cfg, err := store.GetInstance(s.Context, id)
	if err != nil {
		return nil, err
	}
	return requireProviderConfig(cfg, "Sentry")
}

//nolint:funlen // This adapter enumerates every Azure DevOps store assertion.
func azureDevOpsAction() apiAction {
	action := apiAction{name: "azure_devops_configs", key: providerConfigKey}
	action.create = func(s testconformance.ScenarioContext, _ string) (any, error) {
		store, err := azureDevOpsFactory(s)
		if err != nil {
			return nil, err
		}
		workspaceID, err := conformanceWorkspaceID(s)
		if err != nil {
			return nil, err
		}
		cfg := &azuredevops.Config{WorkspaceID: workspaceID, OrganizationURL: "https://dev.azure.example.test", DefaultProjectID: "project", DefaultProjectName: "Conformance", AuthMethod: azuredevops.AuthMethodPAT}
		if err := store.(*azuredevops.Store).UpsertConfig(s.Context, cfg); err != nil {
			return nil, err
		}
		return azureDevOpsConfigRead(s, store.(*azuredevops.Store), workspaceID)
	}
	action.read = func(s testconformance.ScenarioContext, id string) (any, error) {
		store, err := azureDevOpsFactory(s)
		if err != nil {
			return nil, err
		}
		return azureDevOpsConfigRead(s, store.(*azuredevops.Store), id)
	}
	action.update = func(s testconformance.ScenarioContext, id string, _ any) error {
		store, err := azureDevOpsFactory(s)
		if err != nil {
			return err
		}
		cfg, err := azureDevOpsConfigRead(s, store.(*azuredevops.Store), id)
		if err != nil {
			return err
		}
		cfg.DefaultProjectName = "Updated"
		return store.(*azuredevops.Store).UpsertConfig(s.Context, cfg)
	}
	action.delete = func(s testconformance.ScenarioContext, id string) error {
		store, err := azureDevOpsFactory(s)
		if err != nil {
			return err
		}
		return store.(*azuredevops.Store).DeleteConfig(s.Context, id)
	}
	action.assertDeleted = func(s testconformance.ScenarioContext, id string, _ any) error {
		store, err := azureDevOpsFactory(s)
		if err != nil {
			return err
		}
		cfg, err := store.(*azuredevops.Store).GetConfig(s.Context, id)
		if err != nil {
			return err
		}
		if cfg != nil {
			return fmt.Errorf("azure DevOps config %q remains after delete", id)
		}
		return nil
	}
	action.setBoolean = func(s testconformance.ScenarioContext, id string, _ any, enabled bool) (any, error) {
		store, err := azureDevOpsFactory(s)
		if err != nil {
			return nil, err
		}
		if err := store.(*azuredevops.Store).UpdateAuthHealth(s.Context, id, enabled, "", time.Now().UTC()); err != nil {
			return nil, err
		}
		return azureDevOpsConfigRead(s, store.(*azuredevops.Store), id)
	}
	action.readBoolean = providerBoolean
	action.conflict = action.update
	return action
}

func azureDevOpsConfigRead(s testconformance.ScenarioContext, store *azuredevops.Store, workspaceID string) (*azuredevops.Config, error) {
	cfg, err := store.GetConfig(s.Context, workspaceID)
	if err != nil {
		return nil, err
	}
	return requireProviderConfig(cfg, "Azure DevOps")
}

//nolint:funlen // This adapter enumerates every workflow-sync store assertion.
func workflowSyncAction() apiAction {
	action := apiAction{name: "workflow_sync_configs", key: providerConfigKey}
	request := func(owner, repo string, enabled bool) *workflowsync.SetConfigRequest {
		return &workflowsync.SetConfigRequest{
			Provider:        workflowsync.ProviderGitHub,
			RepoOwner:       owner,
			RepoName:        repo,
			Branch:          workflowsync.DefaultBranch,
			Path:            workflowsync.DefaultPath,
			IntervalSeconds: workflowsync.DefaultIntervalSeconds,
			PollEnabled:     boolPointer(enabled),
		}
	}
	readConfig := func(s testconformance.ScenarioContext, store *workflowsync.Store, workspaceID string) (*workflowsync.Config, error) {
		cfg, err := store.GetConfigForWorkspace(s.Context, workspaceID)
		if err != nil {
			return nil, err
		}
		return requireProviderConfig(cfg, "workflow sync")
	}
	action.create = func(s testconformance.ScenarioContext, _ string) (any, error) {
		store, err := workflowSyncFactory(s)
		if err != nil {
			return nil, err
		}
		workspaceID, err := conformanceWorkspaceID(s)
		if err != nil {
			return nil, err
		}
		req := request("conformance", "workflow-sync", true)
		if err := req.Normalize(); err != nil {
			return nil, err
		}
		cfg, err := store.(*workflowsync.Store).UpsertConfigForWorkspace(s.Context, workspaceID, req)
		if err != nil {
			return nil, err
		}
		return requireProviderConfig(cfg, "workflow sync")
	}
	action.read = func(s testconformance.ScenarioContext, id string) (any, error) {
		store, err := workflowSyncFactory(s)
		if err != nil {
			return nil, err
		}
		return readConfig(s, store.(*workflowsync.Store), id)
	}
	action.update = func(s testconformance.ScenarioContext, id string, _ any) error {
		store, err := workflowSyncFactory(s)
		if err != nil {
			return err
		}
		req := request("updated", "workflow-sync", true)
		if err := req.Normalize(); err != nil {
			return err
		}
		_, err = store.(*workflowsync.Store).UpsertConfigForWorkspace(s.Context, id, req)
		return err
	}
	action.delete = func(s testconformance.ScenarioContext, id string) error {
		store, err := workflowSyncFactory(s)
		if err != nil {
			return err
		}
		return store.(*workflowsync.Store).DeleteConfigForWorkspace(s.Context, id)
	}
	action.assertDeleted = func(s testconformance.ScenarioContext, id string, _ any) error {
		store, err := workflowSyncFactory(s)
		if err != nil {
			return err
		}
		cfg, err := store.(*workflowsync.Store).GetConfigForWorkspace(s.Context, id)
		if err != nil {
			return err
		}
		if cfg != nil {
			return fmt.Errorf("workflow sync config %q remains after delete", id)
		}
		return nil
	}
	action.setBoolean = func(s testconformance.ScenarioContext, id string, _ any, enabled bool) (any, error) {
		store, err := workflowSyncFactory(s)
		if err != nil {
			return nil, err
		}
		if err := store.(*workflowsync.Store).RecordSyncStatus(s.Context, id, enabled, "", nil, "hash", time.Now().UTC(), authcircuit.State{}); err != nil {
			return nil, err
		}
		return readConfig(s, store.(*workflowsync.Store), id)
	}
	action.readBoolean = providerBoolean
	action.conflict = action.update
	return action
}

//nolint:funlen // This adapter enumerates every office-config-sync assertion.
func officeConfigSyncAction() apiAction {
	action := apiAction{name: "office_config_sync_configs", key: providerConfigKey}
	path := ""
	request := func(owner, repo string, enabled bool) *configsync.SetConfigRequest {
		return &configsync.SetConfigRequest{
			Provider:        configsync.ProviderGitHub,
			RepoOwner:       owner,
			RepoName:        repo,
			Branch:          configsync.DefaultBranch,
			Path:            &path,
			IntervalSeconds: configsync.DefaultIntervalSeconds,
			PollEnabled:     boolPointer(enabled),
		}
	}
	readConfig := func(s testconformance.ScenarioContext, store *configsync.Store, workspaceID string) (*configsync.Config, error) {
		cfg, err := store.GetConfigForWorkspace(s.Context, workspaceID)
		if err != nil {
			return nil, err
		}
		return requireProviderConfig(cfg, "Office config sync")
	}
	action.create = func(s testconformance.ScenarioContext, _ string) (any, error) {
		store, err := officeConfigSyncFactory(s)
		if err != nil {
			return nil, err
		}
		workspaceID := "conformance-office-workspace"
		req := request("conformance", "office-sync", true)
		if err := req.Normalize(); err != nil {
			return nil, err
		}
		cfg, err := store.(*configsync.Store).UpsertConfigForWorkspace(s.Context, workspaceID, req)
		if err != nil {
			return nil, err
		}
		return requireProviderConfig(cfg, "Office config sync")
	}
	action.read = func(s testconformance.ScenarioContext, id string) (any, error) {
		store, err := officeConfigSyncFactory(s)
		if err != nil {
			return nil, err
		}
		return readConfig(s, store.(*configsync.Store), id)
	}
	action.update = func(s testconformance.ScenarioContext, id string, _ any) error {
		store, err := officeConfigSyncFactory(s)
		if err != nil {
			return err
		}
		req := request("updated", "office-sync", true)
		if err := req.Normalize(); err != nil {
			return err
		}
		_, err = store.(*configsync.Store).UpsertConfigForWorkspace(s.Context, id, req)
		return err
	}
	action.delete = func(s testconformance.ScenarioContext, id string) error {
		store, err := officeConfigSyncFactory(s)
		if err != nil {
			return err
		}
		return store.(*configsync.Store).DeleteConfigForWorkspace(s.Context, id)
	}
	action.assertDeleted = func(s testconformance.ScenarioContext, id string, _ any) error {
		store, err := officeConfigSyncFactory(s)
		if err != nil {
			return err
		}
		cfg, err := store.(*configsync.Store).GetConfigForWorkspace(s.Context, id)
		if err != nil {
			return err
		}
		if cfg != nil {
			return fmt.Errorf("office config sync config %q remains after delete", id)
		}
		return nil
	}
	action.setBoolean = func(s testconformance.ScenarioContext, id string, _ any, enabled bool) (any, error) {
		store, err := officeConfigSyncFactory(s)
		if err != nil {
			return nil, err
		}
		if err := store.(*configsync.Store).RecordSyncStatus(s.Context, id, enabled, "", nil, "hash", time.Now().UTC()); err != nil {
			return nil, err
		}
		return readConfig(s, store.(*configsync.Store), id)
	}
	action.readBoolean = providerBoolean
	action.conflict = action.update
	return action
}

//nolint:cyclop,funlen // This adapter asserts the complete automation store contract.
func automationAction() apiAction {
	action := apiAction{name: "automations", key: func(record any, fallback string) string {
		if value, ok := record.(*automation.Automation); ok && value.ID != "" {
			return value.ID
		}
		return fallback
	}}
	action.create = func(s testconformance.ScenarioContext, id string) (any, error) {
		store, err := automationFactory(s)
		if err != nil {
			return nil, err
		}
		workspaceID, err := conformanceWorkspaceID(s)
		if err != nil {
			return nil, err
		}
		record := &automation.Automation{
			ID: id, WorkspaceID: workspaceID, Name: "Conformance " + id,
			TaskMode: automation.TaskModeAutomationRun, RepositoryMode: automation.RepositoryModeNone,
			AgentProfileID: "conformance-agent", ExecutorProfileID: "conformance-executor",
			Prompt: "conformance prompt", TaskTitleTemplate: "Conformance task",
			MaxConcurrentRuns: 1, ContinuationPolicy: automation.ContinuationPolicyNewTask,
		}
		if err := store.(*automation.Store).CreateAutomation(s.Context, record); err != nil {
			return nil, err
		}
		return automationRead(s, store.(*automation.Store), id)
	}
	action.read = func(s testconformance.ScenarioContext, id string) (any, error) {
		store, err := automationFactory(s)
		if err != nil {
			return nil, err
		}
		return automationRead(s, store.(*automation.Store), id)
	}
	action.update = func(s testconformance.ScenarioContext, id string, _ any) error {
		store, err := automationFactory(s)
		if err != nil {
			return err
		}
		name := "Updated " + id
		return store.(*automation.Store).UpdateAutomation(s.Context, id, &automation.UpdateAutomationRequest{Name: &name})
	}
	action.delete = func(s testconformance.ScenarioContext, id string) error {
		store, err := automationFactory(s)
		if err != nil {
			return err
		}
		return store.(*automation.Store).DeleteAutomation(s.Context, id)
	}
	action.assertDeleted = func(s testconformance.ScenarioContext, id string, _ any) error {
		store, err := automationFactory(s)
		if err != nil {
			return err
		}
		value, err := store.(*automation.Store).GetAutomation(s.Context, id)
		if err != nil {
			return err
		}
		if value != nil {
			return fmt.Errorf("automation %q remains after delete", id)
		}
		return nil
	}
	action.setBoolean = func(s testconformance.ScenarioContext, id string, _ any, enabled bool) (any, error) {
		store, err := automationFactory(s)
		if err != nil {
			return nil, err
		}
		if err := store.(*automation.Store).UpdateAutomation(s.Context, id, &automation.UpdateAutomationRequest{Enabled: boolPointer(enabled)}); err != nil {
			return nil, err
		}
		return automationRead(s, store.(*automation.Store), id)
	}
	action.readBoolean = providerBoolean
	action.conflict = func(s testconformance.ScenarioContext, id string, _ any) error {
		store, err := automationFactory(s)
		if err != nil {
			return err
		}
		if err := store.(*automation.Store).CreateAutomation(s.Context, &automation.Automation{ID: id, WorkspaceID: "duplicate", Name: "Duplicate", TaskMode: automation.TaskModeAutomationRun, RepositoryMode: automation.RepositoryModeNone, AgentProfileID: "a", ExecutorProfileID: "e", MaxConcurrentRuns: 1, ContinuationPolicy: automation.ContinuationPolicyNewTask}); err == nil {
			return fmt.Errorf("duplicate automation succeeded")
		}
		return nil
	}
	action.transaction = func(s testconformance.ScenarioContext, id string) error {
		return transactionAPICheck(action, s, id)
	}
	return action
}

func automationRead(s testconformance.ScenarioContext, store *automation.Store, id string) (*automation.Automation, error) {
	value, err := store.GetAutomation(s.Context, id)
	if err != nil {
		return nil, err
	}
	return requireProviderConfig(value, "automation")
}

func reflectValue(record any) reflect.Value {
	return reflect.ValueOf(record).Elem()
}
