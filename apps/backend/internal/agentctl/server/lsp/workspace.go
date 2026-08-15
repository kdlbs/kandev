package lsp

import (
	"net/url"
	"path/filepath"
	"strings"
)

// WorkspaceFoldersAtRoots converts a trusted task workspace projection into
// ordered LSP workspace folders. Empty roots use the task workspace itself.
func WorkspaceFoldersAtRoots(workspacePath string, roots []string) []WorkspaceFolder {
	workspacePath = filepath.Clean(workspacePath)
	folders := make([]WorkspaceFolder, 0, max(1, len(roots)))
	seen := make(map[string]struct{}, len(roots))
	for _, root := range roots {
		if root == "" || !filepath.IsAbs(root) {
			continue
		}
		root = filepath.Clean(root)
		if _, duplicate := seen[root]; duplicate {
			continue
		}
		seen[root] = struct{}{}
		folders = append(folders, WorkspaceFolder{URI: WorkspaceFileURI(root), Name: filepath.Base(root)})
	}
	if len(folders) == 0 && workspacePath != "." && filepath.IsAbs(workspacePath) {
		folders = append(folders, WorkspaceFolder{
			URI: WorkspaceFileURI(workspacePath), Name: filepath.Base(workspacePath),
		})
	}
	return folders
}

// WorkspaceFileURI applies task-host path semantics, including Windows drive
// and UNC forms, while strictly escaping reserved URI characters.
func WorkspaceFileURI(path string) string {
	normalized := path
	isDrivePath := len(path) >= 3 && path[1] == ':' && (path[2] == '\\' || path[2] == '/')
	isUNCPath := strings.HasPrefix(path, `\\`)
	if isDrivePath || isUNCPath {
		normalized = strings.ReplaceAll(path, `\`, "/")
	}
	if strings.HasPrefix(normalized, "//") {
		parts := strings.Split(strings.TrimPrefix(normalized, "//"), "/")
		if len(parts) >= 2 && parts[0] != "" {
			return strictFileURL(parts[0], "/"+strings.Join(parts[1:], "/"), false)
		}
	}
	isDriveURI := len(normalized) >= 3 && normalized[1] == ':' && normalized[2] == '/'
	if isDriveURI {
		normalized = "/" + normalized
	}
	return strictFileURL("", normalized, isDriveURI)
}

func strictFileURL(host, filePath string, preserveDriveColon bool) string {
	segments := strings.Split(filePath, "/")
	encoded := make([]string, len(segments))
	for index, segment := range segments {
		encoded[index] = strictURISegment(segment)
	}
	if preserveDriveColon && len(segments) > 1 {
		encoded[1] = segments[1]
	}
	return (&url.URL{
		Scheme: "file", Host: host, Path: filePath, RawPath: strings.Join(encoded, "/"),
	}).String()
}

func strictURISegment(segment string) string {
	const hex = "0123456789ABCDEF"
	var encoded strings.Builder
	for _, value := range []byte(segment) {
		if (value >= 'a' && value <= 'z') || (value >= 'A' && value <= 'Z') ||
			(value >= '0' && value <= '9') || strings.ContainsRune("-._~", rune(value)) {
			encoded.WriteByte(value)
			continue
		}
		encoded.WriteByte('%')
		encoded.WriteByte(hex[value>>4])
		encoded.WriteByte(hex[value&0x0f])
	}
	return encoded.String()
}
