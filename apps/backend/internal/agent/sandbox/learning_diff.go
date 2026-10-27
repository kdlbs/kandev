package sandbox

import (
	"slices"

	contract "github.com/kandev/kandev/internal/common/sandbox"
)

func learningDifference(base, proposed contract.Grants) contract.Grants {
	difference := contract.Grants{Filesystem: []contract.FilesystemGrant{}, Network: []contract.NetworkGrant{}, KandevMCP: proposed.KandevMCP && !base.KandevMCP}
	for _, grant := range proposed.Filesystem {
		if contract.WithinCeiling(contract.Grants{Filesystem: []contract.FilesystemGrant{grant}}, base) != nil {
			difference.Filesystem = append(difference.Filesystem, grant)
		}
	}
	for _, grant := range proposed.Network {
		if !slices.Contains(base.Network, grant) {
			difference.Network = append(difference.Network, grant)
		}
	}
	for _, key := range proposed.EnvironmentKeys {
		if !slices.Contains(base.EnvironmentKeys, key) {
			difference.EnvironmentKeys = append(difference.EnvironmentKeys, key)
		}
	}
	return difference
}
