import type { ReactNode } from "react";
import { cleanup, render, renderHook } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { FileTreeNode } from "@/lib/types/backend";
import type { FileBrowserRow } from "./file-browser-hooks";

const fileContextMenuRender = vi.hoisted(() => vi.fn());

vi.mock("./file-context-menu", () => ({
  FileContextMenu: ({ children }: { children: ReactNode }) => {
    fileContextMenuRender();
    return <>{children}</>;
  },
  useFileDeleteAction: () => null,
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

vi.mock("@/hooks/domains/session/use-session-agentctl", () => ({
  useSessionAgentctl: () => ({ isReady: false }),
}));

vi.mock("@/lib/ws/connection", () => ({
  getWebSocketClient: () => null,
}));

import { TreeNodeItem } from "./file-browser-parts";
import { useFileBrowserTree } from "./file-browser-hooks";

const NODE: FileTreeNode = { name: "README.md", path: "README.md", is_dir: false, size: 0 };
const TREE: FileTreeNode = { name: "root", path: "", is_dir: true, size: 0 };
const ROW: FileBrowserRow = {
  node: NODE,
  chainRoot: NODE,
  displayName: NODE.name,
  path: NODE.path,
  depth: 0,
  isExpanded: false,
  isDir: false,
};

function rowProps(row: FileBrowserRow = ROW): Parameters<typeof TreeNodeItem>[0] {
  return {
    row,
    activeFolderPath: "",
    visibleLoadingPaths: new Set(),
    fileStatuses: new Map(),
    tree: null,
    onToggleExpand: vi.fn(),
    onOpenFile: vi.fn(),
    setTree: vi.fn(),
    showTouchActions: false,
  };
}

afterEach(() => {
  cleanup();
  fileContextMenuRender.mockReset();
});

describe("TreeNodeItem render isolation", () => {
  it("skips an unrelated owner render when the row inputs remain equal", () => {
    const stableProps = { ...rowProps(), tree: TREE, treeRef: { current: TREE } };
    const { rerender } = render(<TreeNodeItem {...stableProps} />);

    rerender(<TreeNodeItem {...stableProps} tree={{ ...TREE }} row={{ ...ROW }} />);

    expect(fileContextMenuRender).toHaveBeenCalledTimes(1);
  });
});

describe("useFileBrowserTree render identity", () => {
  it("keeps its aggregate result stable during an unrelated rerender", () => {
    const { result, rerender } = renderHook(() => useFileBrowserTree("session-1", "reset-1"));
    const firstResult = result.current;

    rerender();

    expect(result.current).toBe(firstResult);
  });
});
