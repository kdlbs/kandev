package lifecycle

import "testing"

func TestSSHReclaimEnabledOnlyForExplicitTrue(t *testing.T) {
	tests := []struct {
		name  string
		value interface{}
		want  bool
	}{
		{name: "explicit true", value: "true", want: true},
		{name: "true with surrounding space", value: "  true  ", want: true},
		{name: "explicit false", value: "false"},
		{name: "empty string", value: ""},
		{name: "uppercase true", value: "TRUE"},
		{name: "title case true", value: "True"},
		{name: "numeric one", value: "1"},
		{name: "yes", value: "yes"},
		{name: "unrelated text", value: "enabled"},
		{name: "non-string bool", value: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			md := map[string]interface{}{MetadataKeySSHReclaimTaskDir: tc.value}
			if got := SSHReclaimEnabled(md); got != tc.want {
				t.Fatalf("SSHReclaimEnabled(%#v) = %v, want %v", tc.value, got, tc.want)
			}
		})
	}
}

func TestSSHReclaimEnabledDefaultsOffWhenAbsent(t *testing.T) {
	if SSHReclaimEnabled(nil) {
		t.Fatal("SSHReclaimEnabled(nil) = true, want false")
	}
	if SSHReclaimEnabled(map[string]interface{}{}) {
		t.Fatal("SSHReclaimEnabled(empty) = true, want false")
	}
	// A profile that has never heard of this key must behave exactly as it
	// did before the capability existed.
	legacy := map[string]interface{}{
		MetadataKeySSHHost:        "example.internal",
		MetadataKeySSHWorkdirRoot: "/home/dev/.kandev",
	}
	if SSHReclaimEnabled(legacy) {
		t.Fatal("SSHReclaimEnabled(legacy profile metadata) = true, want false")
	}
}

func TestSSHReclaimTaskDirKeyIsPersisted(t *testing.T) {
	// The cleanup job reads this key from ExecutorRunning.Metadata after the
	// session is gone and possibly after a backend restart, so it has to
	// survive persistence alongside the connection tuple.
	if !persistentMetadataKeys[MetadataKeySSHReclaimTaskDir] {
		t.Fatalf("%s is not in persistentMetadataKeys", MetadataKeySSHReclaimTaskDir)
	}
}
