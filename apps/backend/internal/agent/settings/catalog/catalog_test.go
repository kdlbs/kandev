package catalog

import (
	"testing"

	"github.com/kandev/kandev/internal/agent/settings/dto"
)

func TestProfileDescriptorCoversSharedContract(t *testing.T) {
	descriptor := ProfileDescriptor()
	seen := make(map[string]struct{}, len(descriptor.Fields))
	for _, field := range descriptor.Fields {
		seen[field.FieldPath] = struct{}{}
	}
	for _, field := range dto.ProfileContractFields() {
		if _, ok := seen[field.Path]; !ok {
			t.Errorf("profile contract field %q is absent from catalog", field.Path)
		}
	}
	if descriptor.ResourceType != "agent_profile" || descriptor.Target.RequiresResourceID == false {
		t.Fatalf("invalid profile target descriptor: %#v", descriptor)
	}
}

func TestProfileDescriptorDoesNotMakeComputedFieldsWritable(t *testing.T) {
	descriptor := ProfileDescriptor()
	for _, field := range descriptor.Fields {
		if field.Writable && (field.Validator == "" || field.Authority == "") {
			t.Fatalf("writable field %q has no validator or authority", field.Key)
		}
	}
	if len(MCPDescriptor().Fields) != 3 {
		t.Fatalf("MCP field count = %d, want enabled, servers, and preserved metadata", len(MCPDescriptor().Fields))
	}
}
