package canvas

import (
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/plugins/manifest"
	"github.com/kandev/kandev/internal/plugins/webapp"
)

// @covers AC-PLUGINS-PLUGINS-001.14
func TestDistributionInstallCompatibility(t *testing.T) {
	const nightly = "v1.2.4-nightly.sha123456789abc"
	for _, tt := range []struct {
		name    string
		running string
		minimum string
		reject  bool
	}{
		{name: "nightly future", running: nightly, minimum: "2.0.0", reject: true},
		{name: "nightly same core", running: nightly, minimum: "1.2.4", reject: true},
		{name: "nightly older", running: nightly, minimum: "1.2.3"},
		{name: "stable future", running: "v1.2.4", minimum: "2.0.0", reject: true},
		{name: "stable equal", running: "v1.2.4", minimum: "1.2.4"},
		{name: "development", running: "dev", minimum: "2.0.0"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			svc := &DistributionService{}
			svc.SetKandevVersion(tt.running)
			pkg := &webapp.Package{Manifest: &manifest.Manifest{MinKandevVersion: tt.minimum}}
			err := svc.validateInstallCompatibility(pkg)
			if tt.reject {
				if !errors.Is(err, ErrInstallIncompatible) {
					t.Fatalf("validateInstallCompatibility() = %v, want ErrInstallIncompatible", err)
				}
			} else if err != nil {
				t.Fatalf("validateInstallCompatibility() = %v, want nil", err)
			}
		})
	}
	for _, pkg := range []*webapp.Package{nil, {}} {
		svc := &DistributionService{}
		if err := svc.validateInstallCompatibility(pkg); !errors.Is(err, ErrInstallInvalid) {
			t.Errorf("validateInstallCompatibility(%v) = %v, want ErrInstallInvalid", pkg, err)
		}
	}
}
