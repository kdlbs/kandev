package workflowsync

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/github"
)

const referencingExportYAML = `version: 1
type: kandev_workflow
workflows:
  - name: Dev Flow
    prompt: "Follow @principles and @team-rules"
    steps:
      - name: Todo
        position: 0
        is_start_step: true
        prompt: "Use @principles. Check @ghost."
      - name: Review
        position: 1
        prompt: "Apply @ghost and @code-review"
`

type fakeReferenceChecker struct {
	known map[string]bool
	err   error
	texts []string
}

func (f *fakeReferenceChecker) UnresolvedPromptReferences(_ context.Context, texts []string) ([][]string, error) {
	f.texts = append(f.texts, texts...)
	if f.err != nil {
		return nil, f.err
	}
	out := make([][]string, len(texts))
	for i, text := range texts {
		for _, field := range strings.Fields(text) {
			name := strings.Trim(field, "@.,")
			if strings.HasPrefix(field, "@") && !f.known[name] {
				out[i] = append(out[i], name)
			}
		}
	}
	return out, nil
}

func syncWithChecker(t *testing.T, yaml string, checker PromptReferenceChecker) *SyncResult {
	t.Helper()
	mock := github.NewMockClient()
	mock.SeedRepoFile("acme", "flows", "main", DefaultPath+"/dev.yml", []byte(yaml))
	svc, _ := setupTestService(t, mock)
	svc.SetPromptReferenceChecker(checker)
	configureWorkspace(t, svc, "ws-1")
	result, err := svc.SyncWorkspace(context.Background(), "ws-1")
	require.NoError(t, err)
	return result
}

func TestWorkflowSyncWarnsOncePerMissingReference(t *testing.T) {
	checker := &fakeReferenceChecker{known: map[string]bool{"principles": true, "code-review": true}}
	result := syncWithChecker(t, referencingExportYAML, checker)
	require.Len(t, result.Warnings, 2)
	assert.Equal(t, DefaultPath+`/dev.yml: workflow "Dev Flow" references @team-rules in its workflow prompt, which is not a saved prompt`, result.Warnings[0])
	assert.Equal(t, DefaultPath+`/dev.yml: workflow "Dev Flow" references @ghost in step(s) "Todo", "Review", which is not a saved prompt`, result.Warnings[1])
	assert.False(t, result.Unchanged)
}

func TestWorkflowSyncWithResolvedReferencesIsUnchanged(t *testing.T) {
	checker := &fakeReferenceChecker{known: map[string]bool{"principles": true, "team-rules": true, "ghost": true, "code-review": true}}
	result := syncWithChecker(t, referencingExportYAML, checker)
	assert.Empty(t, result.Warnings)
	assert.True(t, result.Unchanged)
}

func TestWorkflowSyncReferenceCheckErrorIsAWarning(t *testing.T) {
	result := syncWithChecker(t, referencingExportYAML, &fakeReferenceChecker{err: errors.New("db down")})
	require.Len(t, result.Warnings, 1)
	assert.Contains(t, result.Warnings[0], "could not check saved-prompt references")
}

func TestWorkflowSyncSkipsReferenceCheckForInvalidFiles(t *testing.T) {
	checker := &fakeReferenceChecker{}
	result := syncWithChecker(t, "not: [valid", checker)
	assert.Len(t, result.Warnings, 1, "only the parse warning")
	assert.Empty(t, checker.texts)
}
