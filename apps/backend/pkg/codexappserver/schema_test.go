package codexappserver

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"testing"
)

func TestVersionedSchemaFixtureMatchesPinnedDigest(t *testing.T) {
	data, err := os.ReadFile(SchemaPathV0154)
	if err != nil {
		t.Fatalf("read schema fixture: %v", err)
	}
	var schema map[string]json.RawMessage
	if err := json.Unmarshal(data, &schema); err != nil {
		t.Fatalf("decode schema fixture: %v", err)
	}
	digest := sha256.Sum256(data)
	if got := hex.EncodeToString(digest[:]); got != SchemaSHA256V0154 {
		t.Fatalf("schema digest = %s, want %s", got, SchemaSHA256V0154)
	}
	if _, ok := schema["definitions"]; !ok {
		t.Fatal("schema fixture has no protocol definitions")
	}
}

func TestServerRequestInventoryMatchesPinnedFixture(t *testing.T) {
	data, err := os.ReadFile("schema/v0.154.0/server-requests.json")
	if err != nil {
		t.Fatalf("read server-request inventory: %v", err)
	}
	var fixture struct {
		CodexVersion   string   `json:"codexVersion"`
		UpstreamCommit string   `json:"upstreamCommit"`
		Source         string   `json:"source"`
		Methods        []string `json:"methods"`
	}
	if err := json.Unmarshal(data, &fixture); err != nil {
		t.Fatalf("decode server-request inventory: %v", err)
	}
	if fixture.CodexVersion != SupportedCodexVersion || fixture.UpstreamCommit == "" || fixture.Source == "" {
		t.Fatalf("server-request inventory provenance = %#v", fixture)
	}
	got := ServerRequestMethodsV0154()
	if len(got) != len(fixture.Methods) {
		t.Fatalf("server request method count = %d, want %d", len(got), len(fixture.Methods))
	}
	for index, method := range fixture.Methods {
		if got[index] != method {
			t.Errorf("server request method %d = %q, want %q", index, got[index], method)
		}
	}
}
