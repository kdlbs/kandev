package manifest

import (
	"strings"
	"testing"
)

func TestMessagesReadCapabilityRequiresSupportedMinimumVersion(t *testing.T) {
	tests := []struct {
		name       string
		minimum    string
		wantReject bool
	}{
		{name: "missing", wantReject: true},
		{name: "malformed", minimum: "next", wantReject: true},
		{name: "lower", minimum: "0.91.0", wantReject: true},
		{name: "equal", minimum: MinimumMessagesCapabilityVersion},
		{name: "higher", minimum: "0.92.0"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			manifest := validManifest(t)
			manifest.Capabilities.APIRead = append(manifest.Capabilities.APIRead, "messages")
			manifest.MinKandevVersion = test.minimum

			err := manifest.Validate()
			if test.wantReject {
				if err == nil || !strings.Contains(err.Error(), "api_read resource \"messages\" requires min_kandev_version >= 0.91.1") {
					t.Fatalf("Validate() error = %v, want messages capability minimum-version rejection", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("Validate() unexpected error: %v", err)
			}
		})
	}
}

func TestMessagesMinimumVersionPolicyDoesNotAffectOtherResources(t *testing.T) {
	manifest := validManifest(t)
	manifest.Capabilities.APIRead = []string{"tasks"}
	manifest.MinKandevVersion = ""

	if err := manifest.Validate(); err != nil {
		t.Fatalf("Validate() unexpected error: %v", err)
	}
	if RequiresMessagesCapabilityMinimum(manifest) {
		t.Fatal("tasks-only manifest unexpectedly requires the messages capability minimum")
	}
}
