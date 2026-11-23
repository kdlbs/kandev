package manifest

import (
	"errors"
	"regexp"
	"strings"
)

var fileViewerExtensionPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,31}$`)

func (m *Manifest) validateFileViewers() []error {
	if len(m.UI.FileViewers) == 0 {
		return nil
	}
	if len(m.UI.FileViewers) > 16 || m.UI.Bundle == "" {
		return []error{errors.New("file viewers require a UI bundle and at most 16 declarations")}
	}
	ids := make(map[string]bool)
	for _, viewer := range m.UI.FileViewers {
		if !keybindingIDPattern.MatchString(viewer.ID) || len(viewer.ID) > 64 || ids[viewer.ID] || strings.TrimSpace(viewer.Title) == "" || len(viewer.Title) > 128 {
			return []error{errors.New("file viewers require unique IDs and bounded titles")}
		}
		ids[viewer.ID] = true
		if len(viewer.Extensions) == 0 || len(viewer.Extensions) > 32 {
			return []error{errors.New("file viewers require between 1 and 32 extensions")}
		}
		seen := make(map[string]bool)
		for _, extension := range viewer.Extensions {
			if !fileViewerExtensionPattern.MatchString(extension) || seen[extension] {
				return []error{errors.New("file viewer extensions must be unique lowercase suffixes without dots or wildcards")}
			}
			seen[extension] = true
		}
	}
	return nil
}
