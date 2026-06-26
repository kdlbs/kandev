import { describe, it, expect, beforeEach, vi, afterEach } from "vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { QueryClientProvider } from "@tanstack/react-query";
import { makeQueryClient } from "@/lib/query/client";
import { qk } from "@/lib/query/keys";

const navigationMock = vi.hoisted(() => ({ push: vi.fn() }));

vi.mock("@/lib/routing/client-router", () => ({
  useRouter: () => ({ push: navigationMock.push }),
}));

// Radix dropdown primitives rely on pointer/portal behaviour that jsdom doesn't
// model well. Render them as plain elements so the focus stays on the picker's
// routing logic: `onSelect` fires on click of the item.
vi.mock("@kandev/ui/dropdown-menu", () => ({
  DropdownMenu: ({ children }: { children: React.ReactNode }) => <div>{children}</div>,
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
  workspaces: {
    activeId: "w1",
  },
  setActiveWorkspace: vi.fn(),
  setActiveWorkflow: vi.fn(),
};
const KANBAN_WORKSPACE_ITEM = "sidebar-workspace-item-w1";
const OFFICE_WORKSPACE_ITEM = "sidebar-workspace-item-w2";
const workspaceItems = [
  { id: "w1", name: "Default Workspace", office_workflow_id: "" },
  { id: "w2", name: "Office Workspace", office_workflow_id: "wf-office" },
];
let officeFeature = false;

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: typeof storeState) => unknown) => selector(storeState),
}));

import { AppSidebarWorkspacePicker } from "./app-sidebar-workspace-picker";

function renderPicker() {
  const queryClient = makeQueryClient();
  queryClient.setQueryData(qk.features(), { office: officeFeature });
  queryClient.setQueryData(qk.workspaces.all(), workspaceItems);

  return render(
    <QueryClientProvider client={queryClient}>
      <AppSidebarWorkspacePicker />
    </QueryClientProvider>,
  );
}

describe("AppSidebarWorkspacePicker — Add workspace routing", () => {
  beforeEach(() => {
    navigationMock.push = vi.fn();
    officeFeature = false;
    storeState.workspaces.activeId = "w1";
    storeState.setActiveWorkspace = vi.fn();
    storeState.setActiveWorkflow = vi.fn();
  });

  afterEach(() => {
    cleanup();
  });

  it("routes to the office setup wizard when the office feature is enabled", () => {
    officeFeature = true;
    renderPicker();

    fireEvent.click(screen.getByText("New office workspace"));

    expect(navigationMock.push).toHaveBeenCalledWith("/office/setup?mode=new");
  });

  it("routes to the settings workspaces page for a new kanban workspace", () => {
    officeFeature = true;
    renderPicker();

    fireEvent.click(screen.getByText("New kanban workspace"));

    expect(navigationMock.push).toHaveBeenCalledWith("/settings/workspace");
  });

  it("keeps the legacy add-workspace route when the office feature is disabled", () => {
    officeFeature = false;
    renderPicker();

    fireEvent.click(screen.getByText("Add workspace"));

    expect(navigationMock.push).toHaveBeenCalledWith("/settings/workspace");
  });
});

describe("AppSidebarWorkspacePicker — workspace select", () => {
  // jsdom over http drops `secure` cookies, so intercept the setter to capture
  // the write directly rather than reading `document.cookie` back.
  let cookieWrites: string[] = [];
  let cookieDescriptor: PropertyDescriptor | undefined;

  beforeEach(() => {
    navigationMock.push = vi.fn();
    officeFeature = false;
    storeState.workspaces.activeId = "w1";
    storeState.setActiveWorkspace = vi.fn();
    storeState.setActiveWorkflow = vi.fn();
    cookieWrites = [];
    cookieDescriptor = Object.getOwnPropertyDescriptor(Document.prototype, "cookie");
    Object.defineProperty(document, "cookie", {
      configurable: true,
      get: () => cookieWrites.join("; "),
      set: (value: string) => {
        cookieWrites.push(value);
      },
    });
  });

  afterEach(() => {
    if (cookieDescriptor) {
      Object.defineProperty(document, "cookie", cookieDescriptor);
    }
    cleanup();
  });

  it("does nothing when selecting the already active workspace", () => {
    officeFeature = false;
    renderPicker();

    fireEvent.click(screen.getByTestId(KANBAN_WORKSPACE_ITEM));

    expect(cookieWrites).toEqual([]);
    expect(storeState.setActiveWorkspace).not.toHaveBeenCalled();
    expect(navigationMock.push).not.toHaveBeenCalled();
  });

  it("writes the active office workspace cookie and updates the store on office select", () => {
    officeFeature = false;
    renderPicker();

    fireEvent.click(screen.getByTestId(OFFICE_WORKSPACE_ITEM));

    expect(cookieWrites.some((c) => c.startsWith("office-active-workspace=w2"))).toBe(true);
    expect(storeState.setActiveWorkspace).toHaveBeenCalledWith("w2");
    expect(navigationMock.push).not.toHaveBeenCalled();
  });

  it("does not overwrite the office workspace cookie when selecting a kanban workspace", () => {
    officeFeature = true;
    storeState.workspaces.activeId = "w2";
    cookieWrites = ["office-active-workspace=w2; path=/"];
    renderPicker();

    fireEvent.click(screen.getByTestId(KANBAN_WORKSPACE_ITEM));

    expect(cookieWrites.some((c) => c.startsWith("office-active-workspace=w1"))).toBe(false);
    expect(cookieWrites.some((c) => c.startsWith("office-active-workspace=w2"))).toBe(true);
    expect(storeState.setActiveWorkspace).toHaveBeenCalledWith("w1");
    expect(navigationMock.push).toHaveBeenCalledWith("/?workspaceId=w1");
  });

  it("navigates to the office workspace when the office feature is enabled", () => {
    officeFeature = true;
    renderPicker();

    fireEvent.click(screen.getByTestId(OFFICE_WORKSPACE_ITEM));

    expect(storeState.setActiveWorkspace).toHaveBeenCalledWith("w2");
    expect(navigationMock.push).toHaveBeenCalledWith("/office?workspaceId=w2");
  });

  it("navigates to the kanban board when an office user selects a kanban workspace", () => {
    officeFeature = true;
    storeState.workspaces.activeId = "w2";
    renderPicker();

    fireEvent.click(screen.getByTestId(KANBAN_WORKSPACE_ITEM));

    expect(cookieWrites.some((c) => c.startsWith("office-active-workspace=w2"))).toBe(true);
    expect(storeState.setActiveWorkspace).toHaveBeenCalledWith("w1");
    expect(navigationMock.push).toHaveBeenCalledWith("/?workspaceId=w1");
  });

  it("labels workspace types in the menu without using trigger space", () => {
    storeState.workspaces.activeId = "w1";
    renderPicker();

    expect(screen.getByTestId("sidebar-workspace-trigger").textContent).not.toContain("Kanban");
    expect(screen.getByTestId(KANBAN_WORKSPACE_ITEM).textContent).toContain("Kanban");
    expect(screen.getByTestId(OFFICE_WORKSPACE_ITEM).textContent).toContain("Office");
  });

  it("still no-ops on the active workspace when office routing is enabled", () => {
    officeFeature = true;
    renderPicker();

    fireEvent.click(screen.getByTestId(KANBAN_WORKSPACE_ITEM));

    expect(cookieWrites).toEqual([]);
    expect(storeState.setActiveWorkspace).not.toHaveBeenCalled();
    expect(navigationMock.push).not.toHaveBeenCalled();
  });
});
