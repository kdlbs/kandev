package lifecycle

import (
	"fmt"
	"strings"

	"github.com/kandev/kandev/internal/plugins/workspacedescriptor"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/pkg/pluginsdk"
)

const pluginExecutorCapabilityWorkspaceDescriptor = "workspace_descriptor"

// negotiatedDeclares reports whether a recorded `name@version` set contains the
// capability, whatever its version.
func negotiatedDeclares(negotiated []string, name string) bool {
	for _, pair := range negotiated {
		if recorded, _, _ := strings.Cut(pair, "@"); recorded == name {
			return true
		}
	}
	return false
}

// resolvePluginExecutorWorkspace validates the workspace descriptor of a
// provider response against the environment's recorded capability set and, when
// one is already recorded for the generation, against that value. A provider
// that did not declare the capability must return neither field. The path is
// parsed by the resource's platform; the backend filesystem is never consulted.
func resolvePluginExecutorWorkspace(negotiated []string, recorded *workspacedescriptor.Descriptor, resource *pluginsdk.ExecutorResourceDescriptor) (*workspacedescriptor.Descriptor, error) {
	path, identity := resource.GetWorkspacePath(), resource.GetWorkspaceIdentity()
	if !negotiatedDeclares(negotiated, pluginExecutorCapabilityWorkspaceDescriptor) {
		if path != "" || identity != "" {
			return nil, fmt.Errorf("%w: provider returned a workspace descriptor without declaring the capability", models.ErrExecutorConflict)
		}
		return nil, nil
	}
	if err := workspacedescriptor.Validate(resource.GetPlatform(), path, identity); err != nil {
		return nil, fmt.Errorf("provider workspace descriptor is invalid: %w", err)
	}
	next := &workspacedescriptor.Descriptor{Path: path, Identity: identity}
	if recorded != nil && *recorded != *next {
		return nil, fmt.Errorf("%w: provider changed the recorded workspace descriptor", models.ErrExecutorConflict)
	}
	return next, nil
}

// workspacePath is the recorded workspace path, or the fixed path of a
// provider that declares no descriptor.
func (i pluginExecutorInventory) workspacePath() string {
	if i.WorkspaceDescriptor != nil {
		return i.WorkspaceDescriptor.Path
	}
	return pluginExecutorWorkspacePath
}

// requireRecordedPluginExecutorWorkspace fails closed for an environment that
// declared the capability but has no recorded descriptor, or whose stored
// resource disagrees with it.
func requireRecordedPluginExecutorWorkspace(inventory pluginExecutorInventory) error {
	declared, err := resolvePluginExecutorWorkspace(inventory.NegotiatedCapabilities, inventory.WorkspaceDescriptor, inventory.Resource)
	if err != nil {
		return err
	}
	if declared != nil && inventory.WorkspaceDescriptor == nil {
		return fmt.Errorf("%w: environment has no recorded workspace descriptor", models.ErrExecutorConflict)
	}
	return nil
}
