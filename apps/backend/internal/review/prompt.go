package review

import (
	"context"
	"fmt"
	"strings"
)

// PromptSentinel is the marker the code-review prompt template must contain. The
// mock agent matches on it to return a deterministic findings payload in dev and
// E2E runs, so both trigger paths are hermetically testable.
const PromptSentinel = "KANDEV_CODE_REVIEW_REQUEST"

// TemplateSource supplies the reviewer prompt template and resolves its
// placeholders. Backed by the `code-review` utility agent's stored prompt, so a
// user who edits that prompt changes how reviews are performed.
type TemplateSource interface {
	// ReviewPromptTemplate returns the current template text.
	ReviewPromptTemplate(ctx context.Context) (string, error)
	// ResolveTemplate substitutes the {{Placeholder}} values.
	ResolveTemplate(ctx context.Context, template string, values map[string]string) (string, error)
}

// TemplatePromptBuilder renders the reviewer prompt for one batch of files.
type TemplatePromptBuilder struct {
	source TemplateSource
}

// NewTemplatePromptBuilder builds a prompt builder over a template source.
func NewTemplatePromptBuilder(source TemplateSource) *TemplatePromptBuilder {
	return &TemplatePromptBuilder{source: source}
}

// Build renders the prompt for a batch.
//
// The template's placeholders are filled from the batch rather than the whole
// change set: a batched review must only ever describe the files it actually
// contains, otherwise the reviewer anchors findings to files it was not shown.
func (b *TemplatePromptBuilder) Build(ctx context.Context, batch []ChangedFile, promptCtx PromptContext) (string, error) {
	if b.source == nil {
		return "", fmt.Errorf("no review prompt template is configured")
	}
	template, err := b.source.ReviewPromptTemplate(ctx)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(template) == "" {
		return "", fmt.Errorf("the code-review prompt template is empty")
	}

	values := map[string]string{
		"ChangedFiles":    FormatChangedFileList(batch),
		"GitDiff":         FormatBatchDiff(batch),
		"DiffSummary":     FormatDiffSummary(batch),
		"TaskTitle":       promptCtx.TaskTitle,
		"TaskDescription": promptCtx.TaskDescription,
		"BranchName":      promptCtx.BranchName,
		"BaseBranch":      promptCtx.BaseBranch,
	}
	resolved, err := b.source.ResolveTemplate(ctx, template, values)
	if err != nil {
		return "", err
	}
	// The sentinel is what makes a review request recognisable to the mock agent.
	// A user who edited the template out of shape would otherwise silently lose
	// deterministic dev/E2E behaviour.
	if !strings.Contains(resolved, PromptSentinel) {
		resolved = PromptSentinel + "\n\n" + resolved
	}
	return resolved, nil
}

// FormatChangedFileList renders the batch's files one per line, using the same
// composite `<repo>\x00<path>` identity the Review panel keys on but rendered as
// `<repo>/<path>` so a reviewer can copy it back verbatim.
func FormatChangedFileList(batch []ChangedFile) string {
	lines := make([]string, 0, len(batch))
	for _, f := range batch {
		lines = append(lines, formatFileLabel(f))
	}
	return strings.Join(lines, "\n")
}

func formatFileLabel(f ChangedFile) string {
	if f.RepositoryName == "" {
		return f.Path
	}
	return f.RepositoryName + "/" + f.Path
}

// FormatBatchDiff renders the batch's diffs with a header per file so the
// reviewer can attribute each hunk, including in a multi-repository task.
func FormatBatchDiff(batch []ChangedFile) string {
	sections := make([]string, 0, len(batch))
	for _, f := range batch {
		header := fmt.Sprintf("### %s (%s)", formatFileLabel(f), f.Status)
		if f.RepositoryName != "" {
			header = fmt.Sprintf("### repo=%s file=%s (%s)", f.RepositoryName, f.Path, f.Status)
		}
		sections = append(sections, header+"\n"+f.Diff)
	}
	return strings.Join(sections, "\n\n")
}

// FormatDiffSummary renders the batch's aggregate line counts.
func FormatDiffSummary(batch []ChangedFile) string {
	additions, deletions := 0, 0
	for _, f := range batch {
		additions += f.Additions
		deletions += f.Deletions
	}
	return fmt.Sprintf("%d file(s), +%d -%d", len(batch), additions, deletions)
}
