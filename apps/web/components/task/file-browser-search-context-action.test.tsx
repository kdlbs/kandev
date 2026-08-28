import type { ReactNode } from "react";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { FileTreeNode } from "@/lib/types/backend";

vi.mock("./file-context-menu", () => ({
  FileContextMenu: ({
    children,
    node,
    onAddToChatContext,
  }: {
    children: ReactNode;
    node: FileTreeNode;
    onAddToChatContext?: (node: FileTreeNode) => void;
  }) => (
    <div
      data-testid="search-result-context-menu"
      onContextMenu={(event) => {
        event.preventDefault();
        onAddToChatContext?.(node);
      }}
    >
      {children}
    </div>
  ),
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

import { FileBrowserContentArea } from "./file-browser-parts";

const SEARCH_PATH = "src/components/chat-input.tsx";

function renderSearchResults({
  showTouchActions = false,
  onAddToChatContext = vi.fn(),
}: {
  showTouchActions?: boolean;
  onAddToChatContext?: (node: FileTreeNode) => void;
} = {}) {
  const props: Parameters<typeof FileBrowserContentArea>[0] = {
    isSearchActive: true,
    searchResults: [SEARCH_PATH],
    isSessionFailed: false,
    sessionError: null,
    loadState: "loaded",
    isLoadingTree: false,
    tree: null,
    loadError: null,
    creatingInPath: null,
    fileStatuses: new Map(),
    visibleRows: [],
    activeFolderPath: "",
    activeFilePath: null,
    visibleLoadingPaths: new Set(),
    onOpenFile: vi.fn(),
    onToggleExpand: vi.fn(),
    onCreateFileSubmit: vi.fn(),
    onCancelCreate: vi.fn(),
    onRetry: vi.fn(),
    setTree: vi.fn(),
    selectedCount: 0,
    selectedPaths: new Set(),
    showTouchActions,
    onAddToChatContext,
  };
  render(<FileBrowserContentArea {...props} />);
  return onAddToChatContext;
}

afterEach(cleanup);

describe("file browser search result context actions", () => {
  it("exposes the search result through the desktop context menu", () => {
    const onAddToChatContext = renderSearchResults();

    fireEvent.contextMenu(screen.getByTestId("search-result-context-menu"));

    expect(onAddToChatContext).toHaveBeenCalledWith(
      expect.objectContaining({
        path: SEARCH_PATH,
        name: "chat-input.tsx",
        is_dir: false,
      }),
    );
  });

  it("exposes the same search result action through the touch overflow trigger", () => {
    const onAddToChatContext = vi.fn();
    renderSearchResults({ showTouchActions: true, onAddToChatContext });

    const trigger = screen.getByTestId("file-tree-node-actions");
    fireEvent.pointerDown(trigger);
    fireEvent.click(screen.getByTestId("file-tree-touch-add-to-chat"));

    expect(onAddToChatContext).toHaveBeenCalledWith(
      expect.objectContaining({
        path: SEARCH_PATH,
        name: "chat-input.tsx",
        is_dir: false,
      }),
    );
  });
});
