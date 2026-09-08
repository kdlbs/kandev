import type { ReactNode } from "react";
import type { FileTreeNode } from "@/lib/types/backend";
import type * as FileBrowserParts from "./file-browser-parts";
import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

const { responsive, node } = vi.hoisted(() => ({
  responsive: { isMobile: false, isFinePointer: true },
  node: { name: "README.md", path: "README.md", is_dir: false, size: 0 },
}));

vi.mock("@/hooks/use-responsive-breakpoint", () => ({
  useResponsiveBreakpoint: () => responsive,
}));
vi.mock("@/hooks/domains/session/use-session", () => ({
  useSession: () => ({ session: null, isFailed: false, errorMessage: null }),
}));
vi.mock("@/hooks/domains/workspace/use-repository", () => ({ useRepository: () => null }));
vi.mock("@/hooks/domains/session/use-session-git-status", () => ({
  useSessionGitStatus: () => null,
}));
vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: unknown) => unknown) =>
    selector({
      sessionWorktreesBySessionId: { itemsBySessionId: {} },
      workspaceFilesRefresh: { bySessionId: {} },
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
  fetchAndOpenFile: () => Promise.resolve(),
}));
vi.mock("./file-browser-path", () => ({
  getFileBrowserSessionWorkspacePath: () => null,
  resolveFileBrowserPaths: () => ({ fullPath: "/workspace", displayPath: "workspace" }),
}));
vi.mock("./file-tree-reveal", () => ({ useFileTreeReveal: () => {} }));
vi.mock("./file-tree-editor-menu", () => ({
  FileTreeEditorProvider: ({ children }: { children: ReactNode }) => <>{children}</>,
}));
vi.mock("./file-context-menu", () => ({ useFileDeleteAction: () => null }));
vi.mock("./file-browser-parts", async (importOriginal) => {
  const actual = await importOriginal<typeof FileBrowserParts>();
  return {
    ...actual,
    FileBrowserContentArea: ({
      showTouchActions,
      onAddToChatContext,
    }: {
      showTouchActions: boolean;
      onAddToChatContext: (treeNode: FileTreeNode) => void;
    }) => (
      <actual.FileTreeNodeTouchActions
        node={node}
        showTouchActions={showTouchActions}
        onAddToChatContext={onAddToChatContext}
      />
    ),
  };
});

import { FileBrowser } from "./file-browser";

function renderFileBrowser() {
  render(<FileBrowser sessionId="session-1" onOpenFile={vi.fn()} />);
}

afterEach(() => {
  cleanup();
  responsive.isMobile = false;
  responsive.isFinePointer = true;
});

describe("FileBrowser responsive touch action wiring", () => {
  it("renders the 44px action for a coarse-pointer desktop workbench", () => {
    responsive.isFinePointer = false;
    renderFileBrowser();

    const trigger = screen.getByTestId("file-tree-node-actions");
    expect(trigger.className).toContain("min-h-11");
    expect(trigger.className).toContain("min-w-11");
  });

  it("hides the touch action only for fine-pointer desktop", () => {
    renderFileBrowser();

    expect(screen.queryByTestId("file-tree-node-actions")).toBeNull();
  });

  it("renders the touch action for mobile", () => {
    responsive.isMobile = true;
    renderFileBrowser();

    expect(screen.getByTestId("file-tree-node-actions")).toBeTruthy();
  });
});
