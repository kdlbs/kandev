import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { WorkspaceAutomationRun } from "@/lib/types/automation";

const mocks = vi.hoisted(() => ({
  listWorkspaceAutomationRuns: vi.fn(),
  push: vi.fn(),
  // The page follows the ACTIVE workspace from the store, not the boot-time
  // workspace the route hands down, so tests drive it from here.
  activeWorkspaceId: { current: undefined as string | undefined },
}));

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: { workspaces: { activeId: string | undefined } }) => unknown) =>
    selector({ workspaces: { activeId: mocks.activeWorkspaceId.current } }),
}));

vi.mock("@/lib/api/domains/automation-api", () => ({
  listWorkspaceAutomationRuns: mocks.listWorkspaceAutomationRuns,
}));

vi.mock("@/lib/routing/client-router", () => ({
  useRouter: () => ({ push: mocks.push }),
  usePathname: () => "/runs",
}));

// Radix dropdown primitives depend on pointer/portal behaviour happy-dom does
// not model. Render them flat so the assertions land on the filtering logic.
vi.mock("@kandev/ui/dropdown-menu", () => ({
  DropdownMenu: ({ children }: { children: React.ReactNode }) => <div>{children}</div>,
  DropdownMenuTrigger: ({ children }: { children: React.ReactNode }) => <div>{children}</div>,
  DropdownMenuContent: ({ children }: { children: React.ReactNode }) => <div>{children}</div>,
  DropdownMenuItem: ({
    children,
    onSelect,
    "data-testid": testId,
  }: {
    children: React.ReactNode;
    onSelect?: () => void;
    "data-testid"?: string;
  }) => (
    <button type="button" data-testid={testId} onClick={() => onSelect?.()}>
      {children}
    </button>
  ),
}));

import { RunsPageClient } from "./runs-page-client";

const WORKSPACE_ID = "ws-1";
const DRIFT_AUTOMATION = "auto-drift";
const DRIFT_NAME = "Daily km-mobile-app-v2 repo drift --all";
const DRIFT_REPORT =
  "Sweep complete across all 32 specs. Two HIGH findings — D-005 offline-first Phase 3N is stale-blocked.";

function mkRun(overrides: Partial<WorkspaceAutomationRun> = {}): WorkspaceAutomationRun {
  return {
    id: "run-1",
    automation_id: DRIFT_AUTOMATION,
    automation_name: DRIFT_NAME,
    trigger_id: "trig-1",
    trigger_type: "scheduled",
    task_id: "task-1",
    status: "succeeded",
    dedup_key: "",
    trigger_data: {},
    error_message: "",
    created_at: new Date().toISOString(),
    ...overrides,
  };
}

async function renderFeed(runs: WorkspaceAutomationRun[]) {
  mocks.listWorkspaceAutomationRuns.mockResolvedValue(runs);
  render(<RunsPageClient workspaceId={WORKSPACE_ID} />);
  if (runs.length > 0) {
    await screen.findByTestId(`run-entry-${runs[0].id}`);
  }
}

describe("RunsPageClient feed", () => {
  beforeEach(() => {
    mocks.listWorkspaceAutomationRuns.mockReset();
    mocks.push.mockReset();
    mocks.activeWorkspaceId.current = WORKSPACE_ID;
  });

  afterEach(() => cleanup());

  it("leads with what the run actually reported, not its status", async () => {
    await renderFeed([mkRun({ summary: DRIFT_REPORT })]);

    expect(screen.getByTestId("run-outcome").textContent).toContain("Two HIGH findings");
    // The automation filter carries the same name, so read the entry itself.
    expect(screen.getByTestId("run-entry-run-1").textContent).toContain(DRIFT_NAME);
  });

  it("prefers the error over a stale summary so a failure cannot read as success", async () => {
    await renderFeed([
      mkRun({
        id: "run-skipped",
        status: "skipped",
        task_id: "",
        error_message: "max_concurrent_runs=1 reached",
        summary: "Sweep complete",
      }),
    ]);

    expect(screen.getByTestId("run-outcome").textContent).toContain("max_concurrent_runs=1");
  });

  it("opens the run's transcript when it produced a task", async () => {
    await renderFeed([mkRun({ id: "run-x", task_id: "task-hidden" })]);

    fireEvent.click(screen.getByTestId("run-entry-run-x"));

    expect(mocks.push).toHaveBeenCalledWith("/tasks/task-hidden");
  });

  it("does not navigate for a run that never produced a task", async () => {
    // A schedule turned away by the concurrency cap has no transcript to open.
    await renderFeed([mkRun({ id: "run-skipped", status: "skipped", task_id: "" })]);

    fireEvent.click(screen.getByTestId("run-entry-run-skipped"));

    expect(mocks.push).not.toHaveBeenCalled();
  });

  it("invites the user to create an automation when nothing has ever run", async () => {
    await renderFeed([]);

    await screen.findByTestId("runs-empty-state");
    const cta = screen.getByRole("link", { name: "Create an automation" });
    expect(cta.getAttribute("href")).toBe("/settings/automations");
  });

  it("surfaces a load failure instead of an empty feed", async () => {
    mocks.listWorkspaceAutomationRuns.mockRejectedValue(new Error("ws down"));
    render(<RunsPageClient workspaceId={WORKSPACE_ID} />);

    expect((await screen.findByTestId("runs-error")).textContent).toContain("ws down");
  });
});

describe("RunsPageClient filters", () => {
  beforeEach(() => {
    mocks.listWorkspaceAutomationRuns.mockReset();
    mocks.push.mockReset();
    mocks.activeWorkspaceId.current = WORKSPACE_ID;
  });

  afterEach(() => cleanup());

  const MIXED_RUNS = [
    mkRun({ id: "run-ok", status: "succeeded", summary: "all good" }),
    mkRun({
      id: "run-bad",
      status: "failed",
      automation_id: "auto-nightly",
      automation_name: "Nightly build",
      error_message: "exit 1",
    }),
  ];

  it("narrows the feed to a single status", async () => {
    await renderFeed(MIXED_RUNS);

    fireEvent.click(screen.getByTestId("run-status-filter-option-failed"));

    expect(screen.getByTestId("run-entry-run-bad")).toBeTruthy();
    expect(screen.queryByTestId("run-entry-run-ok")).toBeNull();
  });

  it("narrows the feed to a single automation, offering only ones that have run", async () => {
    await renderFeed(MIXED_RUNS);

    expect(screen.getByTestId(`run-automation-filter-option-${DRIFT_AUTOMATION}`)).toBeTruthy();
    expect(screen.queryByTestId("run-automation-filter-option-auto-never-ran")).toBeNull();

    fireEvent.click(screen.getByTestId("run-automation-filter-option-auto-nightly"));

    expect(screen.getByTestId("run-entry-run-bad")).toBeTruthy();
    expect(screen.queryByTestId("run-entry-run-ok")).toBeNull();
  });

  it("offers a way back when the filters exclude everything", async () => {
    await renderFeed([mkRun({ id: "run-ok", status: "succeeded" })]);

    fireEvent.click(screen.getByTestId("run-status-filter-option-failed"));
    fireEvent.click(screen.getByRole("button", { name: "Clear filters" }));

    await waitFor(() => expect(screen.getByTestId("run-entry-run-ok")).toBeTruthy());
  });
});

describe("RunsPageClient workspace switching", () => {
  beforeEach(() => {
    mocks.listWorkspaceAutomationRuns.mockReset();
    mocks.push.mockReset();
    mocks.activeWorkspaceId.current = WORKSPACE_ID;
  });

  afterEach(() => cleanup());

  it("does not let a filter from the previous workspace empty the new one", async () => {
    // An automation id only means something in the workspace that owns it.
    // Carrying the selection across a switch used to leave the chip reading
    // "Any automation" while the filter still excluded everything — an empty
    // feed with no visible cause.
    const first = mkRun({ id: "run-a", automation_id: "auto-a", automation_name: "Alpha" });
    mocks.listWorkspaceAutomationRuns.mockResolvedValue([first]);
    const { rerender } = render(<RunsPageClient workspaceId={WORKSPACE_ID} />);
    await screen.findByTestId("run-entry-run-a");

    fireEvent.click(screen.getByTestId("run-automation-filter-option-auto-a"));
    await waitFor(() => expect(screen.getByTestId("run-entry-run-a")).toBeTruthy());

    const second = mkRun({ id: "run-b", automation_id: "auto-b", automation_name: "Beta" });
    mocks.listWorkspaceAutomationRuns.mockResolvedValue([second]);
    // Switching workspaces changes the STORE, not the prop: the route's prop is
    // the boot payload and never changes for the life of the session.
    mocks.activeWorkspaceId.current = "ws-2";
    rerender(<RunsPageClient workspaceId={WORKSPACE_ID} />);

    // The new workspace's run must be visible, not filtered out by a selection
    // that no longer names anything.
    await waitFor(() => expect(screen.getByTestId("run-entry-run-b")).toBeTruthy());
    expect(screen.queryByTestId("run-entry-run-a")).toBeNull();
  });
});

describe("RunsPageClient workspace source", () => {
  beforeEach(() => {
    mocks.listWorkspaceAutomationRuns.mockReset();
    mocks.push.mockReset();
    mocks.activeWorkspaceId.current = WORKSPACE_ID;
  });

  afterEach(() => cleanup());

  it("follows the active workspace, not the one captured at boot", async () => {
    // The route prop comes from the immutable boot payload. Querying it would
    // pin the feed to whichever workspace was active when the SPA loaded.
    mocks.activeWorkspaceId.current = "ws-live";
    mocks.listWorkspaceAutomationRuns.mockResolvedValue([]);

    render(<RunsPageClient workspaceId="ws-at-boot" />);

    await waitFor(() => expect(mocks.listWorkspaceAutomationRuns).toHaveBeenCalled());
    expect(mocks.listWorkspaceAutomationRuns.mock.calls[0][0]).toBe("ws-live");
  });

  it("falls back to the route's workspace when the store has none yet", async () => {
    mocks.activeWorkspaceId.current = undefined;
    mocks.listWorkspaceAutomationRuns.mockResolvedValue([]);

    render(<RunsPageClient workspaceId="ws-at-boot" />);

    await waitFor(() => expect(mocks.listWorkspaceAutomationRuns).toHaveBeenCalled());
    expect(mocks.listWorkspaceAutomationRuns.mock.calls[0][0]).toBe("ws-at-boot");
  });
});
