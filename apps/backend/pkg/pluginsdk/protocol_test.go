package pluginsdk

import (
	"testing"

	pluginv1 "github.com/kandev/kandev/proto/kandev/plugin/v1"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/descriptorpb"
)

func TestProtocolDeclaresAdditivePluginAndHostMethods(t *testing.T) {
	services := pluginv1.File_kandev_plugin_v1_plugin_proto.Services()
	assertServiceMethods(t, services.ByName("Plugin"), "HandleAction", "SearchEntityReferences", "AuthorizeEntityReference", "ResolveGitCredential", "GetGitCredentialBinding")
	assertServiceMethods(t, services.ByName("Host"), "ListExecutorProfiles", "PreviewPluginOwnedTaskTree", "DeletePluginOwnedTaskTree")
	assertMessageFields(t, "PluginActionRequest", "action_key", "context", "body")
	assertMessageFields(t, "PluginActionResponse", "body", "headers", "status")
	assertMessageFields(t, "VerifiedActionContext", "actor_id", "workspace_id", "task_id", "repository_id", "session_id", "head_branch")
	assertMessageFields(t, "SearchEntityReferencesRequest", "source", "workspace_id", "query", "limit")
	assertMessageFields(t, "AuthorizeEntityReferenceRequest", "source", "workspace_id", "purpose", "reference")
	assertMessageFields(t, "ResolveGitCredentialRequest", "provider_id", "workspace_id", "task_id", "session_id", "repository_id", "host", "path")
	assertMessageFields(t, "GitCredentialBindingRequest", "provider_id", "workspace_id", "task_id", "session_id", "repository_id", "host", "path")
	assertMessageFields(t, "GitCredentialBindingResponse", "binding")
	assertMessageFields(t, "TaskRepository", "id", "repository_id", "base_branch", "position", "checkout_branch")
	assertMessageFields(t, "Repository", "id", "workspace_id", "name", "default_branch", "source_type", "provider_id", "provider_repository_id", "provider_host", "owner_or_project", "provider_name", "remote_url", "provider_scope")
	assertMessageOmitsFields(t, "Repository", "local_path", "setup_script", "cleanup_script", "dev_script", "copy_files")
	assertMessageFields(t, "Task", "priority", "labels")
	assertDeprecatedRepeatedMessageField(t, "Task", "labels", 23)
	assertMessageFields(t, "CreateTaskRequest", "repositories", "launch", "metadata", "priority")
	assertMessageFields(t, "UpdateTaskRequest", "priority")
	assertMessageOmitsFields(t, "CreateTaskRequest", "labels")
	assertMessageOmitsFields(t, "UpdateTaskRequest", "labels")
	assertMessageFields(t, "RemoteRepositoryDescriptor", "provider_id", "provider_host", "owner_or_project", "provider_repository_id", "name", "clone_url", "provider_scope")
	assertMessageFields(t, "DeletePluginOwnedTaskTreeProgress", "deleted_task_ids")
}

func TestReleasedTaskLabelsGeneratedSourceCompatibility(t *testing.T) {
	generated := &pluginv1.Task{Labels: []string{"legacy"}}
	native := Task{Labels: generated.GetLabels()} //nolint:staticcheck // verifies the released API v1 source surface
	if len(native.Labels) != 1 || native.Labels[0] != "legacy" {
		t.Fatalf("released Task.Labels source surface did not round-trip: %+v", native.Labels)
	}
}

func assertServiceMethods(t *testing.T, service protoreflect.ServiceDescriptor, methods ...protoreflect.Name) {
	t.Helper()
	if service == nil {
		t.Fatal("required service is missing")
	}
	for _, method := range methods {
		if service.Methods().ByName(method) == nil {
			t.Fatalf("%s service is missing %s", service.FullName(), method)
		}
	}
}

func assertMessageFields(t *testing.T, messageName string, fields ...protoreflect.Name) {
	t.Helper()
	message := pluginv1.File_kandev_plugin_v1_plugin_proto.Messages().ByName(protoreflect.Name(messageName))
	if message == nil {
		t.Fatalf("required message %s is missing", messageName)
	}
	for _, field := range fields {
		if message.Fields().ByName(field) == nil {
			t.Fatalf("%s is missing field %s", message.FullName(), field)
		}
	}
}

func assertMessageOmitsFields(t *testing.T, messageName string, fields ...protoreflect.Name) {
	t.Helper()
	message := pluginv1.File_kandev_plugin_v1_plugin_proto.Messages().ByName(protoreflect.Name(messageName))
	if message == nil {
		t.Fatalf("required message %s is missing", messageName)
	}
	for _, field := range fields {
		if message.Fields().ByName(field) != nil {
			t.Fatalf("%s must not expose field %s", message.FullName(), field)
		}
	}
}

func assertDeprecatedRepeatedMessageField(t *testing.T, messageName, fieldName string, number protoreflect.FieldNumber) {
	t.Helper()
	message := pluginv1.File_kandev_plugin_v1_plugin_proto.Messages().ByName(protoreflect.Name(messageName))
	if message == nil {
		t.Fatalf("required message %s is missing", messageName)
	}
	field := message.Fields().ByName(protoreflect.Name(fieldName))
	if field == nil {
		t.Fatalf("%s is missing field %s", message.FullName(), fieldName)
	}
	if field.Number() != number {
		t.Fatalf("%s.%s field number = %d, want %d", message.FullName(), fieldName, field.Number(), number)
	}
	if field.Cardinality() != protoreflect.Repeated {
		t.Fatalf("%s.%s cardinality = %s, want repeated", message.FullName(), fieldName, field.Cardinality())
	}
	options, ok := field.Options().(*descriptorpb.FieldOptions)
	if !ok || !options.GetDeprecated() {
		t.Fatalf("%s.%s must remain deprecated", message.FullName(), fieldName)
	}
}
