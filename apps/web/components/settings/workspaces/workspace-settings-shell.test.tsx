import { describe, it, expect, beforeEach, afterEach, vi } from "vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";

const navigationMock = vi.hoisted(() => ({ push: vi.fn() }));

vi.mock("@/lib/routing/client-router", () => ({
  useRouter: () => ({ push: navigationMock.push }),
}));

// Same treatment as the sidebar picker's test: Radix dropdown primitives rely on
// pointer/portal behaviour jsdom doesn't model, so render them as plain elements
// and let `onSelect` fire on click.
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
  features: { office: false, canvases: false },
  workspaces: {
    items: [
      { id: "w1", name: "Default Workspace", office_workflow_id: "" },
      { id: "w2", name: "Office Workspace", office_workflow_id: "wf-office" },
    ],
    activeId: "w1",
  },
  setActiveWorkspace: vi.fn(),
};

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: typeof storeState) => unknown) => selector(storeState),
}));

import { WorkspaceSettingsShell } from "./workspace-settings-shell";

const CURRENT_WORKSPACE_ITEM = "workspace-settings-switcher-item-w1";
const OTHER_WORKSPACE_ITEM = "workspace-settings-switcher-item-w2";

describe("WorkspaceSettingsShell — workspace switcher", () => {
  beforeEach(() => {
    navigationMock.push = vi.fn();
    storeState.features.office = false;
    storeState.features.canvases = false;
    storeState.workspaces.activeId = "w1";
    storeState.setActiveWorkspace = vi.fn();
  });

  afterEach(cleanup);

  it("keeps the current tab when switching to another workspace", () => {
    render(
      <WorkspaceSettingsShell workspaceId="w1" activeTab="secrets">
        <div />
      </WorkspaceSettingsShell>,
    );

    fireEvent.click(screen.getByTestId(OTHER_WORKSPACE_ITEM));

    expect(navigationMock.push).toHaveBeenCalledWith("/settings/workspaces/w2/secrets");
  });

  it("does not navigate when picking the workspace already open", () => {
    render(
      <WorkspaceSettingsShell workspaceId="w1" activeTab="repositories">
        <div />
      </WorkspaceSettingsShell>,
    );

    fireEvent.click(screen.getByTestId(CURRENT_WORKSPACE_ITEM));

    expect(navigationMock.push).not.toHaveBeenCalled();
  });

  it("never changes the active workspace — this switcher only navigates", () => {
    render(
      <WorkspaceSettingsShell workspaceId="w2" activeTab="overview">
        <div />
      </WorkspaceSettingsShell>,
    );

    fireEvent.click(screen.getByTestId(CURRENT_WORKSPACE_ITEM));

    expect(navigationMock.push).toHaveBeenCalledWith("/settings/workspaces/w1");
    expect(storeState.setActiveWorkspace).not.toHaveBeenCalled();
  });

  it("badges the active workspace after its name, not the one being edited", () => {
    storeState.workspaces.activeId = "w2";
    render(
      <WorkspaceSettingsShell workspaceId="w1" activeTab="overview">
        <div />
      </WorkspaceSettingsShell>,
    );

    const activeRow = screen.getByTestId(OTHER_WORKSPACE_ITEM).textContent ?? "";
    expect(activeRow).toContain("Active");
    expect(activeRow.indexOf("Active")).toBeGreaterThan(activeRow.indexOf("Office Workspace"));
    expect(screen.getByTestId(CURRENT_WORKSPACE_ITEM).textContent).not.toContain("Active");
  });

  it("marks the header with the Active badge only on the active workspace's page", () => {
    storeState.workspaces.activeId = "w1";
    const { unmount } = render(
      <WorkspaceSettingsShell workspaceId="w1" activeTab="overview">
        <div />
      </WorkspaceSettingsShell>,
    );

    expect(screen.getByTestId("workspace-settings-active-badge").textContent).toBe("Active");
    unmount();

    storeState.workspaces.activeId = "w2";
    render(
      <WorkspaceSettingsShell workspaceId="w1" activeTab="overview">
        <div />
      </WorkspaceSettingsShell>,
    );

    expect(screen.queryByTestId("workspace-settings-active-badge")).toBeNull();
  });

  it("names the open workspace on the trigger", () => {
    render(
      <WorkspaceSettingsShell workspaceId="w1" activeTab="overview">
        <div />
      </WorkspaceSettingsShell>,
    );

    const trigger = screen.getByTestId("workspace-settings-switcher");
    expect(trigger.getAttribute("aria-label")).toBe("Switch workspace");
    expect(trigger.textContent).toContain("Default Workspace");
  });

  it("falls back to a plain heading for an unknown workspace", () => {
    render(
      <WorkspaceSettingsShell workspaceId="missing" activeTab="overview">
        <div />
      </WorkspaceSettingsShell>,
    );

    expect(screen.queryByTestId("workspace-settings-switcher")).toBeNull();
    expect(screen.getByRole("heading", { level: 2 }).textContent).toBe("Workspace");
  });

  it("offers the office create item only when the office feature is on", () => {
    storeState.features.office = true;
    render(
      <WorkspaceSettingsShell workspaceId="w1" activeTab="overview">
        <div />
      </WorkspaceSettingsShell>,
    );

    fireEvent.click(screen.getByText("New office workspace"));

    expect(navigationMock.push).toHaveBeenCalledWith("/office/setup?mode=new");
  });
});

describe("WorkspaceSettingsShell canvas navigation", () => {
  beforeEach(() => {
    navigationMock.push = vi.fn();
    storeState.features.office = false;
    storeState.features.canvases = false;
    storeState.workspaces.activeId = "w1";
    storeState.setActiveWorkspace = vi.fn();
  });

  afterEach(cleanup);

  it("shows the Canvases tab only while the canvas feature is enabled", () => {
    storeState.features.canvases = true;
    const { unmount } = render(
      <WorkspaceSettingsShell workspaceId="w1" activeTab="canvases">
        <div />
      </WorkspaceSettingsShell>,
    );

    expect(screen.getByRole("link", { name: "Canvases" }).getAttribute("href")).toBe(
      "/settings/workspaces/w1/canvases",
    );
    unmount();

    storeState.features.canvases = false;
    render(
      <WorkspaceSettingsShell workspaceId="w1" activeTab="overview">
        <div />
      </WorkspaceSettingsShell>,
    );

    expect(screen.queryByRole("link", { name: "Canvases" })).toBeNull();
  });
});
