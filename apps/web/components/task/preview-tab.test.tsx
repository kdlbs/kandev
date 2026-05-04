import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { render, fireEvent, screen, cleanup } from "@testing-library/react";

const mockPromote = vi.fn();
const mockMaximizeGroup = vi.fn();
const mockExitMaximizedLayout = vi.fn();

let mockPreMaximizeLayout: object | null = null;

vi.mock("@/lib/state/dockview-store", () => ({
  useDockviewStore: (selector: (state: Record<string, unknown>) => unknown) =>
    selector({
      promotePreviewToPinned: mockPromote,
      preMaximizeLayout: mockPreMaximizeLayout,
      sidebarGroupId: "sidebar-group",
      maximizeGroup: mockMaximizeGroup,
      exitMaximizedLayout: mockExitMaximizedLayout,
    }),
}));

vi.mock("dockview-react", () => ({
  DockviewDefaultTab: () => null,
}));

vi.mock("@kandev/ui/context-menu", () => ({
  ContextMenu: ({ children }: { children: React.ReactNode }) => <>{children}</>,
  ContextMenuTrigger: ({ children }: { children: React.ReactNode }) => <>{children}</>,
  ContextMenuContent: () => null,
  ContextMenuItem: () => null,
  ContextMenuSeparator: () => null,
}));

import { PreviewFileTab } from "./preview-tab";

type TabApi = {
  id: string;
  group: { id: string };
};

function makeProps(promoted: boolean) {
  const api: TabApi = { id: "panel-1", group: { id: "group-a" } };
  const containerApi = { getPanel: () => undefined, removePanel: () => {} };
  return {
    api,
    containerApi,
    params: { promoted },
  } as unknown as React.ComponentProps<typeof PreviewFileTab>;
}

describe("PreviewTab dblclick sequencing", () => {
  beforeEach(() => {
    mockPromote.mockClear();
    mockMaximizeGroup.mockClear();
    mockExitMaximizedLayout.mockClear();
    mockPreMaximizeLayout = null;
  });
  afterEach(() => cleanup());

  it("first dblclick on unpinned preview promotes to pinned (no maximize)", () => {
    render(<PreviewFileTab {...makeProps(false)} />);
    fireEvent.doubleClick(screen.getByTestId("preview-tab-file-editor"));
    expect(mockPromote).toHaveBeenCalledWith("file-editor");
    expect(mockMaximizeGroup).not.toHaveBeenCalled();
  });

  it("dblclick on pinned preview maximizes (no further promote)", () => {
    render(<PreviewFileTab {...makeProps(true)} />);
    fireEvent.doubleClick(screen.getByTestId("preview-tab-file-editor"));
    expect(mockPromote).not.toHaveBeenCalled();
    expect(mockMaximizeGroup).toHaveBeenCalledWith("group-a");
  });

  it("dblclick on pinned preview while maximized restores layout", () => {
    mockPreMaximizeLayout = { columns: [] };
    render(<PreviewFileTab {...makeProps(true)} />);
    fireEvent.doubleClick(screen.getByTestId("preview-tab-file-editor"));
    expect(mockExitMaximizedLayout).toHaveBeenCalledTimes(1);
    expect(mockMaximizeGroup).not.toHaveBeenCalled();
  });
});
