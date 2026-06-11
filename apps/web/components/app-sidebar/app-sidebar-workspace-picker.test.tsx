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

describe("AppSidebarWorkspacePicker — workspace select", () => {
  // jsdom over http drops `secure` cookies, so intercept the setter to capture
  // the write directly rather than reading `document.cookie` back.
  let cookieWrites: string[] = [];
  let cookieDescriptor: PropertyDescriptor | undefined;

  beforeEach(() => {
    navigationMock.push = vi.fn();
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
  });

  afterEach(() => {
    if (cookieDescriptor) {
      Object.defineProperty(document, "cookie", cookieDescriptor);
    }
    cleanup();
  });

  it("writes the active-workspace cookie and updates the store on select", () => {
    storeState.features.office = false;
    render(<AppSidebarWorkspacePicker />);

    fireEvent.click(screen.getByTestId("sidebar-workspace-item-w1"));

    expect(cookieWrites.some((c) => c.startsWith("office-active-workspace=w1"))).toBe(true);
    expect(storeState.setActiveWorkspace).toHaveBeenCalledWith("w1");
    expect(navigationMock.push).not.toHaveBeenCalled();
  });

  it("navigates to the office workspace when the office feature is enabled", () => {
    storeState.features.office = true;
    render(<AppSidebarWorkspacePicker />);

    fireEvent.click(screen.getByTestId("sidebar-workspace-item-w1"));

    expect(storeState.setActiveWorkspace).toHaveBeenCalledWith("w1");
    expect(navigationMock.push).toHaveBeenCalledWith("/office?workspaceId=w1");
  });
});
