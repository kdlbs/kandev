package copilotplugins

import (
	"reflect"
	"runtime"
	"testing"
)

func TestSupportedRuntimeVersions(t *testing.T) {
	want := []string{"1.0.88", "1.0.104"}
	if runtime.GOOS == "linux" && runtime.GOARCH == "amd64" {
		want = append(want, "1.0.105")
	}
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

func TestRuntimeSupportedOnScopesPlatformEvidence(t *testing.T) {
	if !RuntimeSupportedOn("1.0.105", "linux", "amd64") {
		t.Fatal("1.0.105 must be qualified on linux/amd64")
	}
	for _, platform := range [][2]string{{"darwin", "arm64"}, {"windows", "amd64"}, {"linux", "arm64"}} {
		if RuntimeSupportedOn("1.0.105", platform[0], platform[1]) {
			t.Fatalf("1.0.105 must not be admitted on %s/%s", platform[0], platform[1])
		}
	}
	if !RuntimeSupportedOn("1.0.104", "darwin", "arm64") {
		t.Fatal("unscoped qualified versions must remain available on every platform")
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
