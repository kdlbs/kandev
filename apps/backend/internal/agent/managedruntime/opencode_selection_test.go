package managedruntime

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

func testOpenCodeDefaults() OpenCodeRuntimeDefaults {
	return OpenCodeRuntimeDefaults{
		V1Package: "opencode-ai",
		V1Version: "1.18.32",
		V2Package: "@opencode/cli",
		V2Version: "2.0.18",
	}
}

func TestBootstrapOpenCodeFreshInstallSelectsManagedV2(t *testing.T) {
	store := NewStore(&memorySettings{})
	selection, err := store.BootstrapOpenCode(context.Background(), testOpenCodeDefaults(), OpenCodeBootstrapEvidence{})
	if err != nil {
		t.Fatalf("BootstrapOpenCode: %v", err)
	}
	if selection.Family != OpenCodeFamilyV2 || selection.Source != OpenCodeSourceManaged {
		t.Fatalf("fresh selection = %+v, want managed v2", selection)
	}
	if selection.Package != "@opencode/cli" || selection.AppliedDefaultVersion != "2.0.18" || selection.Revision != 1 {
		t.Fatalf("fresh selection fields = %+v", selection)
	}
}

func TestBootstrapOpenCodeNativeInstallTakesPrecedence(t *testing.T) {
	store := NewStore(&memorySettings{})
	selection, err := store.BootstrapOpenCode(context.Background(), testOpenCodeDefaults(), OpenCodeBootstrapEvidence{
		NativeFamily: OpenCodeFamilyV2,
		PriorUse:     true,
	})
	if err != nil {
		t.Fatalf("BootstrapOpenCode: %v", err)
	}
	if selection.Family != OpenCodeFamilyV2 || selection.Source != OpenCodeSourceNative || selection.SelectedVersion != "" {
		t.Fatalf("native selection = %+v, want native v2 without a managed version override", selection)
	}
}

func TestBootstrapOpenCodeImportsLegacySelectionBeforeMarkerAndCleansItAfterSave(t *testing.T) {
	settings := &memorySettings{values: map[string][]byte{
		selectionKey("opencode-acp"):         []byte(`{"package":"opencode-ai","version":"1.18.5"}`),
		defaultGenerationKey("opencode-acp"): []byte(`{"package":"opencode-ai","version":"1.18.32"}`),
	}}
	store := NewStore(settings)

	selection, err := store.BootstrapOpenCode(context.Background(), testOpenCodeDefaults(), OpenCodeBootstrapEvidence{
		PriorUse: true,
	})
	if err != nil {
		t.Fatalf("BootstrapOpenCode: %v", err)
	}
	if selection.Family != OpenCodeFamilyV1 || selection.Source != OpenCodeSourceManaged || selection.SelectedVersion != "1.18.5" {
		t.Fatalf("imported selection = %+v, want managed v1@1.18.5", selection)
	}
	if selection.AppliedDefaultVersion != "1.18.32" || selection.Revision != 1 || selection.SelectedVersion != "1.18.5" {
		t.Fatalf("imported current default = %+v, want the valid selection and marker at revision 1", selection)
	}
	if _, found := settings.values[selectionKey("opencode-acp")]; found {
		t.Fatal("legacy active selection remains after authoritative save")
	}
	if _, found := settings.values[defaultGenerationKey("opencode-acp")]; found {
		t.Fatal("legacy default marker remains after authoritative save")
	}
	if !reflect.DeepEqual(settings.operations[:3], []string{
		"get:" + openCodeSelectionKey,
		"get:" + selectionKey("opencode-acp"),
		"get:" + defaultGenerationKey("opencode-acp"),
	}) {
		t.Fatalf("bootstrap reads = %#v", settings.operations)
	}
	if saveIndex, deleteIndex := operationIndex(settings.operations, "save:"+openCodeSelectionKey), operationIndex(settings.operations, "delete:"+selectionKey("opencode-acp")); saveIndex < 0 || deleteIndex < 0 || saveIndex >= deleteIndex {
		t.Fatalf("authoritative selection was not saved before legacy cleanup: %#v", settings.operations)
	}
}

func TestBootstrapOpenCodeMarkerAndPriorUseKeepV1(t *testing.T) {
	tests := []struct {
		name     string
		values   map[string][]byte
		evidence OpenCodeBootstrapEvidence
	}{
		{
			name: "old generation marker",
			values: map[string][]byte{
				defaultGenerationKey("opencode-acp"): []byte(`{"package":"opencode-ai","version":"1.18.18"}`),
			},
		},
		{name: "existing use evidence", evidence: OpenCodeBootstrapEvidence{PriorUse: true}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := NewStore(&memorySettings{values: tt.values})
			selection, err := store.BootstrapOpenCode(context.Background(), testOpenCodeDefaults(), tt.evidence)
			if err != nil {
				t.Fatalf("BootstrapOpenCode: %v", err)
			}
			if selection.Family != OpenCodeFamilyV1 || selection.Source != OpenCodeSourceManaged {
				t.Fatalf("selection = %+v, want managed v1", selection)
			}
			if tt.name == "old generation marker" && (selection.AppliedDefaultVersion != "1.18.32" || selection.Revision != 2) {
				t.Fatalf("reconciled selection = %+v, want current v1 default at revision 2", selection)
			}
		})
	}
}

func TestBootstrapOpenCodeRejectsInvalidAuthoritativeStateWithoutImporting(t *testing.T) {
	settings := &memorySettings{values: map[string][]byte{
		openCodeSelectionKey:                 []byte(`{"schema_version":1,"family":"v3"}`),
		selectionKey("opencode-acp"):         []byte(`{"package":"opencode-ai","version":"1.18.5"}`),
		defaultGenerationKey("opencode-acp"): []byte(`{"package":"opencode-ai","version":"1.18.32"}`),
	}}
	store := NewStore(settings)
	if _, err := store.BootstrapOpenCode(context.Background(), testOpenCodeDefaults(), OpenCodeBootstrapEvidence{}); !errors.Is(err, ErrInvalidOpenCodeSelection) {
		t.Fatalf("BootstrapOpenCode error = %v, want %v", err, ErrInvalidOpenCodeSelection)
	}
	if _, found := settings.values[selectionKey("opencode-acp")]; !found {
		t.Fatal("legacy selection was cleaned before invalid authoritative state was repaired")
	}
}

func TestBootstrapOpenCodeReconcilesOnlyTheAdoptedFamilyDefault(t *testing.T) {
	settings := &memorySettings{}
	store := NewStore(settings)
	first := OpenCodeSelection{
		SchemaVersion:         1,
		Family:                OpenCodeFamilyV2,
		Source:                OpenCodeSourceManaged,
		Package:               "@opencode/cli",
		SelectedVersion:       "2.0.19",
		AppliedDefaultVersion: "2.0.18",
		Revision:              1,
	}
	if err := store.SaveOpenCodeSelection(context.Background(), 0, first); err != nil {
		t.Fatalf("SaveOpenCodeSelection: %v", err)
	}
	defaults := testOpenCodeDefaults()
	defaults.V1Version = "1.18.33"
	defaults.V2Version = "2.1.0"
	selection, err := store.BootstrapOpenCode(context.Background(), defaults, OpenCodeBootstrapEvidence{NativeFamily: OpenCodeFamilyV1})
	if err != nil {
		t.Fatalf("BootstrapOpenCode: %v", err)
	}
	if selection.Family != OpenCodeFamilyV2 || selection.Source != OpenCodeSourceManaged || selection.Package != "@opencode/cli" {
		t.Fatalf("selection changed family or source: %+v", selection)
	}
	if selection.SelectedVersion != "" || selection.AppliedDefaultVersion != "2.1.0" || selection.Revision != 2 {
		t.Fatalf("reconciled selection = %+v", selection)
	}
}

func TestSaveOpenCodeSelectionRejectsStaleRevision(t *testing.T) {
	store := NewStore(&memorySettings{})
	selection := OpenCodeSelection{
		SchemaVersion:         1,
		Family:                OpenCodeFamilyV1,
		Source:                OpenCodeSourceManaged,
		Package:               "opencode-ai",
		AppliedDefaultVersion: "1.18.32",
		Revision:              1,
	}
	if err := store.SaveOpenCodeSelection(context.Background(), 0, selection); err != nil {
		t.Fatalf("initial SaveOpenCodeSelection: %v", err)
	}
	selection.Revision = 2
	if err := store.SaveOpenCodeSelection(context.Background(), 0, selection); !errors.Is(err, ErrOpenCodeSelectionRevisionConflict) {
		t.Fatalf("stale save error = %v, want %v", err, ErrOpenCodeSelectionRevisionConflict)
	}
}

func operationIndex(operations []string, value string) int {
	for i, operation := range operations {
		if operation == value {
			return i
		}
	}
	return -1
}
