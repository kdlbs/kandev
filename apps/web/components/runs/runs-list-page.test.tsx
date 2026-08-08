import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { Automation, AutomationSummary, WorkspaceAutomationRun } from "@/lib/types/automation";

const WORKSPACE_ID = "workspace-1";
const AUTOMATION_ID = "automation-1";
const NIGHTLY = "Nightly drift";

const mocks = vi.hoisted(() => ({
  listAutomations: vi.fn(),
  listAutomationSummaries: vi.fn(),
  listWorkspaceAutomationRuns: vi.fn(),
  push: vi.fn(),
  activeWorkspaceId: { current: undefined as string | undefined },
}));

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: { workspaces: { activeId: string | undefined } }) => unknown) =>
    selector({ workspaces: { activeId: mocks.activeWorkspaceId.current } }),
}));

vi.mock("@/lib/api/domains/automation-api", () => ({
  listAutomations: mocks.listAutomations,
  listAutomationSummaries: mocks.listAutomationSummaries,
  listWorkspaceAutomationRuns: mocks.listWorkspaceAutomationRuns,
}));

vi.mock("@/lib/routing/client-router", () => ({
  useRouter: () => ({ push: mocks.push }),
  usePathname: () => "/automations",
}));

import { STATE_DOT_CLASS } from "./automation-rows";
import { RunsListPage } from "./runs-list-page";

/** The dot class the shared derivation paints a `running` automation with. */
const STATE_DOT_RUNNING = STATE_DOT_CLASS.running;

function scheduled(id: string, cron: string) {
  return [{ id, type: "scheduled", enabled: true, config: { cron_expression: cron } }];
}

function mkAutomation(overrides: Record<string, unknown> = {}): Automation {
  return {
    id: AUTOMATION_ID,
    workspace_id: WORKSPACE_ID,
    name: NIGHTLY,
    enabled: true,
    max_concurrent_runs: 1,
    last_triggered_at: null,
    created_at: "2026-07-01T00:00:00Z",
    updated_at: "2026-07-01T00:00:00Z",
    triggers: [],
    ...overrides,
  } as unknown as Automation;
}

function summary(overrides: Partial<AutomationSummary> = {}): AutomationSummary {
  return { automation_id: AUTOMATION_ID, open_runs: 0, ...overrides };
}

function mkRun(overrides: Partial<WorkspaceAutomationRun> = {}): WorkspaceAutomationRun {
  return {
    id: "run-1",
    automation_id: AUTOMATION_ID,
    automation_name: NIGHTLY,
    trigger_id: "t1",
    trigger_type: "scheduled",
    task_id: "task-1",
    session_id: "session-1",
    status: "succeeded",
    dedup_key: "",
    trigger_data: {},
    error_message: "",
    summary: "Found 3 drifted specs.",
    created_at: "2026-07-30T16:00:00Z",
    ...overrides,
  } as WorkspaceAutomationRun;
}

beforeEach(() => {
  mocks.activeWorkspaceId.current = WORKSPACE_ID;
  mocks.listAutomations.mockResolvedValue([mkAutomation()]);
  mocks.listAutomationSummaries.mockResolvedValue([]);
  mocks.listWorkspaceAutomationRuns.mockResolvedValue([]);
});

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
  vi.useRealTimers();
});

describe("RunsListPage", () => {
  it("leads with the agenda — the one fact the sidebar cannot hold", async () => {
    // The sidebar already lists automations by name with a health dot. This
    // page has to earn its click, and the time is what it earns it with.
    render(<RunsListPage workspaceId={WORKSPACE_ID} />);

    await screen.findByTestId("automation-agenda");
    expect(screen.getByTestId("automation-next-run").textContent).toBeTruthy();
  });

  it("orders the agenda by when things fire, soonest first", async () => {
    // Time is pinned away from the hour before midnight UTC. There, an hourly
    // and a daily-at-midnight schedule resolve to the same instant, the sort is
    // a tie, and this case failed for an hour a day on the wall clock alone.
    vi.useFakeTimers();
    vi.setSystemTime(new Date("2026-07-31T10:00:00Z"));
    const soon = mkAutomation({
      id: "soon",
      name: "Hourly",
      triggers: scheduled("t-soon", "0 * * * *"),
    });
    const later = mkAutomation({
      id: "later",
      name: "Daily",
      triggers: scheduled("t-late", "0 0 * * *"),
    });
    mocks.listAutomations.mockResolvedValue([later, soon]);

    render(<RunsListPage workspaceId={WORKSPACE_ID} />);

    await vi.waitFor(() => screen.getByTestId("agenda-row-soon"));
    const rows = screen.getAllByTestId(/^agenda-row-/);
    expect(rows[0].getAttribute("data-testid")).toBe("agenda-row-soon");
  });

  it("keeps an automation that will not fire, holding its reason", async () => {
    // Dropping it would make the agenda look complete when it is not.
    mocks.listAutomations.mockResolvedValue([mkAutomation({ enabled: false })]);

    render(<RunsListPage workspaceId={WORKSPACE_ID} />);

    await screen.findByTestId(`agenda-row-${AUTOMATION_ID}`);
    expect(screen.getByTestId("automation-next-run").textContent).toContain("Switched off");
  });

  it("shows what every automation has been saying, without a second click", async () => {
    mocks.listWorkspaceAutomationRuns.mockResolvedValue([mkRun()]);

    render(<RunsListPage workspaceId={WORKSPACE_ID} />);

    await waitFor(() =>
      expect(screen.getByTestId("run-outcome").textContent).toContain("Found 3 drifted specs."),
    );
  });

  it("names the automation on each run, since the feed mixes them", async () => {
    mocks.listWorkspaceAutomationRuns.mockResolvedValue([mkRun()]);

    render(<RunsListPage workspaceId={WORKSPACE_ID} />);

    await waitFor(() => expect(screen.getByTestId("recent-runs").textContent).toContain(NIGHTLY));
  });

  it("opens an automation from the agenda", async () => {
    render(<RunsListPage workspaceId={WORKSPACE_ID} />);
    await screen.findByTestId(`agenda-row-${AUTOMATION_ID}`);

    fireEvent.click(screen.getByTestId(`agenda-row-${AUTOMATION_ID}`));

    expect(mocks.push).toHaveBeenCalledWith(`/automations/${AUTOMATION_ID}`);
  });

  it("opens a run's transcript from the feed", async () => {
    mocks.listWorkspaceAutomationRuns.mockResolvedValue([mkRun()]);

    render(<RunsListPage workspaceId={WORKSPACE_ID} />);
    await waitFor(() => screen.getByTestId("run-entry-run-1"));

    fireEvent.click(screen.getByTestId("run-entry-run-1"));

    expect(mocks.push).toHaveBeenCalledWith("/tasks/task-1");
  });

  it("states the scheduler constraint rather than leaving it to be discovered", async () => {
    render(<RunsListPage workspaceId={WORKSPACE_ID} />);

    await screen.findByTestId("runs-scheduler-note");
    expect(screen.getByTestId("runs-scheduler-note").textContent).toContain(
      "only run while kandev is running",
    );
  });

  it("follows the active workspace, not the one the route was built with", async () => {
    mocks.activeWorkspaceId.current = "workspace-switched";

    render(<RunsListPage workspaceId={WORKSPACE_ID} />);

    await waitFor(() =>
      expect(mocks.listAutomationSummaries).toHaveBeenCalledWith("workspace-switched"),
    );
  });

  it("invites the user to create one when the workspace has no automations", async () => {
    mocks.listAutomations.mockResolvedValue([]);

    render(<RunsListPage workspaceId={WORKSPACE_ID} />);

    await screen.findByTestId("runs-empty-state");
  });
});

describe("RunsListPage live refresh", () => {
  it("keeps a running automation current without the user reloading", async () => {
    vi.useFakeTimers();
    // A scheduled automation with a run open: the row reads as running, which
    // is what gates the polling. Asserted on the dot rather than on the
    // next-run note — one open run against one slot is the ordinary steady
    // state and says nothing about the cap.
    mocks.listAutomations.mockResolvedValue([
      mkAutomation({ triggers: scheduled("t1", "0 0 * * *") }),
    ]);
    mocks.listAutomationSummaries.mockResolvedValue([summary({ open_runs: 1 })]);

    render(<RunsListPage workspaceId={WORKSPACE_ID} />);
    await vi.waitFor(() =>
      expect(
        screen.getByTestId(`agenda-row-${AUTOMATION_ID}`).innerHTML.includes(STATE_DOT_RUNNING),
      ).toBe(true),
    );
    const before = mocks.listAutomationSummaries.mock.calls.length;

    await vi.advanceTimersByTimeAsync(10_000);

    expect(mocks.listAutomationSummaries.mock.calls.length).toBeGreaterThan(before);
  });

  it("makes no repeat requests when nothing is running", async () => {
    vi.useFakeTimers();

    render(<RunsListPage workspaceId={WORKSPACE_ID} />);
    await vi.waitFor(() => expect(mocks.listAutomationSummaries).toHaveBeenCalledTimes(1));

    await vi.advanceTimersByTimeAsync(60_000);

    expect(mocks.listAutomationSummaries).toHaveBeenCalledTimes(1);
  });
});
