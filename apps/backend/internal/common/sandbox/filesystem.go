package sandbox

import (
	"path/filepath"
	"slices"
	"strings"
)

func canonicalFilesystem(grants []FilesystemGrant, protected []string) ([]FilesystemGrant, error) {
	paths := make(map[FilesystemGrant]bool)
	for _, grant := range grants {
		canonical, err := canonicalFilesystemGrant(grant, protected)
		if err != nil {
			return nil, err
		}
		writable := canonical.Writable
		canonical.Writable = false
		paths[canonical] = paths[canonical] || writable
	}
	out := make([]FilesystemGrant, 0, len(paths))
	for grant, writable := range paths {
		grant.Writable = writable
		out = append(out, grant)
	}
	slices.SortFunc(out, func(a, b FilesystemGrant) int {
		if order := strings.Compare(a.Reference, b.Reference); order != 0 {
			return order
		}
		return strings.Compare(a.Path, b.Path)
	})
	return out, nil
}

func canonicalFilesystemGrant(grant FilesystemGrant, protected []string) (FilesystemGrant, error) {
	if grant.Reference != "" {
		if (grant.Reference != ReferenceWorkDir && grant.Reference != ReferenceAgentState) || grant.Path != "" {
			return FilesystemGrant{}, ErrInvalidPolicy
		}
		return grant, nil
	}
	path, err := canonicalPath(grant.Path)
	if err != nil {
		return FilesystemGrant{}, err
	}
	for _, root := range protected {
		if !filepath.IsAbs(root) || within(path, root) || within(root, path) {
			return FilesystemGrant{}, ErrInvalidPolicy
		}
	}
	grant.Path = path
	return grant, nil
}

func filesystemCovered(grant, limit FilesystemGrant) bool {
	if grant.Reference != "" || limit.Reference != "" {
		return grant.Reference != "" && grant.Reference == limit.Reference && grant.Path == "" && limit.Path == ""
	}
	return within(grant.Path, limit.Path)
}
