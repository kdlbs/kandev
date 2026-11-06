import React from "react";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { FileTreeNode } from "@/lib/types/backend";
import * as clipboard from "@/lib/utils/copy-to-clipboard";
import { SearchResultsList, TreeNodeItem } from "./file-browser-parts";

const toastMock = vi.hoisted(() => vi.fn());
vi.mock("@/components/toast-provider", () => ({ useToast: () => ({ toast: toastMock }) }));
const COPY_PATH_LABEL = "Copy path";
const MENU_ITEM_ROLE = "menuitem";
const TREE_NODE_ID = "file-tree-node";

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
  toastMock.mockReset();
});

const file: FileTreeNode = {
  name: "café note.ts",
  path: "repo A/café note.ts",
  is_dir: false,
  size: 0,
};
const folder: FileTreeNode = {
  name: "資料 folder",
  path: "repo A/資料 folder",
  is_dir: true,
  size: 0,
};

function renderRow(node: FileTreeNode, selectedCount = 0, withDelete = false) {
  const onOpenFile = vi.fn();
  const onToggleExpand = vi.fn();
  const onSelect = vi.fn();
  const onAddToChatContext = vi.fn();
  const setTree = vi.fn();
  const onDeleteFile = vi.fn().mockResolvedValue(true);
  render(
    <TreeNodeItem
      row={{
        node,
        chainRoot: node,
        displayName: node.name,
        path: node.path,
        depth: 0,
        isExpanded: false,
        isDir: node.is_dir,
      }}
      activeFolderPath=""
      visibleLoadingPaths={new Set()}
      fileStatuses={new Map()}
      tree={null}
      onToggleExpand={onToggleExpand}
      onOpenFile={onOpenFile}
      onSelect={onSelect}
      onDeleteFile={withDelete ? onDeleteFile : undefined}
      setTree={setTree}
      selectedCount={selectedCount}
      selectedPaths={new Set(selectedCount > 1 ? [node.path, "other"] : [])}
      showTouchActions
      onAddToChatContext={onAddToChatContext}
    />,
  );
  return { onOpenFile, onToggleExpand, onSelect, onAddToChatContext, onDeleteFile, setTree };
}

describe("Files node Copy path", () => {
  // @covers AC-UI-FILE-TREE-PATH-COPY-001.1, .2, .5
  it.each([
    ["file", file],
    ["folder", folder],
  ] as const)(
    "copies an exact %s path from desktop without activating the row",
    async (_kind, node) => {
      const write = vi.spyOn(clipboard, "copyToClipboard").mockResolvedValue(true);
      const callbacks = renderRow(node);
      fireEvent.contextMenu(screen.getByTestId(TREE_NODE_ID));
      fireEvent.click(screen.getByRole(MENU_ITEM_ROLE, { name: COPY_PATH_LABEL }));
      await waitFor(() => expect(write).toHaveBeenCalledExactlyOnceWith(node.path));
      for (const callback of Object.values(callbacks)) expect(callback).not.toHaveBeenCalled();
    },
  );

  // @covers AC-UI-FILE-TREE-PATH-COPY-001.1, .2, .5, .6
  it.each([
    ["file", file],
    ["folder", folder],
  ] as const)(
    "copies an exact %s path from touch without activating the row",
    async (_kind, node) => {
      const write = vi.spyOn(clipboard, "copyToClipboard").mockResolvedValue(true);
      const callbacks = renderRow(node);
      fireEvent.pointerDown(screen.getByTestId("file-tree-node-actions"));
      fireEvent.click(screen.getByRole(MENU_ITEM_ROLE, { name: COPY_PATH_LABEL }));
      await waitFor(() => expect(write).toHaveBeenCalledExactlyOnceWith(node.path));
      for (const callback of Object.values(callbacks)) expect(callback).not.toHaveBeenCalled();
    },
  );
});

describe("Files node bulk and search copy", () => {
  // @covers AC-UI-FILE-TREE-PATH-COPY-001.3
  it("hides copy from the desktop bulk menu while preserving delete", () => {
    const write = vi.spyOn(clipboard, "copyToClipboard").mockResolvedValue(true);
    renderRow(file, 2, true);
    fireEvent.contextMenu(screen.getByTestId(TREE_NODE_ID));
    expect(screen.getByRole(MENU_ITEM_ROLE, { name: "Delete 2 items" })).toBeTruthy();
    expect(screen.queryByRole(MENU_ITEM_ROLE, { name: COPY_PATH_LABEL })).toBeNull();
    expect(write).not.toHaveBeenCalled();
  });

  it("hides touch bulk copy even when delete is unavailable", () => {
    const write = vi.spyOn(clipboard, "copyToClipboard").mockResolvedValue(true);
    renderRow(file, 2);
    fireEvent.pointerDown(screen.getByTestId("file-tree-node-actions"));
    expect(screen.getByRole(MENU_ITEM_ROLE, { name: "Add to chat context" })).toBeTruthy();
    expect(screen.queryByRole(MENU_ITEM_ROLE, { name: COPY_PATH_LABEL })).toBeNull();
    expect(write).not.toHaveBeenCalled();
  });

  // @covers AC-UI-FILE-TREE-PATH-COPY-001.1, .2, .5
  it.each([false, true])(
    "copies the search result identity without opening it (touch=%s)",
    async (touch) => {
      const write = vi.spyOn(clipboard, "copyToClipboard").mockResolvedValue(true);
      const open = vi.fn();
      render(
        <SearchResultsList
          searchResults={[file.path]}
          fileStatuses={new Map()}
          onOpenFile={open}
          showTouchActions={touch}
        />,
      );
      if (touch) fireEvent.pointerDown(screen.getByTestId("file-tree-node-actions"));
      else fireEvent.contextMenu(screen.getByTestId("file-search-result"));
      fireEvent.click(screen.getByRole(MENU_ITEM_ROLE, { name: COPY_PATH_LABEL }));
      await waitFor(() => expect(write).toHaveBeenCalledExactlyOnceWith(file.path));
      expect(open).not.toHaveBeenCalled();
    },
  );

  // @covers AC-UI-FILE-TREE-PATH-COPY-001.4
  it("refuses unsafe paths through the existing guard", async () => {
    const write = vi.spyOn(clipboard, "copyToClipboard").mockResolvedValue(true);
    renderRow({ ...file, path: "repo A/bad\nname.ts" });
    fireEvent.contextMenu(screen.getByTestId(TREE_NODE_ID));
    fireEvent.click(screen.getByRole(MENU_ITEM_ROLE, { name: COPY_PATH_LABEL }));
    await waitFor(() =>
      expect(toastMock).toHaveBeenCalledWith({
        description: "This path contains control characters and cannot be copied.",
      }),
    );
    expect(write).not.toHaveBeenCalled();
  });

  it("does not claim success when the existing clipboard fallback fails", async () => {
    const write = vi.spyOn(clipboard, "copyToClipboard").mockResolvedValue(false);
    renderRow(file);
    fireEvent.contextMenu(screen.getByTestId(TREE_NODE_ID));
    fireEvent.click(screen.getByRole(MENU_ITEM_ROLE, { name: COPY_PATH_LABEL }));
    await waitFor(() => expect(write).toHaveBeenCalledExactlyOnceWith(file.path));
    expect(toastMock).not.toHaveBeenCalled();
  });
});

describe("Files keyboard access", () => {
  // jsdom does not synthesize the browser's keyboard contextmenu event. The
  // native browser test covers Shift+F10; this checks each real row's focus and menu.
  it.each(["tree", "search"] as const)(
    "focuses and selects Copy path on a %s row",
    async (surface) => {
      const write = vi.spyOn(clipboard, "copyToClipboard").mockResolvedValue(true);
      const open = vi.fn();
      const callbacks = surface === "tree" ? renderRow(file) : null;
      if (surface === "search")
        render(
          <SearchResultsList
            searchResults={[file.path]}
            fileStatuses={new Map()}
            onOpenFile={open}
          />,
        );
      const row = screen.getByTestId(surface === "tree" ? TREE_NODE_ID : "file-search-result");
      expect(row.tabIndex).toBe(0);
      row.focus();
      expect(document.activeElement).toBe(row);
      fireEvent.contextMenu(row);
      const item = screen.getByRole(MENU_ITEM_ROLE, { name: COPY_PATH_LABEL });
      item.focus();
      fireEvent.keyDown(item, { key: "Enter" });
      await waitFor(() => expect(write).toHaveBeenCalledExactlyOnceWith(file.path));
      expect(open).not.toHaveBeenCalled();
      if (callbacks)
        for (const callback of Object.values(callbacks)) expect(callback).not.toHaveBeenCalled();
    },
  );
});
