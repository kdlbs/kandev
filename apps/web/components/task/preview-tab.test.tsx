import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { render, fireEvent, screen, cleanup } from "@testing-library/react";

const mockPromote = vi.fn();
const mockMaximizeGroup = vi.fn();
const mockExitMaximizedLayout = vi.fn();

const storeState = {
  promotePreviewToPinned: mockPromote,
  preMaximizeLayout: null as object | null,
  sidebarGroupId: "sidebar-group",
  isRestoringLayout: false,
  maximizeGroup: mockMaximizeGroup,
  exitMaximizedLayout: mockExitMaximizedLayout,
};

vi.mock("@/lib/state/dockview-store", () => ({
  useDockviewStore: Object.assign(
    (selector: (state: typeof storeState) => unknown) => selector(storeState),
    { getState: () => storeState },
  ),
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

const PREVIEW_TAB_TID = "preview-tab-file-editor";

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
    storeState.preMaximizeLayout = null;
    storeState.isRestoringLayout = false;
  });
  afterEach(() => cleanup());

  it("first dblclick on unpinned preview promotes to pinned (no maximize)", () => {
    render(<PreviewFileTab {...makeProps(false)} />);
    fireEvent.doubleClick(screen.getByTestId(PREVIEW_TAB_TID));
    expect(mockPromote).toHaveBeenCalledWith("file-editor");
    expect(mockMaximizeGroup).not.toHaveBeenCalled();
  });

  it("dblclick on pinned preview maximizes (no further promote)", () => {
    render(<PreviewFileTab {...makeProps(true)} />);
    fireEvent.doubleClick(screen.getByTestId(PREVIEW_TAB_TID));
    expect(mockPromote).not.toHaveBeenCalled();
    expect(mockMaximizeGroup).toHaveBeenCalledWith("group-a");
  });

  it("dblclick on pinned preview while maximized restores layout", () => {
    storeState.preMaximizeLayout = { columns: [] };
    render(<PreviewFileTab {...makeProps(true)} />);
    fireEvent.doubleClick(screen.getByTestId(PREVIEW_TAB_TID));
    expect(mockExitMaximizedLayout).toHaveBeenCalledTimes(1);
    expect(mockMaximizeGroup).not.toHaveBeenCalled();
  });

  it("ignores dblclick while a layout restore is in progress", () => {
    storeState.isRestoringLayout = true;
    render(<PreviewFileTab {...makeProps(true)} />);
    fireEvent.doubleClick(screen.getByTestId(PREVIEW_TAB_TID));
    expect(mockMaximizeGroup).not.toHaveBeenCalled();
    expect(mockExitMaximizedLayout).not.toHaveBeenCalled();
  });
});
