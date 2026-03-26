// Package workflows provides embedded workflow template definitions.
// Edit the .yml files in this directory to change the workflow templates.
package workflows

import "embed"

//go:embed *.yml
var embeddedFS embed.FS
