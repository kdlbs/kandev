package configloader

import (
	"embed"
	"fmt"
	"io/fs"
)

//go:embed instructions/*
var bundledInstructions embed.FS

// InstructionTemplate represents a single embedded instruction file.
type InstructionTemplate struct {
	Filename string
	Content  string
}

// LoadRoleTemplates returns the embedded instruction templates for a given role.
// Returns nil if no templates exist for the role.
func LoadRoleTemplates(role string) ([]InstructionTemplate, error) {
	dirPath := fmt.Sprintf("instructions/%s", role)

	entries, err := bundledInstructions.ReadDir(dirPath)
	if err != nil {
		// No templates for this role -- not an error.
		return nil, nil //nolint:nilerr
	}

	templates := make([]InstructionTemplate, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		data, readErr := fs.ReadFile(bundledInstructions, dirPath+"/"+e.Name())
		if readErr != nil {
			return nil, fmt.Errorf("read template %s/%s: %w", role, e.Name(), readErr)
		}
		templates = append(templates, InstructionTemplate{
			Filename: e.Name(),
			Content:  string(data),
		})
	}
	return templates, nil
}

// AvailableInstructionRoles returns the roles that have embedded templates.
func AvailableInstructionRoles() ([]string, error) {
	entries, err := bundledInstructions.ReadDir("instructions")
	if err != nil {
		return nil, err
	}
	roles := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() {
			roles = append(roles, e.Name())
		}
	}
	return roles, nil
}
