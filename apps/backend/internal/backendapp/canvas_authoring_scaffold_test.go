package backendapp

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/canvas"
	"github.com/kandev/kandev/internal/mcp/canvasskill"
	"github.com/kandev/kandev/internal/plugins/manifest"
	"github.com/kandev/kandev/internal/worktree/copyfiles"
	"github.com/stretchr/testify/require"
)

func TestCanvasScaffoldFiles_AreExactAndPublishable(t *testing.T) {
	item := &canvas.Canvas{
		ID:          "1234567890abcdef",
		Title:       "Release dashboard",
		WorkspaceID: "workspace-1",
	}

	files, err := canvasScaffoldFiles(item, "Shows deployment health.")
	require.NoError(t, err)
	require.Equal(t, canvasskill.ScaffoldInventory(), scaffoldFilePaths(files))

	parsed, err := manifest.Parse(files[0].Content)
	require.NoError(t, err)
	require.NoError(t, parsed.Validate())
	require.Equal(t, "canvas-1234567890ab", parsed.ID)
	require.Equal(t, "index.html", parsed.UI.WebApps[0].Entry)

	byPath := make(map[string]string, len(files))
	for _, file := range files {
		byPath[file.Path] = string(file.Content)
	}
	require.Contains(t, byPath["index.html"], "./appearance.js")
	require.Contains(t, byPath["index.html"], "./script.js")
	require.Contains(t, byPath["appearance.js"], "window.parent")
	require.Contains(t, byPath["appearance.js"], "kandev.web_app.appearance")
	require.Contains(t, byPath["appearance.js"], "colorScheme")
	require.Contains(t, byPath["styles.css"], "--background")
	require.Contains(t, byPath["styles.css"], "color-scheme: light")
	require.NotContains(t, byPath["styles.css"], "#9eb4ff")
	require.Contains(t, byPath["script.js"], "./_kandev/v1/context")
}

func TestCanvasCoreBundle_ContainsOneReadContract(t *testing.T) {
	home := t.TempDir()
	require.NoError(t, canvasskill.EnsureMaterialized(home))

	bundle, err := canvasCoreBundle(home)
	require.NoError(t, err)
	require.Equal(t, canvasskill.CoreInventory(), bundle["core_inventory"])
	require.Equal(t, canvasskill.Inventory(), bundle["inventory"])
	require.Equal(t, canvasskill.ScaffoldInventory(), bundle["scaffold_inventory"])
	require.Equal(t, canvasskill.Version, bundle["version"])
	require.Contains(t, bundle["content"], "kandev.web_app.appearance")

	core, ok := bundle["core"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, canvasskill.CoreInventory(), core["inventory"])
	files, ok := core["files"].([]map[string]string)
	require.True(t, ok)
	require.Len(t, files, len(canvasskill.CoreInventory()))
	require.Equal(t, "2", canvasskill.Version)
}

func TestCanvasCreateResponseCarriesCurrentAuthoringVersion(t *testing.T) {
	item := &canvas.Canvas{ID: "canvas-1", Title: "Canvas", WorkspaceID: "workspace-1"}
	response := canvasCreateResponse(item, canvasSourceRoot(item.ID), "owner-1", []canvasScaffoldFile{{
		Path: "manifest.yaml", Content: []byte("id: canvas-1\n"),
	}})

	skill, ok := response["skill"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, canvasskill.Version, skill["version"])
	require.Equal(t, "2", skill["version"])
}

func TestCanvasCreateResponseShapeIsExecutorPortable(t *testing.T) {
	item := &canvas.Canvas{ID: "canvas-portable", Title: "Canvas", WorkspaceID: "workspace-1"}
	scaffold := []canvasScaffoldFile{{
		Path: "manifest.yaml", Content: []byte("id: canvas-portable\n"),
	}}
	var baseline []byte
	for _, executor := range []string{"local", "docker", "ssh"} {
		t.Run(executor, func(t *testing.T) {
			encoded, err := json.Marshal(canvasCreateResponse(
				item,
				canvasSourceRoot(item.ID),
				"owner-1",
				scaffold,
			))
			require.NoError(t, err)
			if baseline == nil {
				baseline = encoded
				return
			}
			require.JSONEq(t, string(baseline), string(encoded))
		})
	}
}

func TestMaterializeCanvasScaffold_UsesAuthenticatedFileWrites(t *testing.T) {
	client := &recordingCanvasAgentCtlClient{}
	files := []canvasScaffoldFile{{Path: "index.html", Content: []byte("<main>ok</main>\n")}}

	require.NoError(t, materializeCanvasScaffold(context.Background(), client, ".kandev/canvases/canvas-1", files))
	require.Equal(t, []string{".kandev/canvases/canvas-1/index.html"}, client.created)
	require.Len(t, client.updated, 1)
	require.Equal(t, ".kandev/canvases/canvas-1/index.html", client.updated[0].path)
	require.Equal(t, "<main>ok</main>\n", client.updated[0].content)
	require.Contains(t, client.updated[0].diff, "+<main>ok</main>")
}

func TestCanvasScaffoldDiffUsesFullInsertionRange(t *testing.T) {
	diff := canvasScaffoldDiff(".kandev/canvases/canvas-1/script.js", []byte("line one\nline two\nline three\n"))
	require.Contains(t, diff, "@@ -0,0 +1,3 @@")
	require.Contains(t, diff, "+line one\n+line two\n+line three\n")
}

type recordingCanvasAgentCtlClient struct {
	created []string
	updated []recordedCanvasFileUpdate
	files   map[string]string
}

type recordedCanvasFileUpdate struct {
	path    string
	content string
	diff    string
}

func (c *recordingCanvasAgentCtlClient) CreateFile(_ context.Context, path, _ string) (*streams.FileCreateResponse, error) {
	c.created = append(c.created, path)
	if c.files == nil {
		c.files = make(map[string]string)
	}
	c.files[path] = ""
	return &streams.FileCreateResponse{Path: path, Success: true}, nil
}

func (c *recordingCanvasAgentCtlClient) ApplyFileDiff(
	_ context.Context,
	path, diff, _, _ string,
	desiredContent *string,
) (*streams.FileUpdateResponse, error) {
	content := ""
	if desiredContent != nil {
		content = *desiredContent
	}
	c.updated = append(c.updated, recordedCanvasFileUpdate{path: path, content: content, diff: diff})
	if c.files == nil {
		c.files = make(map[string]string)
	}
	c.files[path] = content
	return &streams.FileUpdateResponse{Path: path, Success: true}, nil
}

func (c *recordingCanvasAgentCtlClient) DeleteFile(_ context.Context, path, _ string) (*streams.FileDeleteResponse, error) {
	for file := range c.files {
		if file == path || strings.HasPrefix(file, filepath.ToSlash(filepath.Join(path, ""))+"/") {
			delete(c.files, file)
		}
	}
	return &streams.FileDeleteResponse{Path: path, Success: true}, nil
}

func (c *recordingCanvasAgentCtlClient) RenameFile(_ context.Context, oldPath, newPath, _ string) (*streams.FileRenameResponse, error) {
	if c.files == nil {
		c.files = make(map[string]string)
	}
	oldPrefix := filepath.ToSlash(filepath.Join(oldPath, ""))
	for file, content := range c.files {
		if file != oldPath && !strings.HasPrefix(file, oldPrefix) {
			continue
		}
		relative := strings.TrimPrefix(file, oldPath)
		delete(c.files, file)
		c.files[newPath+relative] = content
	}
	return &streams.FileRenameResponse{OldPath: oldPath, NewPath: newPath, Success: true}, nil
}

func (c *recordingCanvasAgentCtlClient) StreamCanvasSource(context.Context, string) (io.ReadCloser, error) {
	return io.NopCloser(strings.NewReader("")), nil
}

func (c *recordingCanvasAgentCtlClient) CopyFiles(context.Context, string, []copyfiles.Entry) (canvasCopyFilesResult, error) {
	return canvasCopyFilesResult{}, nil
}

func scaffoldFilePaths(files []canvasScaffoldFile) []string {
	paths := make([]string, 0, len(files))
	for _, file := range files {
		paths = append(paths, file.Path)
	}
	return paths
}

func TestMaterializeCanvasScaffoldAtomicallyRollsBackEveryBoundary(t *testing.T) {
	files := []canvasScaffoldFile{
		{Path: "index.html", Content: []byte("<main>ok</main>\n")},
		{Path: "styles.css", Content: []byte("main {}\n")},
	}

	for _, testCase := range []struct {
		name string
		fail func(*atomicCanvasAgentCtlClient)
	}{
		{name: "marker create", fail: func(client *atomicCanvasAgentCtlClient) { client.failCreateAt = 1 }},
		{name: "scaffold create", fail: func(client *atomicCanvasAgentCtlClient) { client.failCreateAt = 2 }},
		{name: "scaffold write", fail: func(client *atomicCanvasAgentCtlClient) { client.failApplyAt = 1 }},
		{name: "final rename", fail: func(client *atomicCanvasAgentCtlClient) { client.failRename = true }},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			client := &atomicCanvasAgentCtlClient{files: make(map[string]string)}
			testCase.fail(client)

			err := materializeCanvasScaffoldAtomically(context.Background(), client, "canvas-atomic", files)
			require.Error(t, err)
			require.Empty(t, client.files, "failed canvas creation must not leave source files")
			require.ElementsMatch(t, []string{
				canvasStagingRoot("canvas-atomic"),
				canvasSourceRoot("canvas-atomic"),
			}, client.deleted)
		})
	}
}

func TestMaterializeCanvasScaffoldAtomicallyRenamesOnlyAfterAllWrites(t *testing.T) {
	client := &atomicCanvasAgentCtlClient{files: make(map[string]string)}
	files := []canvasScaffoldFile{{Path: "index.html", Content: []byte("ok\n")}}

	require.NoError(t, materializeCanvasScaffoldAtomically(context.Background(), client, "canvas-atomic", files))
	require.Empty(t, client.deleted)
	require.Contains(t, client.files, filepath.ToSlash(filepath.Join(canvasSourceRoot("canvas-atomic"), ".canvas-root")))
	require.Contains(t, client.files, filepath.ToSlash(filepath.Join(canvasSourceRoot("canvas-atomic"), "index.html")))
	for path := range client.files {
		require.NotContains(t, path, ".staging")
	}
}

type atomicCanvasAgentCtlClient struct {
	files        map[string]string
	deleted      []string
	createCount  int
	applyCount   int
	failCreateAt int
	failApplyAt  int
	failRename   bool
}

func (c *atomicCanvasAgentCtlClient) CreateFile(_ context.Context, path, _ string) (*streams.FileCreateResponse, error) {
	c.createCount++
	if c.createCount == c.failCreateAt {
		return nil, errors.New("create failed")
	}
	c.files[path] = ""
	return &streams.FileCreateResponse{Path: path, Success: true}, nil
}

func (c *atomicCanvasAgentCtlClient) ApplyFileDiff(_ context.Context, path, _, _, _ string, desiredContent *string) (*streams.FileUpdateResponse, error) {
	c.applyCount++
	if c.applyCount == c.failApplyAt {
		return nil, errors.New("write failed")
	}
	if desiredContent != nil {
		c.files[path] = *desiredContent
	}
	return &streams.FileUpdateResponse{Path: path, Success: true}, nil
}

func (c *atomicCanvasAgentCtlClient) DeleteFile(_ context.Context, path, _ string) (*streams.FileDeleteResponse, error) {
	c.deleted = append(c.deleted, path)
	for file := range c.files {
		if file == path || strings.HasPrefix(file, filepath.ToSlash(filepath.Join(path, ""))+"/") {
			delete(c.files, file)
		}
	}
	return &streams.FileDeleteResponse{Path: path, Success: true}, nil
}

func (c *atomicCanvasAgentCtlClient) RenameFile(_ context.Context, oldPath, newPath, _ string) (*streams.FileRenameResponse, error) {
	if c.failRename {
		return nil, errors.New("rename failed")
	}
	oldPrefix := filepath.ToSlash(filepath.Join(oldPath, ""))
	for file, content := range c.files {
		if file != oldPath && !strings.HasPrefix(file, oldPrefix) {
			continue
		}
		relative := strings.TrimPrefix(file, oldPath)
		delete(c.files, file)
		c.files[newPath+relative] = content
	}
	return &streams.FileRenameResponse{OldPath: oldPath, NewPath: newPath, Success: true}, nil
}

func (c *atomicCanvasAgentCtlClient) StreamCanvasSource(context.Context, string) (io.ReadCloser, error) {
	return io.NopCloser(strings.NewReader("")), nil
}

func (c *atomicCanvasAgentCtlClient) CopyFiles(context.Context, string, []copyfiles.Entry) (canvasCopyFilesResult, error) {
	return canvasCopyFilesResult{}, nil
}
