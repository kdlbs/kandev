package copilotplugins

import (
	"reflect"
	"testing"
)

func TestSupportedRuntimeVersions(t *testing.T) {
	want := []string{"1.0.88", "1.0.104", "1.0.105"}
	if got := SupportedRuntimeVersions(); !reflect.DeepEqual(got, want) {
		t.Fatalf("SupportedRuntimeVersions() = %v, want %v", got, want)
	}
	for _, version := range want {
		if !RuntimeSupported(version) {
			t.Fatalf("RuntimeSupported(%q) = false, want true", version)
		}
	}
	if RuntimeSupported("") || RuntimeSupported("9.9.9") {
		t.Fatal("unsupported versions must not be admitted")
	}
}

func TestSupportedRuntimeVersionsReturnsIndependentSlice(t *testing.T) {
	versions := SupportedRuntimeVersions()
	if len(versions) == 0 {
		t.Fatal("SupportedRuntimeVersions() is empty")
	}
	versions[0] = "9.9.9"
	if RuntimeSupported("9.9.9") {
		t.Fatal("mutating the returned slice changed runtime admission")
	}
	if !RuntimeSupported("1.0.88") {
		t.Fatal("mutating the returned slice removed a qualified runtime")
	}
}
