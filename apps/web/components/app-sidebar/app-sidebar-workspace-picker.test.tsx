import { describe, it, expect, beforeEach, vi, afterEach } from "vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";

const navigationMock = vi.hoisted(() => ({ push: vi.fn() }));

vi.mock("next/navigation", () => ({
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
  }: {
    children: React.ReactNode;
    onSelect?: () => void;
    disabled?: boolean;
  }) => (
    <button type="button" disabled={disabled} onClick={() => onSelect?.()}>
      {children}
    </button>
  ),
  DropdownMenuSeparator: () => <hr />,
}));

const storeState = {
  features: { office: false },
  workspaces: {
    items: [{ id: "w1", name: "Default Workspace" }],
    activeId: "w1",
  },
  setActiveWorkspace: vi.fn(),
};

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: typeof storeState) => unknown) => selector(storeState),
}));

import { AppSidebarWorkspacePicker } from "./app-sidebar-workspace-picker";

describe("AppSidebarWorkspacePicker — Add workspace routing", () => {
  beforeEach(() => {
    navigationMock.push = vi.fn();
    storeState.features.office = false;
    storeState.setActiveWorkspace = vi.fn();
  });

  afterEach(() => {
    cleanup();
  });

  it("routes to the office setup wizard when the office feature is enabled", () => {
    storeState.features.office = true;
    render(<AppSidebarWorkspacePicker />);

    fireEvent.click(screen.getByText("Add workspace"));

    expect(navigationMock.push).toHaveBeenCalledWith("/office/setup?mode=new");
  });

  it("routes to the settings workspaces page when the office feature is disabled", () => {
    storeState.features.office = false;
    render(<AppSidebarWorkspacePicker />);

    fireEvent.click(screen.getByText("Add workspace"));

    expect(navigationMock.push).toHaveBeenCalledWith("/settings/workspace");
  });
});
