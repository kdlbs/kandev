package promptsync

import (
	"fmt"
	"path"
	"sort"
	"strings"
	"unicode/utf8"

	promptservice "github.com/kandev/kandev/internal/prompts/service"
	"github.com/kandev/kandev/internal/reposync"
)

const promptFileExtension = ".md"

// fetchedFile is one prompt file read from the repository. tooLarge marks a
// file the provider refused to return because of its size.
type fetchedFile struct {
	path     string
	content  []byte
	tooLarge bool
}

// selectPromptFiles returns the paths of immediate Markdown files, sorted.
func selectPromptFiles(entries []reposync.Entry) []string {
	var paths []string
	for _, entry := range entries {
		if entry.IsFile && hasPromptExtension(entry.Name) {
			paths = append(paths, entry.Path)
		}
	}
	sort.Strings(paths)
	return paths
}

// hasPromptExtension reports whether a file name ends in .md, compared
// case-insensitively. A bare ".md" qualifies so validation can report it.
func hasPromptExtension(fileName string) bool {
	return len(fileName) >= len(promptFileExtension) &&
		strings.EqualFold(fileName[len(fileName)-len(promptFileExtension):], promptFileExtension)
}

// promptName derives a prompt name from a file name, or "" when the file is
// not a Markdown file. A bare ".md" yields "" as well.
func promptName(fileName string) string {
	if len(fileName) <= len(promptFileExtension) || !hasPromptExtension(fileName) {
		return ""
	}
	return fileName[:len(fileName)-len(promptFileExtension)]
}

// parsePromptFiles validates fetched files. Files are grouped by prompt name
// before validation, so two files claiming one name freeze it even when one of
// them is invalid. Valid single files become desired prompts; an invalid or
// duplicated name is frozen so its existing synced prompt stays unchanged.
func parsePromptFiles(files []fetchedFile) ([]promptservice.SyncedPromptFile, map[string]bool, []string) {
	frozen := map[string]bool{}
	var warnings []string
	groups := map[string][]fetchedFile{}
	var order []string
	for _, file := range files {
		name := promptName(path.Base(file.path))
		if _, seen := groups[name]; !seen {
			order = append(order, name)
		}
		groups[name] = append(groups[name], file)
	}
	var desired []promptservice.SyncedPromptFile
	for _, name := range order {
		group := groups[name]
		if len(group) > 1 && validPromptName(name) {
			frozen[name] = true
			for _, file := range group {
				warnings = append(warnings, fmt.Sprintf("%s: another file also defines prompt %q; neither file was applied", file.path, name))
			}
			continue
		}
		for _, file := range group {
			content, problem := validatePromptFile(name, file)
			if problem == "" {
				desired = append(desired, promptservice.SyncedPromptFile{Name: name, Content: content, Path: file.path})
				continue
			}
			warnings = append(warnings, fmt.Sprintf("%s: %s; the prompt was left unchanged", file.path, problem))
			if validPromptName(name) {
				frozen[name] = true
			}
		}
	}
	return desired, frozen, warnings
}

func validPromptName(name string) bool {
	return name != "" && len(name) <= promptservice.MaxPromptNameBytes
}

func validatePromptFile(name string, file fetchedFile) (string, string) {
	raw := file.content
	if name == "" {
		return "", "the file name has no prompt name before the .md extension"
	}
	if len(name) > promptservice.MaxPromptNameBytes {
		return "", fmt.Sprintf("the prompt name is longer than %d bytes", promptservice.MaxPromptNameBytes)
	}
	if strings.TrimSpace(name) != name {
		return "", "the prompt name starts or ends with whitespace"
	}
	if file.tooLarge {
		return "", fmt.Sprintf("the file is larger than the %d byte limit for repository reads", promptservice.MaxPromptContentBytes)
	}
	if !utf8.Valid(raw) {
		return "", "the file is not valid UTF-8 text"
	}
	content := strings.TrimSpace(string(raw))
	if content == "" {
		return "", "the file is empty"
	}
	if len(content) > promptservice.MaxPromptContentBytes {
		return "", fmt.Sprintf("the prompt content is longer than %d bytes", promptservice.MaxPromptContentBytes)
	}
	return content, ""
}
