import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, render, screen } from "@testing-library/react";

const storeState = {
  features: { coordinator: false },
  workspaces: {
    items: [{ id: "w1", name: "Workspace one", office_workflow_id: "" }],
    activeId: "w1",
  },
  setActiveWorkspace: vi.fn(),
};

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: typeof storeState) => unknown) => selector(storeState),
}));

vi.mock("@/hooks/domains/workspace/use-workspace-clone", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/hooks/domains/workspace/use-workspace-clone")>()),
  useWorkspaceClone: () => ({
    source: null,
    name: "",
    setName: vi.fn(),
    pending: false,
    error: null,
    open: vi.fn(),
    close: vi.fn(),
    submit: vi.fn(),
    restoreFocus: vi.fn(),
  }),
}));

vi.mock("@/lib/routing/client-router", () => ({
  useRouter: () => ({ push: vi.fn() }),
}));

vi.mock("@kandev/ui/dropdown-menu", () => ({
  DropdownMenu: ({ children }: { children: React.ReactNode }) => <div>{children}</div>,
  DropdownMenuTrigger: ({ children }: { children: React.ReactNode }) => <div>{children}</div>,
  DropdownMenuContent: ({ children }: { children: React.ReactNode }) => <div>{children}</div>,
  DropdownMenuItem: ({ children }: { children: React.ReactNode }) => <div>{children}</div>,
  DropdownMenuSeparator: () => <hr />,
}));

vi.mock("@/app/settings/workspace/[id]/coordinators/page", () => ({
  default: ({ workspaceId }: { workspaceId: string }) => (
    <div data-testid="stub-coordinators-list">{workspaceId}</div>
  ),
}));
vi.mock("@/app/settings/workspace/[id]/coordinators/new/page", () => ({
  default: ({ workspaceId }: { workspaceId: string }) => (
    <div data-testid="stub-coordinator-new">{workspaceId}</div>
  ),
}));
vi.mock("@/app/settings/workspace/[id]/coordinators/[coordinatorId]/page", () => ({
  default: ({ workspaceId, coordinatorId }: { workspaceId: string; coordinatorId: string }) => (
    <div data-testid="stub-coordinator-editor">
      {workspaceId}:{coordinatorId}
    </div>
  ),
}));

import { renderSettingsRoute } from "./settings-routes";

const NOT_PORTED_TEXT = /still being ported/i;

describe("coordinators workspace settings routes (D3)", () => {
  afterEach(cleanup);

  it("renders the coordinators list page when the flag is on", () => {
    storeState.features.coordinator = true;
    render(renderSettingsRoute("/settings/workspaces/w1/coordinators"));

    expect(screen.getByTestId("stub-coordinators-list").textContent).toBe("w1");
    expect(screen.queryByText(NOT_PORTED_TEXT)).toBeNull();
  });

  it("renders the add-coordinator page for /new when the flag is on", () => {
    storeState.features.coordinator = true;
    render(renderSettingsRoute("/settings/workspaces/w1/coordinators/new"));

    expect(screen.getByTestId("stub-coordinator-new").textContent).toBe("w1");
  });

  it("renders the coordinator editor page with a decoded id when the flag is on", () => {
    storeState.features.coordinator = true;
    render(renderSettingsRoute("/settings/workspaces/w1/coordinators/coordinator%20one"));

    expect(screen.getByTestId("stub-coordinator-editor").textContent).toBe("w1:coordinator one");
  });

  it("falls through to the settings not-found path for the list route when the flag is off (001.2)", () => {
    storeState.features.coordinator = false;
    render(renderSettingsRoute("/settings/workspaces/w1/coordinators"));

    expect(screen.queryByTestId("stub-coordinators-list")).toBeNull();
    expect(screen.getByText(NOT_PORTED_TEXT)).toBeTruthy();
  });

  it("falls through to the settings not-found path for /new when the flag is off (001.2)", () => {
    storeState.features.coordinator = false;
    render(renderSettingsRoute("/settings/workspaces/w1/coordinators/new"));

    expect(screen.queryByTestId("stub-coordinator-new")).toBeNull();
    expect(screen.getByText(NOT_PORTED_TEXT)).toBeTruthy();
  });

  it("falls through to the settings not-found path for the editor route when the flag is off (001.2)", () => {
    storeState.features.coordinator = false;
    render(renderSettingsRoute("/settings/workspaces/w1/coordinators/c1"));

    expect(screen.queryByTestId("stub-coordinator-editor")).toBeNull();
    expect(screen.getByText(NOT_PORTED_TEXT)).toBeTruthy();
  });
});
