package profilesnapshot

import (
	"errors"
	"strings"
	"testing"

	"github.com/kdlbs/kandev/pluginsdk"
)

func testSchema() map[string]any {
	return map[string]any{"type": "object", "properties": map[string]any{
		"region": map[string]any{"type": "string"},
		"cpus":   map[string]any{"type": "integer"},
		"ratio":  map[string]any{"type": "number"},
		"spot":   map[string]any{"type": "boolean"},
		"token":  map[string]any{"type": "string", "secret": true},
	}}
}

func testInput() Input {
	return Input{
		ProfileID:    "profile-1",
		Config:       map[string]string{"region": "eu", "cpus": "4", "ratio": "0.5", "spot": "true"},
		SecretValues: map[string]string{"token": "tok-value-1234"},
		SecretRefs:   map[string]string{"token": "secret-id-1"},
		Schema:       testSchema(),
	}
}

func TestBuildWithoutCapabilitiesCarriesOnlyConfigAndValues(t *testing.T) {
	snapshot, err := Build(testInput())
	if err != nil {
		t.Fatalf("Build(): %v", err)
	}
	if len(snapshot.TypedConfig) != 0 || len(snapshot.SecretRefs) != 0 {
		t.Fatalf("a provider not requiring the capabilities must receive neither map: %v %v", snapshot.TypedConfig, snapshot.SecretRefs)
	}
	if snapshot.Config["cpus"] != "4" || snapshot.SecretValues["token"] != "tok-value-1234" {
		t.Fatalf("config and secret values changed: %v %v", snapshot.Config, snapshot.SecretValues)
	}
}

func TestBuildTypedConfigFollowsTheSchemaType(t *testing.T) {
	input := testInput()
	input.Capabilities = []string{CapabilityTypedProfileValues}
	snapshot, err := Build(input)
	if err != nil {
		t.Fatalf("Build(): %v", err)
	}
	typed := snapshot.TypedConfig
	if typed["region"].GetStringValue() != "eu" || typed["cpus"].GetIntValue() != 4 ||
		typed["ratio"].GetDoubleValue() != 0.5 || !typed["spot"].GetBoolValue() {
		t.Fatalf("typed config = %v", typed)
	}
	if _, secret := typed["token"]; secret {
		t.Fatal("typed_config must never contain a secret property")
	}
	if len(snapshot.SecretRefs) != 0 {
		t.Fatal("secret_refs needs its own capability")
	}
	if snapshot.Config["cpus"] != "4" {
		t.Fatal("config must stay canonical alongside typed_config")
	}
}

func TestBuildRejectsAValueThatDoesNotParse(t *testing.T) {
	for name, config := range map[string]map[string]string{
		"integer text":     {"cpus": "four"},
		"integer overflow": {"cpus": "9223372036854775808"},
		"bool":             {"spot": "yes"},
		"number infinity":  {"ratio": "Inf"},
		"number overflow":  {"ratio": "1e999"},
	} {
		t.Run(name, func(t *testing.T) {
			input := testInput()
			input.Config = config
			input.Capabilities = []string{CapabilityTypedProfileValues}
			_, err := Build(input)
			var invalid *InvalidTypeError
			if !errors.As(err, &invalid) {
				t.Fatalf("err = %v, want *InvalidTypeError", err)
			}
			for _, value := range config {
				if strings.Contains(err.Error(), value) {
					t.Fatalf("error echoes the value: %v", err)
				}
			}
		})
	}
}

func TestBuildSecretRefsNameOnlyDeclaredSecretsThatHoldAValue(t *testing.T) {
	input := testInput()
	input.Capabilities = []string{CapabilitySecretReferences}
	input.SecretRefs = map[string]string{"token": "secret-id-1", "region": "not-secret", "ghost": "id-9"}
	snapshot, err := Build(input)
	if err != nil {
		t.Fatalf("Build(): %v", err)
	}
	if len(snapshot.SecretRefs) != 1 || snapshot.SecretRefs["token"].GetId() != "secret-id-1" {
		t.Fatalf("secret_refs = %v, want exactly the declared secret property", snapshot.SecretRefs)
	}
	if len(snapshot.TypedConfig) != 0 {
		t.Fatal("typed_config needs its own capability")
	}
	input.SecretValues = nil
	snapshot, _ = Build(input)
	if len(snapshot.SecretRefs) != 0 {
		t.Fatalf("a property without a stored value must have no reference: %v", snapshot.SecretRefs)
	}
}

func TestSnapshotTypeIsTheSDKAlias(t *testing.T) {
	var snapshot *pluginsdk.ExecutorProfileSnapshot = mustBuild(t) //nolint:staticcheck // asserts the alias type
	_ = snapshot
}

func mustBuild(t *testing.T) *pluginsdk.ExecutorProfileSnapshot {
	t.Helper()
	snapshot, err := Build(testInput())
	if err != nil {
		t.Fatal(err)
	}
	return snapshot
}
