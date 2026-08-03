import { cleanup, render, screen, waitFor } from "@testing-library/react";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { Automation } from "@/lib/types/automation";

const WORKSPACE_ID = "workspace-1";
const AUTOMATION_ID = "automation-1";
const AUTOMATIONS_SECTION = "automations";

const mocks = vi.hoisted(() => ({
  listAutomations: vi.fn(),
  listAutomationSummaries: vi.fn(),
  activeWorkspaceId: { current: "workspace-1" as string | undefined },
  sectionExpanded: { current: {} as Record<string, boolean> },
  toggleSection: vi.fn(),
  setCollapsed: vi.fn(),
}));

type MockState = {
  workspaces: { activeId: string | undefined };
  appSidebar: { sectionExpanded: Record<string, boolean> };
  toggleAppSidebarSection: typeof mocks.toggleSection;
  setAppSidebarCollapsed: typeof mocks.setCollapsed;
};

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: MockState) => unknown) =>
    selector({
      workspaces: { activeId: mocks.activeWorkspaceId.current },
      appSidebar: { sectionExpanded: mocks.sectionExpanded.current },
      toggleAppSidebarSection: mocks.toggleSection,
      setAppSidebarCollapsed: mocks.setCollapsed,
    }),
}));

vi.mock("@/lib/api/domains/automation-api", () => ({
  listAutomations: mocks.listAutomations,
  listAutomationSummaries: mocks.listAutomationSummaries,
}));

vi.mock("@/lib/routing/client-router", () => ({
  usePathname: () => `/automations/${AUTOMATION_ID}`,
  useRouter: () => ({ push: vi.fn() }),
}));

import { AutomationsSection } from "./automations-section";

function mkAutomation(overrides: Partial<Automation> = {}): Automation {
  return {
    id: AUTOMATION_ID,
    workspace_id: WORKSPACE_ID,
    name: "Nightly drift",
    enabled: true,
    max_concurrent_runs: 1,
    last_triggered_at: null,
    created_at: "2026-07-01T00:00:00Z",
    updated_at: "2026-07-01T00:00:00Z",
    triggers: [],
    ...overrides,
  } as unknown as Automation;
}

function renderSection(collapsed = false) {
  return render(
    <TooltipProvider>
      <AutomationsSection collapsed={collapsed} />
    </TooltipProvider>,
  );
}

beforeEach(() => {
  mocks.activeWorkspaceId.current = WORKSPACE_ID;
  mocks.sectionExpanded.current = {};
  mocks.listAutomations.mockResolvedValue([mkAutomation()]);
  mocks.listAutomationSummaries.mockResolvedValue([]);
});

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

describe("AutomationsSection", () => {
  it("lists the workspace's automations, each linking to its own history", async () => {
    renderSection();

    const row = await screen.findByTestId(`sidebar-automation-${AUTOMATION_ID}`);
    expect(row.getAttribute("href")).toBe(`/automations/${AUTOMATION_ID}`);
    expect(row.textContent).toContain("Nightly drift");
  });

  it("is open by default — the list IS the navigation, not a thing to discover", async () => {
    renderSection();

    await screen.findByTestId(`sidebar-automation-${AUTOMATION_ID}`);
  });

  it("marks the automation currently being read", async () => {
    renderSection();

    const row = await screen.findByTestId(`sidebar-automation-${AUTOMATION_ID}`);
    // The active treatment is the shared sidebar accent, not a bespoke style.
    expect(row.className).toContain("before:bg-primary");
  });

  it("shows health from the same derivation the runs list uses", async () => {
    mocks.listAutomationSummaries.mockResolvedValue([
      { automation_id: AUTOMATION_ID, open_runs: 1 },
    ]);

    renderSection();

    // Reaching a screen reader matters here: the dot alone carries the state.
    await waitFor(() => expect(screen.getByText("Running.")).toBeTruthy());
  });

  it("reads a disabled automation as paused, not idle", async () => {
    mocks.listAutomations.mockResolvedValue([mkAutomation({ enabled: false })]);

    renderSection();

    await waitFor(() => expect(screen.getByText("Paused.")).toBeTruthy());
  });

  it("keeps the cross-automation view reachable without picking one first", async () => {
    renderSection();

    const shortcut = await screen.findByTestId("automations-all-runs");
    expect(shortcut.getAttribute("href")).toBe("/automations");
  });

  it("invites setup when the workspace has no automations", async () => {
    mocks.listAutomations.mockResolvedValue([]);

    renderSection();

    const empty = await screen.findByTestId("sidebar-automations-empty");
    expect(empty.getAttribute("href")).toBe("/settings/automations");
  });

  it("asks for nothing while collapsed to the rail", async () => {
    // The sidebar is mounted on every page, so an off-screen list must not cost
    // two requests per navigation.
    renderSection(true);

    await waitFor(() => expect(screen.getByLabelText("Automations")).toBeTruthy());
    expect(mocks.listAutomations).not.toHaveBeenCalled();
    expect(mocks.listAutomationSummaries).not.toHaveBeenCalled();
  });

  it("asks for nothing while the section is folded shut", async () => {
    mocks.sectionExpanded.current = { [AUTOMATIONS_SECTION]: false };

    renderSection();

    await waitFor(() => expect(screen.getByText("Automations")).toBeTruthy());
    expect(mocks.listAutomations).not.toHaveBeenCalled();
  });

  it("follows the active workspace", async () => {
    mocks.activeWorkspaceId.current = "workspace-other";

    renderSection();

    await waitFor(() => expect(mocks.listAutomations).toHaveBeenCalledWith("workspace-other"));
  });
});
