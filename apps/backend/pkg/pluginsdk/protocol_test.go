package pluginsdk

import (
	"testing"

	pluginv1 "github.com/kandev/kandev/proto/kandev/plugin/v1"
	"google.golang.org/protobuf/reflect/protoreflect"
)

func TestProtocolDeclaresAdditivePluginAndHostMethods(t *testing.T) {
	services := pluginv1.File_kandev_plugin_v1_plugin_proto.Services()
	assertServiceMethods(t, services.ByName("Plugin"), "HandleAction", "SearchEntityReferences", "AuthorizeEntityReference", "ResolveGitCredential", "GetGitCredentialBinding")
	assertServiceMethods(t, services.ByName("Host"), "ListExecutorProfiles", "PreviewPluginOwnedTaskTree", "DeletePluginOwnedTaskTree")
	assertMessageFields(t, "PluginActionRequest", "action_key", "context", "body")
	assertMessageFields(t, "VerifiedActionContext", "actor_id", "workspace_id", "task_id", "repository_id")
	assertMessageFields(t, "SearchEntityReferencesRequest", "source", "workspace_id", "query", "limit")
	assertMessageFields(t, "AuthorizeEntityReferenceRequest", "source", "workspace_id", "purpose", "reference")
	assertMessageFields(t, "ResolveGitCredentialRequest", "provider_id", "workspace_id", "task_id", "session_id", "repository_id", "host", "path")
	assertMessageFields(t, "GitCredentialBindingRequest", "provider_id", "workspace_id", "task_id", "session_id", "repository_id", "host", "path")
	assertMessageFields(t, "GitCredentialBindingResponse", "binding")
	assertMessageFields(t, "TaskRepository", "id", "repository_id", "base_branch", "position", "checkout_branch")
	assertMessageFields(t, "Repository", "id", "workspace_id", "name", "default_branch", "source_type", "provider_id", "provider_repository_id", "provider_host", "owner_or_project", "provider_name", "remote_url")
	assertMessageOmitsFields(t, "Repository", "local_path", "setup_script", "cleanup_script", "dev_script", "copy_files")
	assertMessageFields(t, "CreateTaskRequest", "repositories", "launch", "metadata")
	assertMessageFields(t, "RemoteRepositoryDescriptor", "provider_id", "provider_host", "owner_or_project", "provider_repository_id", "name", "clone_url")
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
