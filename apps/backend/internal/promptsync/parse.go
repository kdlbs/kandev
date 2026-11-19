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

// fetchedFile is one prompt file read from the repository.
type fetchedFile struct {
	path    string
	content []byte
}

// selectPromptFiles returns the paths of immediate Markdown files, sorted.
func selectPromptFiles(entries []reposync.Entry) []string {
	var paths []string
	for _, entry := range entries {
		if entry.IsFile && promptName(entry.Name) != "" {
			paths = append(paths, entry.Path)
		}
	}
	sort.Strings(paths)
	return paths
}

// promptName derives a prompt name from a file name, or "" when the file is
// not a Markdown file. A bare ".md" yields "" as well.
func promptName(fileName string) string {
	if len(fileName) <= len(promptFileExtension) ||
		!strings.EqualFold(fileName[len(fileName)-len(promptFileExtension):], promptFileExtension) {
		return ""
	}
	return fileName[:len(fileName)-len(promptFileExtension)]
}

// parsePromptFiles validates fetched files. Valid files become desired
// prompts; an invalid or duplicated name is frozen so its existing synced
// prompt stays unchanged, and every problem is reported as a warning.
func parsePromptFiles(files []fetchedFile) ([]promptservice.SyncedPromptFile, map[string]bool, []string) {
	frozen := map[string]bool{}
	var warnings []string
	byName := map[string][]promptservice.SyncedPromptFile{}
	var order []string
	for _, file := range files {
		name := promptName(path.Base(file.path))
		content, problem := validatePromptFile(name, file.content)
		if problem != "" {
			warnings = append(warnings, fmt.Sprintf("%s: %s; the prompt was left unchanged", file.path, problem))
			if validPromptName(name) {
				frozen[name] = true
			}
			continue
		}
		if _, seen := byName[name]; !seen {
			order = append(order, name)
		}
		byName[name] = append(byName[name], promptservice.SyncedPromptFile{Name: name, Content: content, Path: file.path})
	}
	var desired []promptservice.SyncedPromptFile
	for _, name := range order {
		candidates := byName[name]
		if len(candidates) == 1 {
			desired = append(desired, candidates[0])
			continue
		}
		frozen[name] = true
		for _, candidate := range candidates {
			warnings = append(warnings, fmt.Sprintf("%s: another file also defines prompt %q; neither file was applied", candidate.Path, name))
		}
	}
	return desired, frozen, warnings
}

func validPromptName(name string) bool {
	return name != "" && len(name) <= promptservice.MaxPromptNameBytes
}

func validatePromptFile(name string, raw []byte) (string, string) {
	if name == "" {
		return "", "the file name has no prompt name before the .md extension"
	}
	if len(name) > promptservice.MaxPromptNameBytes {
		return "", fmt.Sprintf("the prompt name is longer than %d bytes", promptservice.MaxPromptNameBytes)
	}
	if strings.TrimSpace(name) != name {
		return "", "the prompt name starts or ends with whitespace"
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
