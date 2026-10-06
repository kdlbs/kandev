package workspaces

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
)

type scratchChildDirectory struct {
	path   string
	name   string
	owner  OwnershipMarker
	marked bool
}

type scratchInspection struct {
	recognized   bool
	directories  []scratchChildDirectory
	firstSymlink string
}

func discoverTaskRoots(tasksRoot string) ([]candidate, []string, error) {
	entries, err := os.ReadDir(tasksRoot)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil, nil
	}
	if err != nil {
		return nil, nil, err
	}
	roots := make([]candidate, 0)
	warnings := make([]string, 0)
	for _, entry := range entries {
		path := filepath.Join(tasksRoot, entry.Name())
		discovered, classified, discoveryWarnings, err := discoverTaskRoot(path, entry)
		if err != nil {
			return nil, nil, err
		}
		roots = append(roots, discovered...)
		warnings = append(warnings, discoveryWarnings...)
		if entry.IsDir() && !classified {
			warnings = append(warnings, "unclassified task directory kept: "+path)
		}
	}
	return roots, warnings, nil
}

func discoverTaskRoot(path string, entry os.DirEntry) ([]candidate, bool, []string, error) {
	if entry.Type()&os.ModeSymlink != 0 {
		return nil, false, nil, fmt.Errorf("symlink beneath tasks root: %s", path)
	}
	if !entry.IsDir() {
		return nil, false, nil, nil
	}
	owner, marked, err := readOwnershipMarker(path)
	if err != nil {
		if isUnrecognizedTaskRootPermission(err, entry.Name()) {
			return nil, false, nil, nil
		}
		return nil, false, nil, err
	}
	if marked {
		return []candidate{{path: path, owner: owner}}, true, nil, nil
	}
	if looksSemanticTaskDir(entry.Name()) {
		owner = OwnershipMarker{TaskDirName: entry.Name(), LayoutVersion: LayoutVersionSemantic}
		return []candidate{{path: path, owner: owner}}, true, nil, nil
	}
	return discoverScratchRoots(path, entry.Name())
}

func discoverScratchRoots(workspacePath, workspaceID string) ([]candidate, bool, []string, error) {
	children, err := os.ReadDir(workspacePath)
	if err != nil {
		if errors.Is(err, os.ErrPermission) && !isCanonicalUUID(workspaceID) {
			return nil, false, nil, nil
		}
		return nil, false, nil, err
	}
	inspection, err := inspectScratchChildren(workspacePath, workspaceID, children)
	if err != nil {
		return nil, false, nil, err
	}
	if inspection.recognized && inspection.firstSymlink != "" {
		return nil, false, nil, fmt.Errorf("symlink beneath tasks root: %s", inspection.firstSymlink)
	}
	roots, warnings := classifyScratchChildren(workspaceID, inspection)
	return roots, inspection.recognized, warnings, nil
}

func inspectScratchChildren(workspacePath, workspaceID string, children []os.DirEntry) (scratchInspection, error) {
	inspection := scratchInspection{
		recognized:  isCanonicalUUID(workspaceID),
		directories: make([]scratchChildDirectory, 0, len(children)),
	}
	for _, child := range children {
		childPath := filepath.Join(workspacePath, child.Name())
		if child.Type()&os.ModeSymlink != 0 {
			if inspection.firstSymlink == "" {
				inspection.firstSymlink = childPath
			}
			continue
		}
		if !child.IsDir() {
			continue
		}
		owner, marked, err := readOwnershipMarker(childPath)
		if err != nil {
			if isUnclassifiedScratchChildPermission(err, workspaceID, child.Name()) {
				inspection.directories = append(inspection.directories, scratchChildDirectory{
					path: childPath, name: child.Name(),
				})
				continue
			}
			return inspection, err
		}
		if marked && owner.LayoutVersion != LayoutVersionScratch {
			return inspection, fmt.Errorf("unexpected nested semantic workspace marker: %s", childPath)
		}
		if marked {
			inspection.recognized = true
		}
		inspection.directories = append(inspection.directories, scratchChildDirectory{
			path: childPath, name: child.Name(), owner: owner, marked: marked,
		})
	}
	return inspection, nil
}

func classifyScratchChildren(workspaceID string, inspection scratchInspection) ([]candidate, []string) {
	roots := make([]candidate, 0, len(inspection.directories))
	warnings := make([]string, 0)
	for _, child := range inspection.directories {
		if child.marked {
			roots = append(roots, candidate{path: child.path, owner: child.owner})
			continue
		}
		if isLegacyScratchTask(workspaceID, child.name) {
			roots = append(roots, candidate{path: child.path, owner: OwnershipMarker{
				TaskID: child.name, WorkspaceID: workspaceID,
				TaskDirName: child.name, LayoutVersion: LayoutVersionScratch,
			}})
			continue
		}
		if inspection.recognized {
			warnings = append(warnings, "unclassified task directory kept: "+child.path)
		}
	}
	return roots, warnings
}

func isLegacyScratchTask(workspaceID, taskDirName string) bool {
	return isCanonicalUUID(workspaceID) && isCanonicalUUID(taskDirName)
}

func isUnrecognizedTaskRootPermission(err error, name string) bool {
	return errors.Is(err, os.ErrPermission) && !hasTaskLayoutName(name)
}

func isUnclassifiedScratchChildPermission(err error, workspaceID, name string) bool {
	return errors.Is(err, os.ErrPermission) && !looksSemanticTaskDir(name) && !isLegacyScratchTask(workspaceID, name)
}

func hasTaskLayoutName(name string) bool {
	return isCanonicalUUID(name) || looksSemanticTaskDir(name)
}

func isCanonicalUUID(value string) bool {
	parsed, err := uuid.Parse(value)
	return err == nil && parsed.String() == strings.ToLower(value)
}

func looksSemanticTaskDir(name string) bool {
	index := strings.LastIndexByte(name, '_')
	return index > 0 && len(name[index+1:]) == 3
}
