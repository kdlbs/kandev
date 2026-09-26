import type { ReactNode } from "react";
import type { FileTreeNode } from "@/lib/types/backend";
import type { FileBrowserRow } from "./file-browser-hooks";
import type * as FileContextMenu from "./file-context-menu";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

vi.mock("./file-context-menu", async (importOriginal) => {
  const actual = await importOriginal<typeof FileContextMenu>();
  return {
    ...actual,
    FileContextMenu: ({ children }: { children: ReactNode }) => <>{children}</>,
    useFileDeleteAction: () => null,
    useFileRename: () => ({
      isRenaming: false,
      renameValue: "",
      setRenameValue: vi.fn(),
      handleStartRename: vi.fn(),
      handleConfirmRename: vi.fn(),
      handleRenameKeyDown: vi.fn(),
    }),
    getGitStatusTextClass: () => "",
  };
});

import { shouldShowFileTreeTouchActions, TreeNodeItem } from "./file-browser-parts";

const FILE_NODE: FileTreeNode = { name: "README.md", path: "README.md", is_dir: false, size: 0 };
const DIRECTORY_NODE: FileTreeNode = { name: "src", path: "src", is_dir: true, size: 0 };
const ROW: FileBrowserRow = {
  node: FILE_NODE,
  chainRoot: FILE_NODE,
  displayName: FILE_NODE.name,
  path: FILE_NODE.path,
  depth: 0,
  isExpanded: false,
  isDir: false,
};
const DIRECTORY_ROW: FileBrowserRow = {
  node: DIRECTORY_NODE,
  chainRoot: DIRECTORY_NODE,
  displayName: DIRECTORY_NODE.name,
  path: DIRECTORY_NODE.path,
  depth: 0,
  isExpanded: false,
  isDir: true,
};

afterEach(cleanup);

describe("TreeNodeItem touch context action", () => {
  it("opens a visible row menu without invoking the row primary action", () => {
    const onOpenFile = vi.fn();
    const onAddToChatContext = vi.fn();

    render(
      <TreeNodeItem
        row={ROW}
        activeFolderPath=""
        visibleLoadingPaths={new Set()}
        fileStatuses={new Map()}
        tree={null}
        onToggleExpand={vi.fn()}
        onOpenFile={onOpenFile}
        setTree={() => {}}
        showTouchActions
        onAddToChatContext={onAddToChatContext}
      />,
    );

    const actionTrigger = screen.getByTestId("file-tree-node-actions");
    expect(actionTrigger).toBeTruthy();
    fireEvent.pointerDown(actionTrigger);

    expect(onOpenFile).not.toHaveBeenCalled();
    fireEvent.click(screen.getByTestId("file-tree-touch-add-to-chat"));
    expect(onAddToChatContext).toHaveBeenCalledWith(FILE_NODE);
  });

  it("does not render touch actions when the responsive gate is disabled", () => {
    render(
      <TreeNodeItem
        row={ROW}
        activeFolderPath=""
        visibleLoadingPaths={new Set()}
        fileStatuses={new Map()}
        tree={null}
        onToggleExpand={vi.fn()}
        onOpenFile={vi.fn()}
        setTree={() => {}}
        showTouchActions={false}
        onAddToChatContext={vi.fn()}
      />,
    );

    expect(screen.queryByTestId("file-tree-node-actions")).toBeNull();
  });

  it("passes directory nodes through the touch context action", () => {
    const onAddToChatContext = vi.fn();
    render(
      <TreeNodeItem
        row={DIRECTORY_ROW}
        activeFolderPath=""
        visibleLoadingPaths={new Set()}
        fileStatuses={new Map()}
        tree={null}
        onToggleExpand={vi.fn()}
        onOpenFile={vi.fn()}
        setTree={() => {}}
        showTouchActions
        onAddToChatContext={onAddToChatContext}
      />,
    );

    fireEvent.pointerDown(screen.getByTestId("file-tree-node-actions"));
    fireEvent.click(screen.getByTestId("file-tree-touch-add-to-chat"));

    expect(onAddToChatContext).toHaveBeenCalledWith(DIRECTORY_NODE);
  });
});

describe("FileBrowser touch action gate", () => {
  it("keeps the touch action available on a coarse-pointer desktop viewport", () => {
    expect(shouldShowFileTreeTouchActions(false, false)).toBe(true);
  });

  it("keeps the touch action available on mobile and hidden on fine-pointer desktop", () => {
    expect(shouldShowFileTreeTouchActions(true, true)).toBe(true);
    expect(shouldShowFileTreeTouchActions(false, true)).toBe(false);
  });
});

describe("TreeNodeItem row spacing", () => {
  it("keeps the touch-action row single-line with an independent 44px hit area", () => {
    render(
      <TreeNodeItem
        row={ROW}
        activeFolderPath=""
        visibleLoadingPaths={new Set()}
        fileStatuses={new Map()}
        tree={null}
        onToggleExpand={vi.fn()}
        onOpenFile={vi.fn()}
        setTree={() => {}}
        showTouchActions
        onAddToChatContext={vi.fn()}
      />,
    );

    const row = screen.getByTestId("file-tree-node");
    const name = screen.getByText(FILE_NODE.name);
    // @covers AC-UI-FILE-TREE-CHAT-CONTEXT-001.9
    expect(row.className).not.toMatch(/(?:^|\s)flex-wrap(?:\s|$)/);
    expect(row.className).toContain("min-h-11");
    expect(name.className).toContain("min-w-0");
    expect(name.className).toContain("truncate");
  });

  it("keeps fine-pointer rows on the compact layout without touch-action spacing", () => {
    render(
      <TreeNodeItem
        row={ROW}
        activeFolderPath=""
        visibleLoadingPaths={new Set()}
        fileStatuses={new Map()}
        tree={null}
        onToggleExpand={vi.fn()}
        onOpenFile={vi.fn()}
        setTree={() => {}}
        showTouchActions={false}
        onAddToChatContext={vi.fn()}
      />,
    );

    const row = screen.getByTestId("file-tree-node");
    expect(row.className).not.toContain("relative");
    expect(row.className).not.toContain("min-h-11");
    expect(row.className).not.toContain("pr-11");
  });
});
