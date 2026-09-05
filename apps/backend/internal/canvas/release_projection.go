package canvas

import (
	"sort"

	plugininstances "github.com/kandev/kandev/internal/plugins/instances"
)

func releaseMetadata(release plugininstances.Release, scope string, grants []plugininstances.Grant) *ReleaseMetadata {
	permissions := ReleasePermissionSummary(release)
	return &ReleaseMetadata{
		ID:                 release.ID,
		PackageDigest:      release.PackageDigest,
		ValidationStatus:   release.ValidationStatus,
		ValidationError:    release.ValidationError,
		Permissions:        &permissions,
		MissingPermissions: MissingPermissionKeys(permissions, scope, grants),
		PermissionDigest:   PermissionDigest(release),
		SourceActorKind:    release.SourceActorKind,
		SourceUserID:       release.SourceUserID,
		SourceTaskID:       release.SourceTaskID,
		SourceSessionID:    release.SourceSessionID,
		ProtocolVersion:    release.ProtocolVersion,
		CreatedAt:          release.CreatedAt,
	}
}

func effectiveGrantProjection(instance plugininstances.Instance, summary PermissionSummary, grants []plugininstances.Grant) []GrantProjection {
	if instance.ActiveReleaseID == "" {
		return nil
	}
	declared := permissionKeys(summary)
	result := make([]GrantProjection, 0, len(grants))
	for _, grant := range grants {
		if !grantScopeCovers(grant.ScopeCeiling, instance.ScopeKind) {
			continue
		}
		permission := grant.PermissionKind + ":" + grant.Resource
		if grant.PermissionKind == permissionKindNetwork {
			permission = permissionKindNetwork + ":" + grant.NetworkOrigin
		}
		if grant.PermissionKind == permissionKindState {
			permission = permissionKindState
		}
		if !containsString(declared, permission) {
			continue
		}
		result = append(result, GrantProjection{
			PermissionKind: grant.PermissionKind,
			Resource:       grant.Resource,
			NetworkOrigin:  grant.NetworkOrigin,
			ScopeCeiling:   grant.ScopeCeiling,
		})
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].PermissionKind != result[j].PermissionKind {
			return result[i].PermissionKind < result[j].PermissionKind
		}
		if result[i].Resource != result[j].Resource {
			return result[i].Resource < result[j].Resource
		}
		return result[i].NetworkOrigin < result[j].NetworkOrigin
	})
	return result
}
