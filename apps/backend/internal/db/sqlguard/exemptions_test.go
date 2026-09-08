package sqlguard

import (
	"path/filepath"
	"testing"
)

func TestLoadExemptions(t *testing.T) {
	path := filepath.Join("testdata", "exemptions.json")
	exemptions, err := LoadExemptions(path)
	if err != nil {
		t.Fatalf("LoadExemptions() error = %v", err)
	}
	if len(exemptions) != 1 || exemptions[0].Rule != RuleSQLiteCatalog {
		t.Fatalf("LoadExemptions() = %#v", exemptions)
	}
}
