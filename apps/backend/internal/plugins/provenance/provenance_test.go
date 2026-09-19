package provenance

import (
	"testing"
	"time"
)

func TestEvidenceValidateRequiresCompleteRegistryIdentity(t *testing.T) {
	valid := Evidence{
		SchemaVersion: 1,
		RepositoryID:  "123",
		OwnerID:       "456",
		Login:         "acme",
		Repository:    "acme/example",
		Official:      false,
	}
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid evidence rejected: %v", err)
	}

	for name, mutate := range map[string]func(*Evidence){
		"schema":                   func(e *Evidence) { e.SchemaVersion = 2 },
		"repository id":            func(e *Evidence) { e.RepositoryID = "" },
		"owner id":                 func(e *Evidence) { e.OwnerID = "owner" },
		"login":                    func(e *Evidence) { e.Login = "" },
		"repository":               func(e *Evidence) { e.Repository = "other/example" },
		"repository id whitespace": func(e *Evidence) { e.RepositoryID = " 123" },
		"owner id whitespace":      func(e *Evidence) { e.OwnerID = "456 " },
		"login whitespace":         func(e *Evidence) { e.Login = " acme" },
		"repository whitespace":    func(e *Evidence) { e.Repository = "acme/example " },
	} {
		t.Run(name, func(t *testing.T) {
			candidate := valid
			mutate(&candidate)
			if err := candidate.Validate(); err == nil {
				t.Fatal("invalid evidence was accepted")
			}
		})
	}
}

func TestInstallationProvenanceRejectsMismatchedEvidence(t *testing.T) {
	verifiedAt := time.Date(2026, 9, 18, 12, 0, 0, 0, time.UTC)
	p := &InstallationProvenance{
		Origin:             OriginCatalog,
		SourceID:           "official",
		SourceURL:          "https://kdlbs.github.io/kandev/plugins/index.json",
		PackageID:          "example",
		Version:            "1.2.3",
		PackageSHA256:      "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		Publisher:          &Evidence{SchemaVersion: 1, RepositoryID: "123", OwnerID: "456", Login: "acme", Repository: "acme/example"},
		VerifiedAt:         &verifiedAt,
		VerificationMethod: VerificationArchiveDownload,
	}
	if err := p.Validate(); err != nil {
		t.Fatalf("valid provenance rejected: %v", err)
	}

	p.Publisher.Repository = "other/example"
	if err := p.Validate(); err == nil {
		t.Fatal("provenance accepted mismatched publisher evidence")
	}
}

func TestIdentityProjectsOnlyValidatedEvidence(t *testing.T) {
	verifiedAt := time.Date(2026, 9, 18, 12, 0, 0, 0, time.UTC)
	p := &InstallationProvenance{
		Origin:             OriginUpload,
		PackageID:          "example",
		Version:            "1.2.3",
		PackageSHA256:      "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		Publisher:          &Evidence{SchemaVersion: 1, RepositoryID: "123", OwnerID: "456", Login: "acme", Repository: "acme/example"},
		VerifiedAt:         &verifiedAt,
		VerificationMethod: VerificationInstalledFiles,
	}
	identity := p.Identity()
	if identity.Status != StatusVerified || identity.Login != "acme" || identity.Repository != "acme/example" {
		t.Fatalf("identity = %+v, want verified acme identity", identity)
	}

	p.PackageSHA256 = "bad"
	if got := p.Identity(); got.Status != StatusUnverified || got.Login != "" {
		t.Fatalf("invalid provenance projected as %+v, want empty unverified identity", got)
	}
}
