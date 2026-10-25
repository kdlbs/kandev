package backendapp

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	editorcontroller "github.com/kandev/kandev/internal/editors/controller"
	editormodels "github.com/kandev/kandev/internal/editors/models"
	editorservice "github.com/kandev/kandev/internal/editors/service"
	editorstore "github.com/kandev/kandev/internal/editors/store"
)

type bootEditorRepository struct{ editorstore.Repository }

func (bootEditorRepository) ListEditors(context.Context) ([]*editormodels.Editor, error) {
	return nil, nil
}

func TestEditorsBootStateIncludesFolderCapability(t *testing.T) {
	commands := map[string]string{"darwin": "open", "linux": "xdg-open", "windows": "explorer.exe"}
	command := commands[runtime.GOOS]
	if command == "" {
		t.Skip("unsupported host platform")
	}
	for _, available := range []bool{false, true} {
		t.Run(map[bool]string{false: "missing", true: "installed"}[available], func(t *testing.T) {
			dir := t.TempDir()
			t.Setenv("PATH", dir)
			if available {
				if err := os.WriteFile(filepath.Join(dir, command), []byte("#!/bin/sh\nexit 0\n"), 0o700); err != nil {
					t.Fatal(err)
				}
			}
			ctrl := editorcontroller.NewController(editorservice.NewService(bootEditorRepository{}, nil, nil))
			builder := bootStateBuilder{p: routeParams{editorCtrl: ctrl}}
			state := map[string]any{}
			builder.addEditorsState(context.Background(), state)
			editors := state["editors"].(map[string]any)
			if editors["loaded"] != true || editors["folderOpeningAvailable"] != available {
				t.Fatalf("editors boot state = %#v, want loaded with capability %v", editors, available)
			}
		})
	}
}
