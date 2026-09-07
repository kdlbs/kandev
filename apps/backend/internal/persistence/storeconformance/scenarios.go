//nolint:revive // The shared scenario runner keeps all cross-owner assertions together.
package storeconformance

import (
	"context"
	"database/sql"
	"fmt"
	"reflect"
	"strings"
	"time"

	settingsstore "github.com/kandev/kandev/internal/agent/settings/store"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/persistence/requiredstores"
	taskrepository "github.com/kandev/kandev/internal/task/repository"
	testconformance "github.com/kandev/kandev/internal/testutil/storeconformance"
	userstore "github.com/kandev/kandev/internal/user/store"
	workflowrepository "github.com/kandev/kandev/internal/workflow/repository"

	editorstore "github.com/kandev/kandev/internal/editors/store"
	notificationstore "github.com/kandev/kandev/internal/notifications/store"
	promptstore "github.com/kandev/kandev/internal/prompts/store"
	utilitystore "github.com/kandev/kandev/internal/utility/store"
)

// apiAction is a behavior probe for one real owner API. The conformance suite
// deliberately keeps the schema initializer and the behavior API separate:
// an adapter must both create the schema and prove that its owning repository
// can persist a valid record in it.
type apiAction struct {
	name          string
	key           func(any, string) string
	create        func(testconformance.ScenarioContext, string) (any, error)
	read          func(testconformance.ScenarioContext, string) (any, error)
	update        func(testconformance.ScenarioContext, string, any) error
	assertUpdated func(before, after any, id string) error
	delete        func(testconformance.ScenarioContext, string) error
	assertDeleted func(testconformance.ScenarioContext, string, any) error
	setBoolean    func(testconformance.ScenarioContext, string, any, bool) (any, error)
	readBoolean   func(any) (bool, bool)
	transaction   func(testconformance.ScenarioContext, string) error
	conflict      func(testconformance.ScenarioContext, string, any) error
	// nullableTimestampUpdates lists fields that this owner's update API is
	// expected to populate or clear. All other nullable timestamps must retain
	// their NULL state across the update.
	nullableTimestampUpdates []string
}

type ownerBehavior struct {
	actions []apiAction
}

type storeFactory func(testconformance.ScenarioContext) (any, error)

// reflectedSpec describes the common repository shape used by the small
// stores: Create(ctx, *Record), Get(ctx, id), Update(ctx, *Record), and
// Delete(ctx, id). Reflection is confined to this test adapter so production
// stores remain strongly typed while every adapter still calls their real API.
type reflectedSpec struct {
	name            string
	factory         storeFactory
	createMethod    string
	readMethod      string
	updateMethod    string
	deleteMethod    string
	createArgs      func(string, any) []any
	readArgs        func(string) []any
	updateArgs      func(string, any) []any
	deleteArgs      func(string) []any
	populate        func(any, string)
	booleanFields   []string
	updateMutation  func(any, string)
	updateAssertion func(before, after any, id string) error
}

func behaviorScenarios(descriptor requiredstores.Descriptor) testconformance.Scenarios {
	behavior, ok := ownerBehaviors[descriptor.ID]
	if !ok {
		panic(fmt.Sprintf("no real behavior adapter for %q", descriptor.ID))
	}
	scenarios := testconformance.Scenarios{CRUD: ownerCRUDScenario(descriptor, behavior)}
	for _, capability := range descriptor.Capabilities {
		scenarios.Capabilities = append(scenarios.Capabilities, testconformance.CapabilityScenario{
			Capability: capability,
			Run:        ownerCapabilityScenario(descriptor, behavior, capability),
		})
	}
	return scenarios
}

func ownerCRUDScenario(descriptor requiredstores.Descriptor, behavior ownerBehavior) testconformance.Scenario {
	return func(s testconformance.ScenarioContext) error {
		if len(behavior.actions) == 0 {
			return fmt.Errorf("%s has no owning API actions", descriptor.ID)
		}
		for index, action := range behavior.actions {
			if err := exerciseAction(s, action, fmt.Sprintf("%s-%d", descriptor.ID, index)); err != nil {
				return fmt.Errorf("%s CRUD (%s): %w", descriptor.ID, action.name, err)
			}
		}
		return nil
	}
}

func ownerCapabilityScenario(
	descriptor requiredstores.Descriptor,
	behavior ownerBehavior,
	capability requiredstores.Capability,
) testconformance.Scenario {
	return func(s testconformance.ScenarioContext) error {
		ran := false
		for index, action := range behavior.actions {
			if !supportsCapability(action, capability) {
				continue
			}
			ran = true
			if err := exerciseCapability(s, action, capability, fmt.Sprintf("%s-%d", descriptor.ID, index)); err != nil {
				return fmt.Errorf("%s %s (%s): %w", descriptor.ID, capability, action.name, err)
			}
		}
		if !ran {
			return fmt.Errorf("%s has no owning API action for %s", descriptor.ID, capability)
		}
		return nil
	}
}

func supportsCapability(action apiAction, capability requiredstores.Capability) bool {
	switch capability {
	case requiredstores.CapabilityBoolean:
		return action.setBoolean != nil && action.readBoolean != nil
	case requiredstores.CapabilityTransaction:
		return action.transaction != nil
	default:
		return true
	}
}

// transactionAPICheck proves both sides of the owning repository's write
// boundary. The update must be visible after the repository commits, while a
// duplicate create must fail without removing the already committed record.
// This deliberately calls the owner API instead of issuing a synthetic SQL
// statement against an empty test table.
func transactionAPICheck(action apiAction, s testconformance.ScenarioContext, id string) error {
	before, err := action.read(s, id)
	if err != nil {
		return fmt.Errorf("read before transaction: %w", err)
	}
	if !recordPresent(before) {
		return fmt.Errorf("read before transaction returned no record")
	}
	if err := action.update(s, id, before); err != nil {
		return fmt.Errorf("committed update: %w", err)
	}
	committed, err := action.read(s, id)
	if err != nil {
		return fmt.Errorf("read committed update: %w", err)
	}
	if !recordPresent(committed) {
		return fmt.Errorf("committed update cannot be read")
	}
	if _, err := action.create(s, id); err == nil {
		return fmt.Errorf("duplicate create succeeded")
	}
	retained, err := action.read(s, id)
	if err != nil {
		return fmt.Errorf("read after rolled-back duplicate: %w", err)
	}
	if !recordPresent(retained) {
		return fmt.Errorf("rolled-back duplicate removed the committed record")
	}
	return nil
}

func recordPresent(record any) bool {
	if record == nil {
		return false
	}
	value := reflect.ValueOf(record)
	switch value.Kind() {
	case reflect.Pointer, reflect.Interface, reflect.Map, reflect.Slice:
		return !value.IsNil()
	default:
		return true
	}
}

//nolint:cyclop,gocognit // The scenario verifies the complete CRUD lifecycle.
func exerciseAction(s testconformance.ScenarioContext, action apiAction, suffix string) error {
	if action.create == nil || action.read == nil || action.update == nil || action.delete == nil {
		return fmt.Errorf("incomplete API action")
	}
	id := behaviorID(s, action.name+"-"+suffix)
	record, err := action.create(s, id)
	if err != nil {
		return fmt.Errorf("create: %w", err)
	}
	if record == nil {
		return fmt.Errorf("create returned no record")
	}
	key := id
	if action.key != nil {
		key = action.key(record, id)
	}
	read, err := action.read(s, key)
	if err != nil {
		return fmt.Errorf("read: %w", err)
	}
	if read == nil {
		return fmt.Errorf("read returned no record")
	}
	if err := action.update(s, key, record); err != nil {
		return fmt.Errorf("update: %w", err)
	}
	updated, err := action.read(s, key)
	if err != nil {
		return fmt.Errorf("read after update: %w", err)
	} else if updated == nil {
		return fmt.Errorf("read after update returned no record")
	}
	if action.assertUpdated != nil {
		if err := action.assertUpdated(read, updated, key); err != nil {
			return fmt.Errorf("assert update: %w", err)
		}
	}
	if err := action.delete(s, key); err != nil {
		return fmt.Errorf("delete: %w", err)
	}
	if action.assertDeleted != nil {
		if err := action.assertDeleted(s, key, record); err != nil {
			return fmt.Errorf("assert delete: %w", err)
		}
		return nil
	}
	if _, err := action.read(s, key); err == nil {
		return fmt.Errorf("read after delete succeeded")
	}
	return nil
}

//nolint:cyclop,gocognit,funlen // Capability probes intentionally cover every failure branch.
func exerciseCapability(
	s testconformance.ScenarioContext,
	action apiAction,
	capability requiredstores.Capability,
	suffix string,
) error {
	id := behaviorID(s, action.name+"-"+string(capability)+"-"+suffix)
	record, err := action.create(s, id)
	if err != nil {
		return fmt.Errorf("create: %w", err)
	}
	if record == nil {
		return fmt.Errorf("create returned no record")
	}
	key := id
	if action.key != nil {
		key = action.key(record, id)
	}
	defer func() { _ = action.delete(s, key) }()

	switch capability {
	case requiredstores.CapabilityBoolean:
		if action.setBoolean == nil || action.readBoolean == nil {
			return fmt.Errorf("action has no boolean API assertion")
		}
		for _, want := range []bool{false, true} {
			record, err = action.setBoolean(s, key, record, want)
			if err != nil {
				return fmt.Errorf("set %t: %w", want, err)
			}
			got, ok := action.readBoolean(record)
			if !ok {
				return fmt.Errorf("boolean value is not exposed by the owning API")
			}
			if got != want {
				return fmt.Errorf("boolean = %t, want %t", got, want)
			}
		}
	case requiredstores.CapabilityTimestamp:
		before, err := action.read(s, key)
		if err != nil {
			return fmt.Errorf("read timestamp record: %w", err)
		}
		created, updatedAt, ok := recordTimestamps(before)
		if !ok {
			return fmt.Errorf("timestamp fields are not exposed by the owning API")
		}
		if created.IsZero() || updatedAt.IsZero() {
			return fmt.Errorf("timestamp values are zero: created=%v updated=%v", created, updatedAt)
		}
		if created.Location() != time.UTC || updatedAt.Location() != time.UTC {
			return fmt.Errorf("timestamps are not UTC: created=%s updated=%s", created.Location(), updatedAt.Location())
		}
		if updatedAt.Before(created) {
			return fmt.Errorf("updated timestamp %v precedes created timestamp %v", updatedAt, created)
		}
		// Compare the owning API's read before and after a real update. This
		// catches stores that expose timestamps but never persist updates.
		nullBefore := nilTimestampFields(before)
		if err := action.update(s, key, before); err != nil {
			return fmt.Errorf("update timestamp record: %w", err)
		}
		after, err := action.read(s, key)
		if err != nil {
			return fmt.Errorf("read timestamp record after update: %w", err)
		}
		createdAfter, updatedAfter, ok := recordTimestamps(after)
		if !ok {
			return fmt.Errorf("updated record does not expose timestamps")
		}
		if createdAfter.Location() != time.UTC || updatedAfter.Location() != time.UTC {
			return fmt.Errorf("updated timestamps are not UTC: created=%s updated=%s", createdAfter.Location(), updatedAfter.Location())
		}
		if updatedAfter.Before(updatedAt) {
			return fmt.Errorf("updated timestamp %v regressed from %v", updatedAfter, updatedAt)
		}
		if got := nilTimestampFields(after); !sameStrings(
			withoutFields(nullBefore, action.nullableTimestampUpdates),
			withoutFields(got, action.nullableTimestampUpdates),
		) {
			return fmt.Errorf("nullable timestamp fields were not preserved on update: before=%v after=%v", nullBefore, got)
		}
	case requiredstores.CapabilityConflict:
		if action.conflict != nil {
			if err := action.conflict(s, key, record); err != nil {
				return err
			}
		} else if _, err := action.create(s, id); err == nil {
			return fmt.Errorf("duplicate create succeeded")
		}
	case requiredstores.CapabilityTransaction:
		if action.transaction == nil {
			return fmt.Errorf("action has no transaction API assertion")
		}
		if err := action.transaction(s, id); err != nil {
			return err
		}
	default:
		return fmt.Errorf("unsupported capability %q", capability)
	}
	return nil
}

//nolint:cyclop,gocognit,funlen // Reflection adapters must cover each repository method boundary.
func standardAction(spec reflectedSpec) apiAction {
	populate := spec.populate
	if populate == nil {
		populate = populateRecord
	}
	defaultUpdate := spec.updateArgs == nil
	createArgs := spec.createArgs
	if createArgs == nil {
		createArgs = func(_ string, record any) []any { return []any{record} }
	}
	readArgs := spec.readArgs
	if readArgs == nil {
		readArgs = func(id string) []any { return []any{id} }
	}
	updateArgs := spec.updateArgs
	if updateArgs == nil {
		updateArgs = func(_ string, record any) []any { return []any{record} }
	}
	deleteArgs := spec.deleteArgs
	if deleteArgs == nil {
		deleteArgs = func(id string) []any { return []any{id} }
	}
	action := apiAction{name: spec.name, key: recordIdentifier}
	action.create = func(s testconformance.ScenarioContext, id string) (any, error) {
		store, err := spec.factory(s)
		if err != nil {
			return nil, err
		}
		record, err := newRecordForMethod(store, spec.createMethod)
		if err != nil {
			return nil, err
		}
		populate(record, id)
		outputs, err := invoke(store, spec.createMethod, s.Context, createArgs(id, record)...)
		if err != nil {
			return nil, err
		}
		for _, output := range outputs {
			if output.IsValid() && output.CanInterface() && output.Type() != reflect.TypeOf((*error)(nil)).Elem() {
				return output.Interface(), nil
			}
		}
		return record, nil
	}
	action.read = func(s testconformance.ScenarioContext, id string) (any, error) {
		store, err := spec.factory(s)
		if err != nil {
			return nil, err
		}
		out, err := invoke(store, spec.readMethod, s.Context, readArgs(id)...)
		if err != nil {
			return nil, err
		}
		if len(out) == 0 {
			return nil, fmt.Errorf("%s returned no record", spec.readMethod)
		}
		return out[0].Interface(), nil
	}
	action.update = func(s testconformance.ScenarioContext, id string, record any) error {
		store, err := spec.factory(s)
		if err != nil {
			return err
		}
		if spec.updateMutation != nil {
			spec.updateMutation(record, id)
		} else if defaultUpdate {
			markRecordUpdated(record, id)
		}
		_, err = invoke(store, spec.updateMethod, s.Context, updateArgs(id, record)...)
		return err
	}
	if spec.updateAssertion != nil {
		action.assertUpdated = spec.updateAssertion
	} else if defaultUpdate {
		action.assertUpdated = func(_ any, after any, id string) error {
			name, ok := recordStringField(after, "Name")
			if !ok || name != "updated-"+id {
				return fmt.Errorf("name = %q, want %q", name, "updated-"+id)
			}
			return nil
		}
	}
	action.delete = func(s testconformance.ScenarioContext, id string) error {
		store, err := spec.factory(s)
		if err != nil {
			return err
		}
		_, err = invoke(store, spec.deleteMethod, s.Context, deleteArgs(id)...)
		return err
	}
	if len(spec.booleanFields) > 0 {
		action.setBoolean = func(s testconformance.ScenarioContext, id string, record any, enabled bool) (any, error) {
			if !setRecordBoolean(record, spec.booleanFields, enabled) {
				return record, fmt.Errorf("no boolean field in %s", spec.name)
			}
			store, err := spec.factory(s)
			if err != nil {
				return record, err
			}
			if _, err := invoke(store, spec.updateMethod, s.Context, updateArgs(id, record)...); err != nil {
				return record, err
			}
			read, err := action.read(s, id)
			return read, err
		}
		action.readBoolean = func(record any) (bool, bool) {
			return recordBoolean(record, spec.booleanFields)
		}
	}
	action.conflict = func(s testconformance.ScenarioContext, id string, _ any) error {
		if _, err := action.create(s, id); err == nil {
			return fmt.Errorf("duplicate create succeeded")
		}
		return nil
	}
	action.transaction = func(s testconformance.ScenarioContext, id string) error {
		return transactionAPICheck(action, s, id)
	}
	return action
}

func invoke(store any, methodName string, ctx context.Context, args ...any) ([]reflect.Value, error) {
	method := reflect.ValueOf(store).MethodByName(methodName)
	if !method.IsValid() {
		return nil, fmt.Errorf("store %T has no method %s", store, methodName)
	}
	methodType := method.Type()
	if methodType.NumIn() != len(args)+1 {
		return nil, fmt.Errorf("%s expects %d arguments after context, got %d", methodName, methodType.NumIn()-1, len(args))
	}
	values := make([]reflect.Value, 0, len(args)+1)
	values = append(values, reflect.ValueOf(ctx))
	for index, arg := range args {
		value, err := reflectArgument(arg, methodType.In(index+1))
		if err != nil {
			return nil, fmt.Errorf("%s argument %d: %w", methodName, index, err)
		}
		values = append(values, value)
	}
	outputs := method.Call(values)
	if len(outputs) > 0 {
		errorType := reflect.TypeOf((*error)(nil)).Elem()
		last := outputs[len(outputs)-1]
		if last.Type().Implements(errorType) && !last.IsNil() {
			return nil, last.Interface().(error)
		}
	}
	return outputs, nil
}

func invokeError(store any, methodName string, ctx context.Context, args ...any) error {
	_, err := invoke(store, methodName, ctx, args...)
	return err
}

func invokeRecord(store any, methodName string, ctx context.Context, args ...any) (any, error) {
	outputs, err := invoke(store, methodName, ctx, args...)
	if err != nil {
		return nil, err
	}
	if len(outputs) == 0 || !outputs[0].IsValid() || !outputs[0].CanInterface() {
		return nil, fmt.Errorf("%s returned no record", methodName)
	}
	return outputs[0].Interface(), nil
}

func reflectArgument(arg any, target reflect.Type) (reflect.Value, error) {
	if arg == nil {
		return reflect.Zero(target), nil
	}
	value := reflect.ValueOf(arg)
	if value.Type().AssignableTo(target) {
		return value, nil
	}
	if value.Kind() == reflect.Pointer && target.Kind() == reflect.Struct && value.Elem().IsValid() && value.Elem().Type().AssignableTo(target) {
		return value.Elem(), nil
	}
	if value.Kind() == reflect.Struct && target.Kind() == reflect.Pointer && value.Type().AssignableTo(target.Elem()) {
		copy := reflect.New(target.Elem())
		copy.Elem().Set(value)
		return copy, nil
	}
	if value.Type().ConvertibleTo(target) {
		return value.Convert(target), nil
	}
	return reflect.Value{}, fmt.Errorf("%T is not assignable to %s", arg, target)
}

func newRecordForMethod(store any, methodName string) (any, error) {
	method := reflect.ValueOf(store).MethodByName(methodName)
	if !method.IsValid() {
		return nil, fmt.Errorf("store %T has no method %s", store, methodName)
	}
	for index := 1; index < method.Type().NumIn(); index++ {
		argument := method.Type().In(index)
		if argument.Kind() == reflect.Pointer && argument.Elem().Kind() == reflect.Struct {
			return reflect.New(argument.Elem()).Interface(), nil
		}
		if argument.Kind() == reflect.Struct {
			return reflect.New(argument).Interface(), nil
		}
	}
	for index := 0; index < method.Type().NumOut(); index++ {
		output := method.Type().Out(index)
		if output.Kind() == reflect.Pointer && output.Elem().Kind() == reflect.Struct {
			return reflect.New(output.Elem()).Interface(), nil
		}
		if output.Kind() == reflect.Struct {
			return reflect.New(output).Interface(), nil
		}
	}
	return nil, fmt.Errorf("%s has no pointer record argument", methodName)
}

//nolint:funlen // The reflection fallback populates the varied record shapes used by stores.
func populateRecord(record any, id string) {
	value := reflect.ValueOf(record)
	if value.Kind() != reflect.Pointer || value.IsNil() {
		return
	}
	value = value.Elem()
	if value.Kind() != reflect.Struct {
		return
	}
	now := time.Now().UTC()
	setStringField(value, "ID", id)
	setStringField(value, "Name", "conformance-"+id)
	setStringField(value, "Description", "conformance record")
	setStringField(value, "Content", "conformance content")
	setStringField(value, "Prompt", "conformance prompt")
	setStringField(value, "Email", id+"@example.test")
	setStringField(value, "DisplayName", "Conformance Record")
	setStringField(value, "OwnerID", userstore.DefaultUserID)
	setStringField(value, "UserID", userstore.DefaultUserID)
	setStringField(value, "Type", "local")
	setStringField(value, "Kind", "conformance")
	setStringField(value, "Command", "conformance")
	setStringField(value, "Scheme", "conformance")
	setStringField(value, "Model", "conformance-model")
	setStringField(value, "Role", "member")
	setStringField(value, "Status", "active")
	setStringField(value, "Provider", "local")
	setStringField(value, "Backend", "conformance")
	setStringField(value, "State", "active")
	setStringField(value, "CreatedBy", userstore.DefaultUserID)
	setStringField(value, "URL", "https://example.test/"+id)
	setStringField(value, "Path", "/"+id+"/")
	setTimeField(value, "CreatedAt", now)
	setTimeField(value, "UpdatedAt", now)
	setTimeField(value, "ExpiresAt", now.Add(time.Hour))
	setTimeField(value, "LastSeenAt", now)
	setTimeField(value, "ResolvedAt", now)
	setTimeField(value, "OccurredAt", now)
	setTimeField(value, "CheckedAt", now)
	setStringField(value, "WorkspaceID", "conformance-workspace")
	setStringField(value, "OrgID", "conformance-org")
	setStringField(value, "AgentID", "conformance-agent")
	setStringField(value, "PluginID", "conformance-plugin")
	setStringField(value, "SourceKind", "installed")
	setStringField(value, "ScopeKind", "instance")
	setStringField(value, "ContinuationPolicy", "new_task")
	setStringField(value, "TaskMode", "automation_run")
	setStringField(value, "RepositoryMode", "none")
	setStringField(value, "ValidationStatus", "valid")
	setStringField(value, "Config", "{}")
	setStringField(value, "Settings", "{}")
	setBoolField(value, "Enabled", false)
	setBoolField(value, "Active", false)
	setBoolField(value, "Builtin", false)
	setBoolField(value, "IsDefault", false)
	setBoolField(value, "SupportsMCP", false)
	setBoolField(value, "AutoApprove", false)
}

func markRecordUpdated(record any, id string) {
	value := reflect.ValueOf(record)
	if value.Kind() == reflect.Pointer && !value.IsNil() {
		value = value.Elem()
	}
	if value.IsValid() && value.Kind() == reflect.Struct {
		setStringField(value, "Name", "updated-"+id)
	}
}

func setStringField(value reflect.Value, name, text string) {
	field := value.FieldByName(name)
	if field.IsValid() && field.CanSet() && field.Kind() == reflect.String && text != "" {
		field.SetString(text)
	}
}

func setTimeField(value reflect.Value, name string, at time.Time) {
	field := value.FieldByName(name)
	if field.IsValid() && field.CanSet() && field.Type() == reflect.TypeOf(time.Time{}) {
		field.Set(reflect.ValueOf(at))
	}
}

func setBoolField(value reflect.Value, name string, enabled bool) {
	field := value.FieldByName(name)
	if field.IsValid() && field.CanSet() && field.Kind() == reflect.Bool {
		field.SetBool(enabled)
	}
}

func setRecordBoolean(record any, preferred []string, enabled bool) bool {
	value := reflect.ValueOf(record)
	if value.Kind() == reflect.Pointer && value.IsNil() {
		return false
	}
	if value.Kind() == reflect.Pointer {
		value = value.Elem()
	}
	if value.Kind() != reflect.Struct {
		return false
	}
	for _, name := range preferred {
		field := value.FieldByName(name)
		if field.IsValid() && field.CanSet() && field.Kind() == reflect.Bool {
			field.SetBool(enabled)
			return true
		}
	}
	for index := 0; index < value.NumField(); index++ {
		field := value.Field(index)
		if field.CanSet() && field.Kind() == reflect.Bool {
			field.SetBool(enabled)
			return true
		}
	}
	return false
}

func recordBoolean(record any, preferred []string) (bool, bool) {
	value := reflect.ValueOf(record)
	if value.Kind() == reflect.Pointer && value.IsNil() {
		return false, false
	}
	if value.Kind() == reflect.Pointer {
		value = value.Elem()
	}
	if value.Kind() != reflect.Struct {
		return false, false
	}
	for _, name := range preferred {
		field := value.FieldByName(name)
		if field.IsValid() && field.Kind() == reflect.Bool {
			return field.Bool(), true
		}
	}
	for index := 0; index < value.NumField(); index++ {
		field := value.Field(index)
		if field.Kind() == reflect.Bool {
			return field.Bool(), true
		}
	}
	return false, false
}

func recordIdentifier(record any, fallback string) string {
	value := reflect.ValueOf(record)
	if value.Kind() == reflect.Pointer && !value.IsNil() {
		value = value.Elem()
	}
	if value.IsValid() && value.Kind() == reflect.Struct {
		field := value.FieldByName("ID")
		if field.IsValid() && field.Kind() == reflect.String && field.String() != "" {
			return field.String()
		}
	}
	return fallback
}

func recordStringField(record any, name string) (string, bool) {
	value := reflect.ValueOf(record)
	if value.Kind() == reflect.Pointer {
		if value.IsNil() {
			return "", false
		}
		value = value.Elem()
	}
	if !value.IsValid() || value.Kind() != reflect.Struct {
		return "", false
	}
	field := value.FieldByName(name)
	if !field.IsValid() || !field.CanInterface() {
		return "", false
	}
	if field.Kind() == reflect.String {
		return field.String(), true
	}
	if field.Kind() == reflect.Pointer && !field.IsNil() && field.Elem().Kind() == reflect.String {
		return field.Elem().String(), true
	}
	return "", false
}

func recordTimestamps(record any) (time.Time, time.Time, bool) {
	value := reflect.ValueOf(record)
	if value.Kind() == reflect.Pointer && value.IsNil() {
		return time.Time{}, time.Time{}, false
	}
	if value.Kind() == reflect.Pointer {
		value = value.Elem()
	}
	if value.Kind() != reflect.Struct {
		return time.Time{}, time.Time{}, false
	}
	created, createdOK := firstTimeField(value, "CreatedAt", "ActivatedAt", "QueuedAt", "ResolvedAt", "OccurredAt", "StartedAt", "LastCheckedAt", "UpdatedAt")
	updated, updatedOK := firstTimeField(value, "UpdatedAt", "LastCheckedAt", "ResolvedAt", "OccurredAt", "QueuedAt", "ActivatedAt", "CreatedAt")
	return created, updated, createdOK && updatedOK
}

func firstTimeField(value reflect.Value, names ...string) (time.Time, bool) {
	for _, name := range names {
		if at, ok := timeField(value, name); ok {
			return at, true
		}
	}
	return time.Time{}, false
}

func timeField(value reflect.Value, name string) (time.Time, bool) {
	field := value.FieldByName(name)
	if !field.IsValid() || !field.CanInterface() {
		return time.Time{}, false
	}
	if field.Type() == reflect.TypeOf(time.Time{}) {
		return field.Interface().(time.Time), true
	}
	if field.Kind() == reflect.Pointer && field.Type().Elem() == reflect.TypeOf(time.Time{}) && !field.IsNil() {
		return field.Elem().Interface().(time.Time), true
	}
	return time.Time{}, false
}

// nilTimestampFields lists nullable time values that the owning API exposed
// as NULL after creation. Keeping this check at the API boundary catches a
// driver that silently turns SQL NULL into a zero or local timestamp.
func nilTimestampFields(record any) []string {
	value := reflect.ValueOf(record)
	if value.Kind() == reflect.Pointer {
		if value.IsNil() {
			return nil
		}
		value = value.Elem()
	}
	if !value.IsValid() || value.Kind() != reflect.Struct {
		return nil
	}
	nullTimeType := reflect.TypeOf(sql.NullTime{})
	timeType := reflect.TypeOf(time.Time{})
	var fields []string
	for index := 0; index < value.NumField(); index++ {
		field := value.Field(index)
		fieldType := field.Type()
		if fieldType.Kind() == reflect.Pointer && fieldType.Elem() == timeType && field.IsNil() {
			fields = append(fields, value.Type().Field(index).Name)
			continue
		}
		if fieldType == nullTimeType && field.FieldByName("Valid").IsValid() && !field.FieldByName("Valid").Bool() {
			fields = append(fields, value.Type().Field(index).Name)
		}
	}
	return fields
}

func sameStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	counts := make(map[string]int, len(left))
	for _, value := range left {
		counts[value]++
	}
	for _, value := range right {
		counts[value]--
		if counts[value] < 0 {
			return false
		}
	}
	for _, count := range counts {
		if count != 0 {
			return false
		}
	}
	return true
}

func withoutFields(fields, excluded []string) []string {
	if len(excluded) == 0 {
		return fields
	}
	set := make(map[string]struct{}, len(excluded))
	for _, field := range excluded {
		set[field] = struct{}{}
	}
	result := make([]string, 0, len(fields))
	for _, field := range fields {
		if _, ok := set[field]; !ok {
			result = append(result, field)
		}
	}
	return result
}

func behaviorID(s testconformance.ScenarioContext, suffix string) string {
	engine := strings.ReplaceAll(string(s.Engine), "-", "_")
	return "conformance-" + engine + "-" + strings.ReplaceAll(suffix, " ", "-")
}

func taskFactory(s testconformance.ScenarioContext) (any, error) {
	repo, _, err := taskrepository.Provide(s.DB, s.DB, logger.Default())
	return repo, err
}

func workflowFactory(s testconformance.ScenarioContext) (any, error) {
	if _, _, err := taskrepository.Provide(s.DB, s.DB, logger.Default()); err != nil {
		return nil, err
	}
	return workflowrepository.NewWithDB(s.DB, s.DB, logger.Default())
}

func settingsFactory(s testconformance.ScenarioContext) (any, error) {
	repo, _, err := settingsstore.Provide(s.DB, s.DB, logger.Default())
	return repo, err
}

func userFactory(s testconformance.ScenarioContext) (any, error) {
	repo, _, err := userstore.Provide(s.DB, s.DB)
	return repo, err
}

func notificationFactory(s testconformance.ScenarioContext) (any, error) {
	repo, _, err := notificationstore.Provide(s.Context, s.DB, s.DB)
	return repo, err
}

func editorFactory(s testconformance.ScenarioContext) (any, error) {
	repo, _, err := editorstore.Provide(s.DB, s.DB)
	return repo, err
}

func promptFactory(s testconformance.ScenarioContext) (any, error) {
	repo, _, err := promptstore.Provide(s.DB, s.DB)
	return repo, err
}

func utilityFactory(s testconformance.ScenarioContext) (any, error) {
	repo, _, err := utilitystore.Provide(s.DB, s.DB)
	return repo, err
}

var ownerBehaviors = buildOwnerBehaviors()

// Keep the database pool constructor referenced in this package. More complex
// owner adapters below use it; this helper also documents that all stores are
// opened over the same writer/reader boundary as production.
func ownerPool(s testconformance.ScenarioContext) *db.Pool {
	return db.NewPool(s.DB, s.DB)
}
