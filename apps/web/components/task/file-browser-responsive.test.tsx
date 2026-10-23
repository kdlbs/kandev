import type { ReactNode } from "react";
import type { FileTreeNode } from "@/lib/types/backend";
import type * as FileBrowserParts from "./file-browser-parts";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

const openFile = vi.hoisted(() => vi.fn(() => Promise.resolve()));
const browserSession = vi.hoisted(() => ({
  current: null as null | {
    id: string;
    task_id: string;
    workspace_path: string;
    worktrees: Array<{
      repository_id: string;
      worktree_path: string;
      position: number;
      worktree_id: string;
    }>;
  },
}));

const NODE_ACTIONS_TEST_ID = "file-tree-node-actions";

const { responsive, node } = vi.hoisted(() => ({
  responsive: { isMobile: false, isFinePointer: true },
  node: { name: "README.md", path: "README.md", is_dir: false, size: 0 },
}));

vi.mock("@/hooks/use-responsive-breakpoint", () => ({
  useResponsiveBreakpoint: () => responsive,
}));
vi.mock("@/hooks/domains/session/use-session", () => ({
  useSession: () => ({ session: browserSession.current, isFailed: false, errorMessage: null }),
}));
vi.mock("@/hooks/domains/workspace/use-repository", () => ({ useRepository: () => null }));
vi.mock("@/hooks/domains/session/use-session-git-status", () => ({
  useSessionGitStatus: () => null,
}));
vi.mock("./file-browser-tree-state", () => ({ useFileTreeCacheBinding: () => undefined }));
vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: unknown) => unknown) =>
    selector({
      sessionWorktreesBySessionId: { itemsBySessionId: {} },
      workspaceFilesRefresh: { bySessionId: {} },
      worktrees: { items: {} },
      repositories: { itemsByWorkspaceId: {} },
    }),
  useAppStoreApi: () => ({
    getState: () => ({ workspaceRestoration: { byEnvironmentId: {} } }),
  }),
}));
vi.mock("@/hooks/use-open-session-folder", () => ({
  useOpenSessionFolder: () => ({ open: vi.fn() }),
}));
vi.mock("@/hooks/use-copy-to-clipboard", () => ({
  useCopyToClipboard: () => ({ copied: false, copy: vi.fn() }),
}));
vi.mock("@/components/toast-provider", () => ({ useToast: () => ({ toast: vi.fn() }) }));
vi.mock("react-i18next", () => ({ useTranslation: () => ({ t: (key: string) => key }) }));
vi.mock("@/hooks/use-multi-select", () => ({
  useMultiSelect: () => ({
    selectedPaths: new Set(),
    setSelectedPaths: vi.fn(),
    isSelected: () => false,
    handleClick: () => false,
    clearSelection: vi.fn(),
    selectAll: vi.fn(),
  }),
}));
vi.mock("@/lib/state/context-files-store", () => ({
  useContextFilesStore: (selector: (state: { addFile: () => void }) => unknown) =>
    selector({ addFile: vi.fn() }),
}));
vi.mock("./file-browser-header", () => ({ FileBrowserHeader: () => null }));
vi.mock("./file-browser-hooks", () => ({
  useFileBrowserSearch: () => ({}),
  useFileBrowserTree: () => ({
    tree: node,
    isLoadingTree: false,
    loadState: "loaded",
    loadError: null,
    visibleRows: [],
    visibleLoadingPaths: new Set(),
    expandedPaths: new Set(),
    setTree: vi.fn(),
    setExpandedPaths: vi.fn(),
    isLoading: false,
    loadTree: vi.fn(),
  }),
  useScrollPersistence: () => {},
  loadNodeChildren: vi.fn(),
  toggleFolderExpand: vi.fn(),
  fetchAndOpenFile: (...args: unknown[]) =>
    (openFile as (...args: unknown[]) => Promise<void>)(...args),
}));
vi.mock("./file-browser-path", () => ({
  getFileBrowserSessionWorkspacePath: (session: { workspace_path?: string } | null) =>
    session?.workspace_path ?? null,
  resolveFileBrowserPaths: () => ({ fullPath: "/workspace", displayPath: "workspace" }),
}));
vi.mock("./file-tree-reveal", () => ({ useFileTreeReveal: () => {} }));
vi.mock("./file-tree-editor-menu", () => ({
  FileTreeEditorProvider: ({ children }: { children: ReactNode }) => <>{children}</>,
}));
vi.mock("./file-context-menu", () => ({ useFileDeleteAction: () => null }));
vi.mock("./file-browser-content-area", async () => {
  const actual = await vi.importActual<typeof FileBrowserParts>("./file-browser-parts");
  return {
    ...actual,
    FileBrowserContentArea: ({
      showTouchActions,
      onAddToChatContext,
      onOpenFile,
    }: {
      showTouchActions: boolean;
      onAddToChatContext: (treeNode: FileTreeNode) => void;
      onOpenFile: (path: string) => void;
    }) => (
      <>
        <button onClick={() => onOpenFile("secondary/README.md")}>Open secondary file</button>
        <actual.FileTreeNodeTouchActions
          node={node}
          showTouchActions={showTouchActions}
          onAddToChatContext={onAddToChatContext}
        />
      </>
    ),
  };
});

import { FileBrowser } from "./file-browser";

function renderFileBrowser() {
  render(<FileBrowser sessionId="session-1" onOpenFile={vi.fn()} />);
}

afterEach(() => {
  cleanup();
  browserSession.current = null;
  openFile.mockClear();
  responsive.isMobile = false;
  responsive.isFinePointer = true;
});

describe("FileBrowser responsive touch action wiring", () => {
  it("releases row-owned confirmations when the file session changes", () => {
    responsive.isMobile = true;
    const { rerender } = render(<FileBrowser sessionId="session-1" onOpenFile={vi.fn()} />);
    const trigger = screen.getByTestId(NODE_ACTIONS_TEST_ID);
    rerender(<FileBrowser sessionId="session-2" onOpenFile={vi.fn()} />);
    expect(trigger.isConnected).toBe(false);
    expect(screen.getByTestId(NODE_ACTIONS_TEST_ID)).not.toBe(trigger);
  });
  it("renders the 44px action for a coarse-pointer desktop workbench", () => {
    responsive.isFinePointer = false;
    renderFileBrowser();

    const trigger = screen.getByTestId(NODE_ACTIONS_TEST_ID);
    expect(trigger.className).toContain("min-h-11");
    expect(trigger.className).toContain("min-w-11");
  });

  it("hides the touch action only for fine-pointer desktop", () => {
    renderFileBrowser();

    expect(screen.queryByTestId(NODE_ACTIONS_TEST_ID)).toBeNull();
  });

  it("renders the touch action for mobile", () => {
    responsive.isMobile = true;
    renderFileBrowser();

    expect(screen.getByTestId(NODE_ACTIONS_TEST_ID)).toBeTruthy();
  });
});

// @covers AC-UI-MARKDOWN-FILE-EDITING-002.13
it("passes the current checkout inventory when opening a workspace-prefixed tree file", async () => {
  browserSession.current = {
    id: "session-1",
    task_id: "task-1",
    workspace_path: "/tmp/task-root",
    worktrees: [
      {
        repository_id: "secondary-id",
        worktree_path: "/tmp/task-root/secondary",
        position: 1,
        worktree_id: "wt-secondary",
      },
    ],
  };
  renderFileBrowser();
  fireEvent.click(screen.getByRole("button", { name: "Open secondary file" }));
  await waitFor(() =>
    expect(openFile).toHaveBeenCalledWith(
      "session-1",
      "secondary/README.md",
      expect.any(Function),
      expect.any(Function),
      expect.objectContaining({
        workspacePath: "/tmp/task-root",
        worktrees: browserSession.current?.worktrees,
      }),
    ),
  );
});
