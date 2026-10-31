package unidiff

import (
	"context"
	"strconv"
	"strings"
)

// BuildUntrackedDiff creates the patch representation used for untracked files
// by both the agent runtime and the backend review collector.
func BuildUntrackedDiff(ctx context.Context, filePath string, content []byte, maxOutputSize int) (string, int, bool, error) {
	lines := strings.Split(string(content), "\n")
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}

	var diffBuilder strings.Builder
	diffBuilder.WriteString("diff --git a/" + filePath + " b/" + filePath + "\n")
	diffBuilder.WriteString("new file mode 100644\n")
	diffBuilder.WriteString("index 0000000..0000000\n")
	diffBuilder.WriteString("--- /dev/null\n")
	diffBuilder.WriteString("+++ b/" + filePath + "\n")
	diffBuilder.WriteString("@@ -0,0 +1," + strconv.Itoa(len(lines)) + " @@\n")
	for _, line := range lines {
		if err := ctx.Err(); err != nil {
			return "", 0, false, err
		}
		diffBuilder.WriteString("+" + line + "\n")
	}

	diffContent := diffBuilder.String()
	if len(diffContent) > maxOutputSize {
		return diffContent[:maxOutputSize], len(lines), true, nil
	}
	return diffContent, len(lines), false, nil
}
