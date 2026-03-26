// Package utilityagents provides embedded default utility agent prompt templates.
// Edit the .md files in this directory to change the prompts.
package utilityagents

import (
	"embed"
	"strings"
)

//go:embed *.md
var fs embed.FS

// Get reads the named prompt file (without .md extension) and returns its content.
// It panics if the file does not exist, since missing prompts are a build-time error.
func Get(name string) string {
	data, err := fs.ReadFile(name + ".md")
	if err != nil {
		panic("utilityagents: missing embedded file: " + name + ".md")
	}
	return strings.TrimSpace(string(data))
}
