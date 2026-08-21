import { describe, it, expect, beforeEach, vi, afterEach } from "vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";

// The sidebar write path scopes names with the API-origin port; pin it so the
// captured write assertions are deterministic.
vi.mock("@/lib/config", () => ({
  getBackendConfig: () => ({ apiBaseUrl: "http://localhost:8443" }),
}));

const navigationMock = vi.hoisted(() => ({ push: vi.fn() }));

vi.mock("@/lib/routing/client-router", () => ({
  useRouter: () => ({ push: navigationMock.push }),
}));

// Radix dropdown primitives rely on pointer/portal behaviour that jsdom doesn't
// model well. Render them as plain elements so the focus stays on the picker's
// routing logic: `onSelect` fires on click of the item.
vi.mock("@kandev/ui/dropdown-menu", () => ({
  DropdownMenu: ({ children, open }: { children: React.ReactNode; open?: boolean }) => (
    <div data-testid="dropdown-root" data-open={String(open)}>
      {children}
    </div>
  ),
  DropdownMenuTrigger: ({ children }: { children: React.ReactNode }) => <div>{children}</div>,
  DropdownMenuContent: ({ children }: { children: React.ReactNode }) => <div>{children}</div>,
  DropdownMenuItem: ({
    children,
    onSelect,
    disabled,
    "data-testid": testId,
  }: {
    children: React.ReactNode;
    onSelect?: () => void;
    disabled?: boolean;
    "data-testid"?: string;
  }) => (
    <button type="button" disabled={disabled} data-testid={testId} onClick={() => onSelect?.()}>
      {children}
    </button>
  ),
  DropdownMenuSeparator: () => <hr />,
}));

const storeState = {
  features: { office: false },
  workspaces: {
    items: [
      { id: "w1", name: "Default Workspace", office_workflow_id: "" },
      { id: "w2", name: "Office Workspace", office_workflow_id: "wf-office" },
      { id: "w3", name: "Alternate Workspace", office_workflow_id: "" },
    ],
    activeId: "w1",
  },
  setActiveWorkspace: vi.fn(),
  resetKanbanWorkspaceContext: vi.fn(),
};
const KANBAN_WORKSPACE_ITEM = "sidebar-workspace-item-w1";
const OFFICE_WORKSPACE_ITEM = "sidebar-workspace-item-w2";
const ALTERNATE_KANBAN_WORKSPACE_ITEM = "sidebar-workspace-item-w3";
// jsdom over http drops `secure` cookies, so intercept the setter to capture
// the write directly rather than reading `document.cookie` back.
let cookieWrites: string[] = [];
let cookieDescriptor: PropertyDescriptor | undefined;

function resetWorkspaceSelectTest() {
  navigationMock.push = vi.fn();
  storeState.features.office = false;
  storeState.workspaces.activeId = "w1";
  storeState.setActiveWorkspace = vi.fn();
  storeState.resetKanbanWorkspaceContext = vi.fn();
  cookieWrites = [];
  cookieDescriptor = Object.getOwnPropertyDescriptor(Document.prototype, "cookie");
  Object.defineProperty(document, "cookie", {
    configurable: true,
    get: () => cookieWrites.join("; "),
    set: (value: string) => {
      cookieWrites.push(value);
    },
  });
}

function cleanupWorkspaceSelectTest() {
  if (cookieDescriptor) {
    Object.defineProperty(document, "cookie", cookieDescriptor);
  }
  cleanup();
}

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: typeof storeState) => unknown) => selector(storeState),
}));

import { AppSidebarWorkspacePicker } from "./app-sidebar-workspace-picker";

describe("AppSidebarWorkspacePicker — Add workspace routing", () => {
  beforeEach(() => {
    navigationMock.push = vi.fn();
    storeState.features.office = false;
    storeState.workspaces.activeId = "w1";
    storeState.setActiveWorkspace = vi.fn();
  });

  afterEach(() => {
    cleanup();
  });

  it("routes to the office setup wizard when the office feature is enabled", () => {
    storeState.features.office = true;
    render(<AppSidebarWorkspacePicker />);

    fireEvent.click(screen.getByText("New office workspace"));

    expect(navigationMock.push).toHaveBeenCalledWith("/office/setup?mode=new");
  });

  it("routes to the settings workspaces page for a new kanban workspace", () => {
    storeState.features.office = true;
    render(<AppSidebarWorkspacePicker />);

    fireEvent.click(screen.getByText("New kanban workspace"));

    expect(navigationMock.push).toHaveBeenCalledWith("/settings/workspaces");
  });

  it("keeps the legacy add-workspace route when the office feature is disabled", () => {
    storeState.features.office = false;
    render(<AppSidebarWorkspacePicker />);

    fireEvent.click(screen.getByText("Add workspace"));

    expect(navigationMock.push).toHaveBeenCalledWith("/settings/workspaces");
  });

  it("calls onActionComplete after navigating to add a workspace", () => {
    const onActionComplete = vi.fn();
    storeState.features.office = false;
    render(<AppSidebarWorkspacePicker onActionComplete={onActionComplete} />);

    fireEvent.click(screen.getByText("Add workspace"));

    expect(onActionComplete).toHaveBeenCalledOnce();
  });
});

describe("AppSidebarWorkspacePicker — workspace select", () => {
  beforeEach(resetWorkspaceSelectTest);

  afterEach(cleanupWorkspaceSelectTest);

  it("does nothing when selecting the already active workspace", () => {
    storeState.features.office = false;
    render(<AppSidebarWorkspacePicker />);

    fireEvent.click(screen.getByTestId(KANBAN_WORKSPACE_ITEM));

    expect(cookieWrites).toEqual([]);
    expect(storeState.setActiveWorkspace).not.toHaveBeenCalled();
    expect(navigationMock.push).not.toHaveBeenCalled();
  });

  it("writes the active office workspace cookie and updates the store on office select", () => {
    storeState.features.office = false;
    render(<AppSidebarWorkspacePicker />);

    fireEvent.click(screen.getByTestId(OFFICE_WORKSPACE_ITEM));

    expect(cookieWrites.some((c) => c.startsWith("office-active-workspace_8443=w2"))).toBe(true);
    expect(storeState.setActiveWorkspace).toHaveBeenCalledWith("w2");
    expect(navigationMock.push).not.toHaveBeenCalled();
  });

  it("clears stale kanban context and routes to another kanban workspace with office disabled", () => {
    storeState.features.office = false;
    render(<AppSidebarWorkspacePicker />);

    fireEvent.click(screen.getByTestId(ALTERNATE_KANBAN_WORKSPACE_ITEM));

    expect(storeState.resetKanbanWorkspaceContext).toHaveBeenCalledOnce();
    expect(storeState.resetKanbanWorkspaceContext.mock.invocationCallOrder[0]).toBeLessThan(
      storeState.setActiveWorkspace.mock.invocationCallOrder[0],
    );
    expect(storeState.setActiveWorkspace).toHaveBeenCalledWith("w3");
    expect(navigationMock.push).toHaveBeenCalledWith("/?home=overview&workspaceId=w3");
  });

  it("calls onActionComplete when selecting a different workspace", () => {
    const onActionComplete = vi.fn();
    storeState.features.office = false;
    render(<AppSidebarWorkspacePicker onActionComplete={onActionComplete} />);

    fireEvent.click(screen.getByTestId(OFFICE_WORKSPACE_ITEM));

    expect(onActionComplete).toHaveBeenCalledOnce();
  });

  it("does not overwrite the office workspace cookie when selecting a kanban workspace", () => {
    storeState.features.office = true;
    storeState.workspaces.activeId = "w2";
    cookieWrites = ["office-active-workspace_8443=w2; path=/"];
    render(<AppSidebarWorkspacePicker />);

    fireEvent.click(screen.getByTestId(KANBAN_WORKSPACE_ITEM));

    expect(cookieWrites.some((c) => c.startsWith("office-active-workspace_8443=w1"))).toBe(false);
    expect(cookieWrites.some((c) => c.startsWith("office-active-workspace_8443=w2"))).toBe(true);
    expect(storeState.setActiveWorkspace).toHaveBeenCalledWith("w1");
    expect(navigationMock.push).toHaveBeenCalledWith("/?home=overview&workspaceId=w1");
  });

  it("navigates to the office workspace when the office feature is enabled", () => {
    storeState.features.office = true;
    render(<AppSidebarWorkspacePicker />);

    fireEvent.click(screen.getByTestId(OFFICE_WORKSPACE_ITEM));

    expect(storeState.setActiveWorkspace).toHaveBeenCalledWith("w2");
    expect(navigationMock.push).toHaveBeenCalledWith("/office?workspaceId=w2");
  });

  it("navigates to the kanban board when an office user selects a kanban workspace", () => {
    storeState.features.office = true;
    storeState.workspaces.activeId = "w2";
    // Written when w2 was selected. A workspace is recorded on the way in, not
    // on the way out, so leaving it must simply not disturb this.
    cookieWrites = ["office-active-workspace_8443=w2; path=/"];
    render(<AppSidebarWorkspacePicker />);

    fireEvent.click(screen.getByTestId(KANBAN_WORKSPACE_ITEM));

    expect(cookieWrites.some((c) => c.startsWith("office-active-workspace_8443=w1"))).toBe(false);
    expect(cookieWrites.some((c) => c.startsWith("office-active-workspace_8443=w2"))).toBe(true);
    expect(storeState.setActiveWorkspace).toHaveBeenCalledWith("w1");
    expect(navigationMock.push).toHaveBeenCalledWith("/?home=overview&workspaceId=w1");
  });
});

describe("AppSidebarWorkspacePicker — active workspace display and routing", () => {
  beforeEach(resetWorkspaceSelectTest);

  afterEach(cleanupWorkspaceSelectTest);

  it("labels workspace types in the menu without using trigger space", () => {
    storeState.workspaces.activeId = "w1";
    render(<AppSidebarWorkspacePicker />);

    expect(screen.getByTestId("sidebar-workspace-trigger").textContent).not.toContain("Kanban");
    expect(screen.getByTestId(KANBAN_WORKSPACE_ITEM).textContent).toContain("Kanban");
    expect(screen.getByTestId(OFFICE_WORKSPACE_ITEM).textContent).toContain("Office");
  });

  it("marks the active workspace with the Active badge between its name and its type", () => {
    storeState.workspaces.activeId = "w1";
    render(<AppSidebarWorkspacePicker />);

    const activeRow = screen.getByTestId(KANBAN_WORKSPACE_ITEM).textContent ?? "";
    expect(activeRow).toContain("Active");
    expect(activeRow.indexOf("Active")).toBeGreaterThan(activeRow.indexOf("Default Workspace"));
    expect(activeRow.indexOf("Active")).toBeLessThan(activeRow.indexOf("Kanban"));
  });

  it("leaves non-active workspace rows unbadged and drops the check icon", () => {
    storeState.workspaces.activeId = "w1";
    const { container } = render(<AppSidebarWorkspacePicker />);

    expect(screen.getByTestId(OFFICE_WORKSPACE_ITEM).textContent).not.toContain("Active");
    expect(screen.getByTestId(ALTERNATE_KANBAN_WORKSPACE_ITEM).textContent).not.toContain("Active");
    expect(container.querySelector(".tabler-icon-check")).toBeNull();
  });

  it("renders the trigger as an obvious selector", () => {
    render(<AppSidebarWorkspacePicker />);

    const trigger = screen.getByTestId("sidebar-workspace-trigger");
    expect(trigger.getAttribute("aria-label")).toBe("Switch workspace");
    expect(trigger.textContent).toContain("Default Workspace");
    expect(screen.getByTestId("sidebar-workspace-trigger-chevron")).not.toBeNull();
  });

  it("routes to the active kanban workspace home when office routing is enabled", () => {
    storeState.features.office = true;
    render(<AppSidebarWorkspacePicker />);

    fireEvent.click(screen.getByTestId(KANBAN_WORKSPACE_ITEM));

    expect(cookieWrites.some((c) => c.startsWith("kandev-active-workspace_8443=w1"))).toBe(true);
    // Re-selecting the workspace that is already active goes through the same
    // one selection path as any other pick; the store action ignores a
    // no-op change, so there is no reason for this branch to skip it.
    expect(storeState.setActiveWorkspace).toHaveBeenCalledWith("w1");
    expect(navigationMock.push).toHaveBeenCalledWith("/?home=overview&workspaceId=w1");
  });

  it("does not route when selecting the active office workspace", () => {
    storeState.features.office = true;
    storeState.workspaces.activeId = "w2";
    render(<AppSidebarWorkspacePicker />);

    fireEvent.click(screen.getByTestId(OFFICE_WORKSPACE_ITEM));

    expect(cookieWrites).toEqual([]);
    expect(storeState.setActiveWorkspace).not.toHaveBeenCalled();
    expect(navigationMock.push).not.toHaveBeenCalled();
  });

  it("calls onActionComplete when re-selecting the active workspace", () => {
    const onActionComplete = vi.fn();
    storeState.features.office = false;
    render(<AppSidebarWorkspacePicker onActionComplete={onActionComplete} />);

    fireEvent.click(screen.getByTestId(KANBAN_WORKSPACE_ITEM));

    expect(onActionComplete).toHaveBeenCalledOnce();
  });
});

function menuOpenState(): string | null {
  return screen.getByTestId("dropdown-root").getAttribute("data-open");
}

describe("AppSidebarWorkspacePicker — controlled open state", () => {
  beforeEach(resetWorkspaceSelectTest);

  afterEach(cleanupWorkspaceSelectTest);

  it("stays closed and self-managed when no open prop is passed", () => {
    render(<AppSidebarWorkspacePicker />);

    expect(menuOpenState()).toBe("false");
  });

  it("passes a controlled open prop straight through to the menu", () => {
    render(<AppSidebarWorkspacePicker open onOpenChange={vi.fn()} />);

    expect(menuOpenState()).toBe("true");
  });

  it("keeps the controlled value when the picker asks to close", () => {
    // A controlled owner that ignores the request keeps the menu open — proof
    // the component does not fall back to its internal state.
    render(<AppSidebarWorkspacePicker open onOpenChange={vi.fn()} />);

    fireEvent.click(screen.getByTestId(ALTERNATE_KANBAN_WORKSPACE_ITEM));

    expect(menuOpenState()).toBe("true");
  });

  it("reports the close request to the controlled owner on workspace select", () => {
    const onOpenChange = vi.fn();
    render(<AppSidebarWorkspacePicker open onOpenChange={onOpenChange} />);

    fireEvent.click(screen.getByTestId(ALTERNATE_KANBAN_WORKSPACE_ITEM));

    expect(onOpenChange).toHaveBeenCalledWith(false);
  });

  it("reports open changes while still self-managing state", () => {
    const onOpenChange = vi.fn();
    render(<AppSidebarWorkspacePicker onOpenChange={onOpenChange} />);

    fireEvent.click(screen.getByTestId(ALTERNATE_KANBAN_WORKSPACE_ITEM));

    expect(onOpenChange).toHaveBeenCalledWith(false);
    expect(menuOpenState()).toBe("false");
  });
});
