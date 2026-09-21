package config

import (
	"testing"

	"github.com/spf13/viper"
)

// The shipped default must be empty, which leaves the daemon's own default
// network in place.
//
// The key previously defaulted to "kandev-network" while nothing read it.
// Nothing in Kandev creates a network by that name, so making the setting
// effective while keeping that default would fail container creation on every
// installation that had never hand-created it.
//
// @covers AC-EXECUTORS-DOCKER-NETWORKS-001.3
func TestDockerDefaultNetworkDefaultsToEmpty(t *testing.T) {
	v := viper.New()
	setDefaults(v)

	if got := v.GetString("docker.defaultNetwork"); got != "" {
		t.Errorf("docker.defaultNetwork default = %q, want empty", got)
	}
}

// The catalog is the documented contract for the key, so its recorded default
// has to agree with the one the loader applies.
//
// @covers AC-EXECUTORS-DOCKER-NETWORKS-001.3
func TestDockerDefaultNetworkCatalogEntryMatches(t *testing.T) {
	entry, ok := CatalogEntryForKey("docker.defaultNetwork")
	if !ok {
		t.Fatal("docker.defaultNetwork is missing from the catalog")
	}
	if entry.Default != "" {
		t.Errorf("catalog default = %q, want empty", entry.Default)
	}
}
