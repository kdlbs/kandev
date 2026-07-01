import type { ComponentProps } from "react";
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
  features: { office: false },
  workspaces: {
    items: [
      { id: "w1", name: "Default Workspace", office_workflow_id: "" },
      { id: "w2", name: "Office Workspace", office_workflow_id: "wf-office" },
    ],
    activeId: "w1",
  },
  setActiveWorkspace: vi.fn(),
};
const KANBAN_WORKSPACE_ITEM = "sidebar-workspace-item-w1";
const OFFICE_WORKSPACE_ITEM = "sidebar-workspace-item-w2";
// jsdom over http drops `secure` cookies, so intercept the setter to capture
// the write directly rather than reading `document.cookie` back.
let cookieWrites: string[] = [];
let cookieDescriptor: PropertyDescriptor | undefined;

function resetWorkspaceSelectTest() {
  navigationMock.push = vi.fn();
  storeState.features.office = false;
  storeState.workspaces.activeId = "w1";
  storeState.setActiveWorkspace = vi.fn();
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

type AppSidebarWorkspacePickerProps = ComponentProps<typeof AppSidebarWorkspacePicker>;

function renderPicker(props: AppSidebarWorkspacePickerProps = {}) {
  const queryClient = makeQueryClient();
  queryClient.setQueryData(qk.features(), { office: storeState.features.office });
  queryClient.setQueryData(qk.workspaces.all(), storeState.workspaces.items);
  return render(
    <QueryClientProvider client={queryClient}>
      <AppSidebarWorkspacePicker {...props} />
    </QueryClientProvider>,
  );
}

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
    renderPicker();

    fireEvent.click(screen.getByText("New office workspace"));

    expect(navigationMock.push).toHaveBeenCalledWith("/office/setup?mode=new");
  });

  it("routes to the settings workspaces page for a new kanban workspace", () => {
    storeState.features.office = true;
    renderPicker();

    fireEvent.click(screen.getByText("New kanban workspace"));

    expect(navigationMock.push).toHaveBeenCalledWith("/settings/workspace");
  });

  it("keeps the legacy add-workspace route when the office feature is disabled", () => {
    storeState.features.office = false;
    renderPicker();

    fireEvent.click(screen.getByText("Add workspace"));

    expect(navigationMock.push).toHaveBeenCalledWith("/settings/workspace");
  });

  it("calls onActionComplete after navigating to add a workspace", () => {
    const onActionComplete = vi.fn();
    storeState.features.office = false;
    renderPicker({ onActionComplete });

    fireEvent.click(screen.getByText("Add workspace"));

    expect(onActionComplete).toHaveBeenCalledOnce();
  });
});

describe("AppSidebarWorkspacePicker — workspace select", () => {
  beforeEach(resetWorkspaceSelectTest);

  afterEach(cleanupWorkspaceSelectTest);

  it("does nothing when selecting the already active workspace", () => {
    storeState.features.office = false;
    renderPicker();

    fireEvent.click(screen.getByTestId(KANBAN_WORKSPACE_ITEM));

    expect(cookieWrites).toEqual([]);
    expect(storeState.setActiveWorkspace).not.toHaveBeenCalled();
    expect(navigationMock.push).not.toHaveBeenCalled();
  });

  it("writes the active office workspace cookie and updates the store on office select", () => {
    storeState.features.office = false;
    renderPicker();

    fireEvent.click(screen.getByTestId(OFFICE_WORKSPACE_ITEM));

    expect(cookieWrites.some((c) => c.startsWith("office-active-workspace=w2"))).toBe(true);
    expect(storeState.setActiveWorkspace).toHaveBeenCalledWith("w2");
    expect(navigationMock.push).not.toHaveBeenCalled();
  });

  it("calls onActionComplete when selecting a different workspace", () => {
    const onActionComplete = vi.fn();
    storeState.features.office = false;
    renderPicker({ onActionComplete });

    fireEvent.click(screen.getByTestId(OFFICE_WORKSPACE_ITEM));

    expect(onActionComplete).toHaveBeenCalledOnce();
  });

  it("does not overwrite the office workspace cookie when selecting a kanban workspace", () => {
    storeState.features.office = true;
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
    storeState.features.office = true;
    renderPicker();

    fireEvent.click(screen.getByTestId(OFFICE_WORKSPACE_ITEM));

    expect(storeState.setActiveWorkspace).toHaveBeenCalledWith("w2");
    expect(navigationMock.push).toHaveBeenCalledWith("/office?workspaceId=w2");
  });

  it("navigates to the kanban board when an office user selects a kanban workspace", () => {
    storeState.features.office = true;
    storeState.workspaces.activeId = "w2";
    renderPicker();

    fireEvent.click(screen.getByTestId(KANBAN_WORKSPACE_ITEM));

    expect(cookieWrites.some((c) => c.startsWith("office-active-workspace=w2"))).toBe(true);
    expect(storeState.setActiveWorkspace).toHaveBeenCalledWith("w1");
    expect(navigationMock.push).toHaveBeenCalledWith("/?workspaceId=w1");
  });
});

describe("AppSidebarWorkspacePicker — active workspace display and routing", () => {
  beforeEach(resetWorkspaceSelectTest);

  afterEach(cleanupWorkspaceSelectTest);

  it("labels workspace types in the menu without using trigger space", () => {
    storeState.workspaces.activeId = "w1";
    renderPicker();

    expect(screen.getByTestId("sidebar-workspace-trigger").textContent).not.toContain("Kanban");
    expect(screen.getByTestId(KANBAN_WORKSPACE_ITEM).textContent).toContain("Kanban");
    expect(screen.getByTestId(OFFICE_WORKSPACE_ITEM).textContent).toContain("Office");
  });

  it("renders the trigger as an obvious selector", () => {
    renderPicker();

    const trigger = screen.getByTestId("sidebar-workspace-trigger");
    expect(trigger.getAttribute("aria-label")).toBe("Switch workspace");
    expect(trigger.textContent).toContain("Default Workspace");
    expect(screen.getByTestId("sidebar-workspace-trigger-chevron")).not.toBeNull();
  });

  it("routes to the active kanban workspace home when office routing is enabled", () => {
    storeState.features.office = true;
    renderPicker();

    fireEvent.click(screen.getByTestId(KANBAN_WORKSPACE_ITEM));

    expect(cookieWrites.some((c) => c.startsWith("kandev-active-workspace=w1"))).toBe(true);
    expect(storeState.setActiveWorkspace).not.toHaveBeenCalled();
    expect(navigationMock.push).toHaveBeenCalledWith("/?workspaceId=w1");
  });

  it("does not route when selecting the active office workspace", () => {
    storeState.features.office = true;
    storeState.workspaces.activeId = "w2";
    renderPicker();

    fireEvent.click(screen.getByTestId(OFFICE_WORKSPACE_ITEM));

    expect(cookieWrites).toEqual([]);
    expect(storeState.setActiveWorkspace).not.toHaveBeenCalled();
    expect(navigationMock.push).not.toHaveBeenCalled();
  });

  it("calls onActionComplete when re-selecting the active workspace", () => {
    const onActionComplete = vi.fn();
    storeState.features.office = false;
    renderPicker({ onActionComplete });

    fireEvent.click(screen.getByTestId(KANBAN_WORKSPACE_ITEM));

    expect(onActionComplete).toHaveBeenCalledOnce();
  });
});
