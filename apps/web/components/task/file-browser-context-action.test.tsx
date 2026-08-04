import type { ReactNode } from "react";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { FileTreeNode } from "@/lib/types/backend";
import type { FileBrowserRow } from "./file-browser-hooks";

vi.mock("./file-context-menu", () => ({
  FileContextMenu: ({ children }: { children: ReactNode }) => <>{children}</>,
  useFileRename: () => ({
    isRenaming: false,
    renameValue: "",
    setRenameValue: vi.fn(),
    handleStartRename: vi.fn(),
    handleConfirmRename: vi.fn(),
    handleRenameKeyDown: vi.fn(),
  }),
  TreeNodeName: ({ node }: { node: FileTreeNode }) => <span>{node.name}</span>,
  getGitStatusTextClass: () => "",
}));

import { TreeNodeItem } from "./file-browser-parts";

const FILE_NODE: FileTreeNode = { name: "README.md", path: "README.md", is_dir: false, size: 0 };
const ROW: FileBrowserRow = {
  node: FILE_NODE,
  chainRoot: FILE_NODE,
  displayName: FILE_NODE.name,
  path: FILE_NODE.path,
  depth: 0,
  isExpanded: false,
  isDir: false,
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
});
