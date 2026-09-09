package sysprompt

import (
	"testing"
	"unicode/utf8"

	promptcfg "github.com/kandev/kandev/config/prompts"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestKandevContext_UsesConditionalMCPDiscoveryGuidance(t *testing.T) {
	context := FormatKandevContext("task", "session", false)

	assert.Contains(t, context, "These instructions list selected Kandev tools, not the complete MCP catalog.")
	assert.Contains(t, context, "native tool search or discovery")
	assert.Contains(t, context, "An omitted entry here does not mean that the tool is unavailable.")
}

func TestFormatKandevContext_CanvasGuidanceFollowsCapability(t *testing.T) {
	withoutCanvas := FormatKandevContextWithOptions("task", "session", KandevContextOptions{})
	assert.NotContains(t, withoutCanvas, "create_canvas_kandev")

	withCanvas := FormatKandevContextWithOptions("task", "session", KandevContextOptions{
		IncludeCanvasGuidance: true,
	})
	for _, tool := range []string{
		"create_canvas_kandev",
		"read_canvas_authoring_skill_kandev",
		"publish_canvas_kandev",
	} {
		assert.Contains(t, withCanvas, tool)
	}
	assert.Contains(t, withCanvas, "Create the draft in Kandev before writing application files.")
	assert.Contains(t, withCanvas, "on failure, report the failure")
	assert.Contains(t, withCanvas, "do not claim publication")
	assert.Contains(t, withCanvas, "Files or a successful local build do not create a published Kandev canvas.")
}

func TestKandevContextTemplate_IsCompactEnoughForEveryTask(t *testing.T) {
	template := promptcfg.Get("kandev-context")

	require.LessOrEqual(t, len([]byte(template)), 2800)
	assert.True(t, utf8.ValidString(template))
}

func TestKandevContext_RenderedSizesStayWithinRecordedBudgets(t *testing.T) {
	ordinary := FormatKandevContext("task", "session", false)
	canvas := FormatKandevContextWithOptions("task", "session", KandevContextOptions{
		IncludeCoordinatorTaskControls: true,
		IncludeCanvasGuidance:          true,
	})

	// The 2,800-byte contract applies to the reusable raw template. Rendered
	// contexts also contain dynamic capability sections and identifiers, so
	// these ceilings detect growth without conflating the two measurements.
	require.LessOrEqual(t, len([]byte(ordinary)), 4800)
	require.LessOrEqual(t, len([]byte(canvas)), 5300)
	assert.True(t, utf8.ValidString(ordinary))
	assert.True(t, utf8.ValidString(canvas))
	t.Logf("rendered prompt sizes: ordinary=%d bytes, canvas=%d bytes", len([]byte(ordinary)), len([]byte(canvas)))
}
