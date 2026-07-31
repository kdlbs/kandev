import React from "react";
import { render, screen, cleanup, fireEvent } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { FileTreeNode } from "@/lib/types/backend";
import type { EditorOption } from "@/lib/types/http";

vi.mock("@/components/toast-provider", () => ({
  useToast: () => ({ toast: vi.fn() }),
}));

import { FileContextMenu } from "./file-context-menu";
import { FileTreeEditorContext, type FileTreeEditorActions } from "./file-tree-editor-menu";

const FILE_NODE: FileTreeNode = {
  name: "README.md",
  path: "apps/web/README.md",
  is_dir: false,
  size: 0,
};
const DIR_NODE: FileTreeNode = { name: "web", path: "apps/web", is_dir: true, size: 0 };
const VSCODE_ID = "vscode";
const OPEN_ITEM = "file-tree-open-in-editor";

function editor(id: string, name: string): EditorOption {
  return { id, type: id, name, kind: "built_in", installed: true, enabled: true };
}

afterEach(cleanup);

function renderMenu({
  node = FILE_NODE,
  actions,
  selectedCount,
}: {
  node?: FileTreeNode;
  actions: FileTreeEditorActions | null;
  selectedCount?: number;
}) {
  render(
    <FileTreeEditorContext.Provider value={actions}>
      <FileContextMenu
        node={node}
        tree={null}
        setTree={() => {}}
        onDeleteFile={vi.fn().mockResolvedValue(true)}
        onStartRename={() => {}}
        selectedCount={selectedCount}
        selectedPaths={selectedCount ? new Set(["a", "b", "c"]) : undefined}
      >
        <div data-testid="file-row">row</div>
      </FileContextMenu>
    </FileTreeEditorContext.Provider>,
  );
  fireEvent.contextMenu(screen.getByTestId("file-row"));
}

function actionsWith(editors: EditorOption[], defaultEditorId: string) {
  const openInEditor = vi.fn();
  const resolveTarget = vi.fn(() => ({ filePath: "resolved" }));
  return {
    actions: { editors, defaultEditorId, resolveTarget, openInEditor },
    openInEditor,
    resolveTarget,
  };
}

describe("Open in editor context menu items", () => {
  it("opens the node path in the default editor", () => {
    const { actions, openInEditor } = actionsWith([editor(VSCODE_ID, "VS Code")], VSCODE_ID);
    renderMenu({ actions });

    fireEvent.click(screen.getByTestId(OPEN_ITEM));

    expect(openInEditor).toHaveBeenCalledWith(FILE_NODE, VSCODE_ID);
  });

  it("offers directories the same open action", () => {
    const { actions, openInEditor } = actionsWith([editor(VSCODE_ID, "VS Code")], VSCODE_ID);
    renderMenu({ node: DIR_NODE, actions });

    fireEvent.click(screen.getByTestId(OPEN_ITEM));

    expect(openInEditor).toHaveBeenCalledWith(DIR_NODE, VSCODE_ID);
  });

  it("labels the primary item with the default editor and lists the rest in a submenu", () => {
    const { actions } = actionsWith(
      [editor(VSCODE_ID, "VS Code"), editor("cursor", "Cursor")],
      "cursor",
    );
    renderMenu({ actions });

    expect(screen.getByTestId(OPEN_ITEM).textContent).toContain("Open in Cursor");
    expect(screen.getByTestId("file-tree-open-in-other-editor")).toBeTruthy();
  });

  it("omits the submenu when only one editor is available", () => {
    const { actions } = actionsWith([editor(VSCODE_ID, "VS Code")], VSCODE_ID);
    renderMenu({ actions });

    expect(screen.queryByTestId("file-tree-open-in-other-editor")).toBeNull();
  });

  it("hides the open action when no editors are available", () => {
    renderMenu({
      actions: {
        editors: [],
        defaultEditorId: "",
        resolveTarget: () => ({ filePath: "resolved" }),
        openInEditor: vi.fn(),
      },
    });

    expect(screen.queryByTestId(OPEN_ITEM)).toBeNull();
  });

  it("hides the open action for a node that resolves to no worktree", () => {
    const { actions } = actionsWith([editor(VSCODE_ID, "VS Code")], VSCODE_ID);
    renderMenu({ actions: { ...actions, resolveTarget: () => null } });

    expect(screen.queryByTestId(OPEN_ITEM)).toBeNull();
    expect(screen.getByText("Delete")).toBeTruthy();
  });

  it("hides the open action outside a file browser that provides editors", () => {
    renderMenu({ actions: null });

    expect(screen.queryByTestId(OPEN_ITEM)).toBeNull();
    expect(screen.getByText("Delete")).toBeTruthy();
  });

  it("hides the open action while a bulk selection is active", () => {
    const { actions } = actionsWith([editor(VSCODE_ID, "VS Code")], VSCODE_ID);
    renderMenu({ actions, selectedCount: 3 });

    expect(screen.queryByTestId(OPEN_ITEM)).toBeNull();
  });
});
