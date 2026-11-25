package backendapp

import (
	"context"
	"io"
	"testing"

	agentctltypes "github.com/kandev/kandev/internal/agentctl/types"
)

type previewTestClient struct{ reads, closes int }

func (c *previewTestClient) PrepareFilePreview(context.Context, string, string) (*agentctltypes.FilePreviewMetadata, error) {
	return &agentctltypes.FilePreviewMetadata{Token: "token", Path: "/tmp/model.stl", External: true, SizeBytes: 4}, nil
}
func (c *previewTestClient) CopyFilePreview(_ context.Context, _ string, _ int64, w io.Writer) error {
	c.reads++
	_, err := w.Write([]byte("mesh"))
	return err
}
func (c *previewTestClient) CancelFilePreview(context.Context, string) error { c.closes++; return nil }
func TestFilePreviewExecutionReplacement(t *testing.T) {
	client := &previewTestClient{}
	id := "first"
	held := 0
	resolve := func(context.Context, string) (previewExecution, error) {
		held++
		return previewExecution{id: id, workspace: "workspace", client: client, release: func() { held-- }}, nil
	}
	source, err := preparePluginFilePreview(t.Context(), resolve, "session", "repo", "model.stl")
	if err != nil {
		t.Fatal(err)
	}
	defer source.Close()
	if held != 0 || client.reads != 0 {
		t.Fatal("admission holds lease or reads body")
	}
	id = "replacement"
	if source.Read(t.Context(), io.Discard) == nil || client.reads != 0 {
		t.Fatal("read from replaced execution")
	}
	if held != 0 {
		t.Fatal("lease leaked")
	}
}
