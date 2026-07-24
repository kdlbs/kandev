package review

import (
	"context"
	"errors"
	"strings"
	"testing"
)

type fakeTemplateSource struct {
	template    string
	templateErr error
	resolveErr  error
	lastValues  map[string]string
}

func (f *fakeTemplateSource) ReviewPromptTemplate(context.Context) (string, error) {
	return f.template, f.templateErr
}

func (f *fakeTemplateSource) ResolveTemplate(_ context.Context, template string, values map[string]string) (string, error) {
	if f.resolveErr != nil {
		return "", f.resolveErr
	}
	f.lastValues = values
	resolved := template
	for key, value := range values {
		resolved = strings.ReplaceAll(resolved, "{{"+key+"}}", value)
	}
	return resolved, nil
}

func batchFixture() []ChangedFile {
	return []ChangedFile{
		{Path: "a.go", Status: "modified", Diff: "@@ -1 +1,2 @@\n x\n+a\n", Additions: 1, Deletions: 0},
		{Path: "b.go", Status: "added", Diff: "@@ -0,0 +1 @@\n+b\n", Additions: 1, Deletions: 0},
	}
}

func TestTemplatePromptBuilder_FillsPlaceholdersFromBatch(t *testing.T) {
	source := &fakeTemplateSource{template: PromptSentinel + "\nfiles:\n{{ChangedFiles}}\ndiff:\n{{GitDiff}}\ntask:{{TaskTitle}}"}
	builder := NewTemplatePromptBuilder(source)

	prompt, err := builder.Build(context.Background(), batchFixture(), PromptContext{TaskTitle: "Add login"})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if !strings.Contains(prompt, "a.go") || !strings.Contains(prompt, "b.go") {
		t.Fatalf("expected both files listed, got:\n%s", prompt)
	}
	if !strings.Contains(prompt, "Add login") {
		t.Fatalf("expected the task title interpolated, got:\n%s", prompt)
	}
	if source.lastValues["DiffSummary"] != "2 file(s), +2 -0" {
		t.Fatalf("unexpected diff summary: %q", source.lastValues["DiffSummary"])
	}
}

func TestTemplatePromptBuilder_OnlyDescribesTheBatch(t *testing.T) {
	// A batched review must never mention a file it does not carry, or the
	// reviewer will anchor findings to code it was not shown.
	source := &fakeTemplateSource{template: PromptSentinel + "\n{{ChangedFiles}}\n{{GitDiff}}"}
	builder := NewTemplatePromptBuilder(source)

	prompt, err := builder.Build(context.Background(), batchFixture()[:1], PromptContext{})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if strings.Contains(prompt, "b.go") {
		t.Fatalf("prompt leaked a file outside the batch:\n%s", prompt)
	}
}

func TestTemplatePromptBuilder_AddsSentinelWhenTemplateLostIt(t *testing.T) {
	source := &fakeTemplateSource{template: "review these:\n{{GitDiff}}"}
	builder := NewTemplatePromptBuilder(source)

	prompt, err := builder.Build(context.Background(), batchFixture(), PromptContext{})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if !strings.Contains(prompt, PromptSentinel) {
		t.Fatal("a user-edited template that dropped the sentinel must still be recognisable")
	}
}

func TestTemplatePromptBuilder_MultiRepoLabelsCarryRepository(t *testing.T) {
	source := &fakeTemplateSource{template: PromptSentinel + "\n{{ChangedFiles}}\n{{GitDiff}}"}
	builder := NewTemplatePromptBuilder(source)
	batch := []ChangedFile{
		{Path: "src/a.ts", RepositoryName: "frontend", Status: "modified", Diff: "@@ -1 +1 @@\n+x\n"},
	}

	prompt, err := builder.Build(context.Background(), batch, PromptContext{})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if !strings.Contains(prompt, "frontend/src/a.ts") {
		t.Fatalf("expected a repo-qualified file label, got:\n%s", prompt)
	}
	if !strings.Contains(prompt, "repo=frontend") {
		t.Fatalf("expected the diff header to name the repository, got:\n%s", prompt)
	}
}

func TestTemplatePromptBuilder_Failures(t *testing.T) {
	cases := map[string]*TemplatePromptBuilder{
		"no source":     NewTemplatePromptBuilder(nil),
		"template err":  NewTemplatePromptBuilder(&fakeTemplateSource{templateErr: errors.New("gone")}),
		"empty":         NewTemplatePromptBuilder(&fakeTemplateSource{template: "   "}),
		"resolve error": NewTemplatePromptBuilder(&fakeTemplateSource{template: "t", resolveErr: errors.New("bad")}),
	}
	for name, builder := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := builder.Build(context.Background(), batchFixture(), PromptContext{}); err == nil {
				t.Fatal("expected an error")
			}
		})
	}
}

func TestFormatDiffSummary_EmptyBatch(t *testing.T) {
	if got := FormatDiffSummary(nil); got != "0 file(s), +0 -0" {
		t.Fatalf("unexpected summary for an empty batch: %q", got)
	}
}
