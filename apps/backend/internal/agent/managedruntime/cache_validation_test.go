package managedruntime

import "testing"

func TestValidateExactPackageSpecAcceptsStablePackageVersions(t *testing.T) {
	for _, packageSpec := range []string{
		"managed-acp@1.2.3",
		"@scope/managed-acp@1.2.3+build.7",
	} {
		t.Run(packageSpec, func(t *testing.T) {
			if err := ValidateExactPackageSpec(packageSpec); err != nil {
				t.Fatalf("ValidateExactPackageSpec(%q): %v", packageSpec, err)
			}
		})
	}
}

func TestValidateExactPackageSpecRejectsUntrustedSelectors(t *testing.T) {
	for _, packageSpec := range []string{
		"managed-acp",
		"managed-acp@1.2.3-beta.1",
		"managed-acp@latest",
		"managed-acp@latest@1.2.3",
		"../managed-acp@1.2.3",
		"/tmp/managed-acp@1.2.3",
		"@scope/managed-acp@1.2.3/extra",
		"@scope@other/managed-acp@1.2.3",
		" managed-acp@1.2.3",
		"managed-acp@1.2.3 ",
	} {
		t.Run(packageSpec, func(t *testing.T) {
			if err := ValidateExactPackageSpec(packageSpec); err == nil {
				t.Fatalf("ValidateExactPackageSpec(%q) = nil, want rejection", packageSpec)
			}
		})
	}
}
