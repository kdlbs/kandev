import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { Automation, AutomationRun } from "@/lib/types/automation";

const mockIsMobile = vi.hoisted(() => ({ value: false }));
vi.mock("@/hooks/use-responsive-breakpoint", () => ({
  useResponsiveBreakpoint: () => ({ isMobile: mockIsMobile.value }),
}));

const AUTOMATION_ID = "automation-1";
const WORKSPACE_ID = "workspace-1";
const ACTIVITY = "automation-activity";
const TRANSCRIPT = "run-transcript";
const SESSION_ATTR = "data-session-id";
const RUN_NOW = "automation-run-now";
const NEXT_RUN = "automation-next-run";
const DETAIL_TOGGLE = "run-detail-toggle";
const PROMPT = "automation-prompt";
const SESSION_OLDER = "session-older";
const SESSION_NEWEST = "session-newest";
const OLDER_AT = "2026-07-29T00:00:00Z";
const NEWEST_AT = "2026-07-30T16:00:00Z";

const mocks = vi.hoisted(() => ({
  getAutomation: vi.fn(),
  getAutomationSummary: vi.fn(),
  listAutomationRuns: vi.fn(),
  triggerAutomation: vi.fn(),
  stopAutomationRun: vi.fn(),
  push: vi.fn(),
  // The page follows the ACTIVE workspace: an automation belonging to another
  // one must not stay on screen after a switch.
  activeWorkspaceId: { current: undefined as string | undefined },
}));

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: { workspaces: { activeId: string | undefined } }) => unknown) =>
    selector({ workspaces: { activeId: mocks.activeWorkspaceId.current } }),
}));

vi.mock("@/lib/api/domains/automation-api", () => ({
  getAutomation: mocks.getAutomation,
  getAutomationSummary: mocks.getAutomationSummary,
  listAutomationRuns: mocks.listAutomationRuns,
  triggerAutomation: mocks.triggerAutomation,
  stopAutomationRun: mocks.stopAutomationRun,
}));

vi.mock("@/lib/routing/client-router", () => ({
  useRouter: () => ({ push: mocks.push }),
  usePathname: () => `/automations/${AUTOMATION_ID}`,
}));

vi.mock("sonner", () => ({
  toast: { info: vi.fn(), success: vi.fn(), error: vi.fn() },
}));

// The editor is a settings-shell component with its own data fetching; this
// page's contract is that it mounts on the Configure tab and nowhere else.
vi.mock("@/components/automations/automation-editor", () => ({
  AutomationEditor: () => <div data-testid="automation-editor" />,
}));

// The transcript is the task chat stack. This suite is about which run the page
// puts in the pane, so it asserts on the session the embed is handed.
vi.mock("./run-transcript", () => ({
  RunTranscript: ({ sessionId, turnId }: { sessionId: string; turnId?: string }) => (
    <div data-testid="run-transcript" data-session-id={sessionId} data-turn-id={turnId ?? ""} />
  ),
}));

vi.mock("@/components/settings/settings-save-provider", () => ({
  SettingsSaveProvider: ({ children }: { children: React.ReactNode }) => <div>{children}</div>,
}));

import { AutomationDetailPage } from "./automation-detail-page";

const AUTOMATION: Automation = {
  id: AUTOMATION_ID,
  workspace_id: WORKSPACE_ID,
  name: "Nightly drift",
  prompt: "Run /ksdd:drift --all and report what moved.",
  description: "",
  enabled: true,
  workflow_id: "",
  workflow_step_id: "",
  agent_profile_id: "",
  executor_profile_id: "",
  repository_id: "",
  max_concurrent_runs: 1,
  triggers: [
    {
      id: "trigger-1",
      automation_id: AUTOMATION_ID,
      trigger_type: "scheduled",
      enabled: true,
      config: { expression: "0 0 * * *", timezone: "Asia/Singapore" },
    },
  ],
  created_at: "2026-07-30T00:00:00Z",
  updated_at: "2026-07-30T00:00:00Z",
} as unknown as Automation;

function run(overrides: Partial<AutomationRun> = {}): AutomationRun {
  return {
    id: "run-1",
    automation_id: AUTOMATION_ID,
    trigger_id: "trigger-1",
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
  } as AutomationRun;
}

beforeEach(() => {
  mocks.activeWorkspaceId.current = WORKSPACE_ID;
  mocks.getAutomation.mockResolvedValue(AUTOMATION);
  mocks.getAutomationSummary.mockResolvedValue({
    automation_id: AUTOMATION_ID,
    open_runs: 0,
  });
  mocks.listAutomationRuns.mockResolvedValue([run()]);
  mocks.triggerAutomation.mockResolvedValue({ triggered: true });
  mocks.stopAutomationRun.mockResolvedValue({ run_id: "run-1", status: "failed" });
});

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
  mockIsMobile.value = false;
});

describe("AutomationDetailPage", () => {
  it("opens on the newest run's conversation, not on a list of runs", async () => {
    // The automation is a thread that recurs, so landing here lands you in what
    // it last said — zero clicks to the thing you came for.
    mocks.listAutomationRuns.mockResolvedValue([
      run({ id: "older", session_id: SESSION_OLDER, created_at: OLDER_AT }),
      run({ id: "newest", session_id: SESSION_NEWEST, created_at: NEWEST_AT }),
    ]);

    render(<AutomationDetailPage automationId={AUTOMATION_ID} tab="activity" />);

    const transcript = await screen.findByTestId(TRANSCRIPT);
    expect(transcript.getAttribute(SESSION_ATTR)).toBe(SESSION_NEWEST);
    // Configuration is behind Details, never the landing view.
    expect(screen.queryByTestId("automation-editor")).toBeNull();
  });

  // The rail is a permanent column resized by dragging its edge — a mouse
  // gesture, and a large share of a phone viewport. On mobile the same runs
  // move behind a control so the transcript and composer, which are the point
  // of the page, get the screen.
  it("puts the runs in a drawer on mobile, and a rail everywhere else", async () => {
    render(<AutomationDetailPage automationId={AUTOMATION_ID} tab="activity" />);
    await screen.findByTestId(TRANSCRIPT);
    expect(screen.getByTestId("runs-rail")).toBeTruthy();
    expect(screen.queryByTestId("runs-drawer-trigger")).toBeNull();

    cleanup();
    mockIsMobile.value = true;

    render(<AutomationDetailPage automationId={AUTOMATION_ID} tab="activity" />);
    await screen.findByTestId(TRANSCRIPT);
    expect(screen.getByTestId("runs-drawer-trigger")).toBeTruthy();
    expect(screen.queryByTestId("runs-rail")).toBeNull();
  });

  it("puts nothing above the transcript — the conversation is what the page is for", async () => {
    // The standing instruction is the same text on every run and it is long, so
    // pinning it here pushed what the agent actually said down the page on
    // every visit.
    render(<AutomationDetailPage automationId={AUTOMATION_ID} tab="activity" />);

    await screen.findByTestId(TRANSCRIPT);
    expect(screen.queryByTestId(PROMPT)).toBeNull();
  });

  it("keeps the standing instruction one click away in the rail", async () => {
    render(<AutomationDetailPage automationId={AUTOMATION_ID} tab="activity" />);

    const toggle = await screen.findByTestId(DETAIL_TOGGLE);
    expect(screen.queryByTestId("run-detail-panel")).toBeNull();

    fireEvent.click(toggle);

    expect(screen.getByTestId(PROMPT).textContent).toContain("/ksdd:drift");
  });

  it("offers the same detail from the drawer on mobile", async () => {
    mockIsMobile.value = true;

    render(<AutomationDetailPage automationId={AUTOMATION_ID} tab="activity" />);
    await screen.findByTestId(TRANSCRIPT);

    fireEvent.click(screen.getByTestId("runs-drawer-trigger"));

    fireEvent.click(await screen.findByTestId(DETAIL_TOGGLE));
    expect(screen.getByTestId(PROMPT)).toBeTruthy();
  });
});

describe("AutomationDetailPage run selection", () => {
  it("honours an explicitly requested run", async () => {
    // "The run that failed overnight" is a thing people link each other to.
    mocks.listAutomationRuns.mockResolvedValue([
      run({ id: "older", session_id: SESSION_OLDER, created_at: OLDER_AT }),
      run({ id: "newest", session_id: SESSION_NEWEST, created_at: NEWEST_AT }),
    ]);

    render(<AutomationDetailPage automationId={AUTOMATION_ID} tab="activity" runId="older" />);

    const transcript = await screen.findByTestId(TRANSCRIPT);
    expect(transcript.getAttribute(SESSION_ATTR)).toBe(SESSION_OLDER);
  });

  it("focuses the exact turn when two runs share a session", async () => {
    mocks.listAutomationRuns.mockResolvedValue([
      run({ id: "first", turn_id: "turn-1", session_id: "shared-session" }),
      run({
        id: "second",
        turn_id: "turn-2",
        session_id: "shared-session",
        created_at: NEWEST_AT,
      }),
    ]);

    render(<AutomationDetailPage automationId={AUTOMATION_ID} tab="activity" runId="second" />);

    const transcript = await screen.findByTestId(TRANSCRIPT);
    expect(transcript.getAttribute("data-session-id")).toBe("shared-session");
    expect(transcript.getAttribute("data-turn-id")).toBe("turn-2");
  });

  it("offers exact-run stop for a selected open run", async () => {
    mocks.listAutomationRuns.mockResolvedValue([
      run({ id: "open-run", status: "task_created", turn_id: "turn-open" }),
    ]);

    render(<AutomationDetailPage automationId={AUTOMATION_ID} tab="activity" />);

    const stop = await screen.findByTestId("automation-stop-run");
    fireEvent.click(stop);

    await waitFor(() =>
      expect(mocks.stopAutomationRun).toHaveBeenCalledWith(AUTOMATION_ID, "open-run"),
    );
  });

  it("falls back to the newest run when the requested one is not in the window", async () => {
    render(<AutomationDetailPage automationId={AUTOMATION_ID} tab="activity" runId="long-gone" />);

    const transcript = await screen.findByTestId(TRANSCRIPT);
    expect(transcript.getAttribute(SESSION_ATTR)).toBe("session-1");
  });

  it("skips runs that never produced a conversation", async () => {
    // A skipped firing has nothing to read; selecting it would render an empty
    // pane that looks like a broken link.
    mocks.listAutomationRuns.mockResolvedValue([
      run({ id: "skipped", session_id: "", status: "skipped", created_at: "2026-07-31T00:00:00Z" }),
      run({ id: "real", session_id: "session-real", created_at: "2026-07-30T00:00:00Z" }),
    ]);

    render(<AutomationDetailPage automationId={AUTOMATION_ID} tab="activity" />);

    const transcript = await screen.findByTestId(TRANSCRIPT);
    expect(transcript.getAttribute(SESSION_ATTR)).toBe("session-real");
  });

  it("mounts the editor only when Details asks for it", async () => {
    render(<AutomationDetailPage automationId={AUTOMATION_ID} tab="configure" />);

    await waitFor(() => screen.getByTestId("automation-editor"));
    expect(screen.queryByTestId(ACTIVITY)).toBeNull();
  });

  it("keeps the rail alongside configuration, so runs stay one click away", async () => {
    render(<AutomationDetailPage automationId={AUTOMATION_ID} tab="configure" />);

    await waitFor(() => screen.getByTestId("runs-rail"));
  });

  it("fires the automation on demand and refreshes so the run appears", async () => {
    // Waiting until tomorrow to learn whether a schedule works is the whole
    // reason this button is on the reading surface.
    render(<AutomationDetailPage automationId={AUTOMATION_ID} tab="activity" />);
    await waitFor(() => expect(mocks.listAutomationRuns).toHaveBeenCalledTimes(1));

    fireEvent.click(screen.getByTestId(RUN_NOW));

    await waitFor(() => expect(mocks.triggerAutomation).toHaveBeenCalledWith(AUTOMATION_ID));
    await waitFor(() => expect(mocks.listAutomationRuns).toHaveBeenCalledTimes(2));
  });

  it("keeps Run now inert until the automation it would fire has loaded", () => {
    render(<AutomationDetailPage automationId={AUTOMATION_ID} tab="activity" />);

    expect(screen.getByTestId(RUN_NOW).hasAttribute("disabled")).toBe(true);
  });
});

describe("AutomationDetailPage run switching", () => {
  it("switches runs from the rail, putting the selection in the URL", async () => {
    mocks.listAutomationRuns.mockResolvedValue([
      run({ id: "older", session_id: SESSION_OLDER, created_at: OLDER_AT }),
      run({ id: "newest", session_id: SESSION_NEWEST, created_at: NEWEST_AT }),
    ]);

    render(<AutomationDetailPage automationId={AUTOMATION_ID} tab="activity" />);
    await waitFor(() => screen.getByTestId("run-row-older"));

    fireEvent.click(screen.getByTestId("run-row-older"));

    expect(mocks.push).toHaveBeenCalledWith(`/automations/${AUTOMATION_ID}?run=older`);
  });

  it("lets the rail be resized, the way the sidebar edge can be", async () => {
    // Run rows carry a timestamp and a status word; how much room the reader
    // wants depends on whether they are scanning or reading beside it.
    render(<AutomationDetailPage automationId={AUTOMATION_ID} tab="activity" />);

    const handle = await screen.findByTestId("runs-rail-resize");
    expect(handle.getAttribute("aria-label")).toBe("Resize runs");
  });

  it("offers configuration from the rail rather than as a tab beside the transcript", async () => {
    render(<AutomationDetailPage automationId={AUTOMATION_ID} tab="activity" />);

    const details = await screen.findByTestId("automation-details-link");
    expect(details.getAttribute("href")).toBe(`/automations/${AUTOMATION_ID}?tab=configure`);
  });

  it("leaves for the list when the active workspace no longer owns this automation", async () => {
    // Switching workspaces with this page open would otherwise leave the
    // previous workspace's automation on screen under a sidebar that says the
    // user is somewhere else.
    mocks.activeWorkspaceId.current = "workspace-other";

    render(<AutomationDetailPage automationId={AUTOMATION_ID} tab="activity" />);

    await waitFor(() => expect(mocks.push).toHaveBeenCalledWith("/automations"));
    expect(screen.queryByTestId(ACTIVITY)).toBeNull();
    expect(screen.queryByTestId("automation-schedule")).toBeNull();
  });

  it("keeps polling for an open run older than its own history window", async () => {
    // The window is capped, so counting open runs from it would report nothing
    // in flight once enough newer entries pile up in front of the open one.
    vi.useFakeTimers();
    mocks.getAutomationSummary.mockResolvedValue({
      automation_id: AUTOMATION_ID,
      open_runs: 1,
    });
    mocks.listAutomationRuns.mockResolvedValue([run({ id: "run-done", status: "succeeded" })]);

    render(<AutomationDetailPage automationId={AUTOMATION_ID} tab="activity" />);
    // Wait for the load to land, not just for the request: polling is gated on
    // the applied open count.
    await vi.waitFor(() => screen.getByTestId(TRANSCRIPT));
    const before = mocks.getAutomationSummary.mock.calls.length;

    await vi.advanceTimersByTimeAsync(10_000);

    expect(mocks.getAutomationSummary.mock.calls.length).toBeGreaterThan(before);
    vi.useRealTimers();
  });

  it("makes no repeat requests once nothing is open", async () => {
    vi.useFakeTimers();

    render(<AutomationDetailPage automationId={AUTOMATION_ID} tab="activity" />);
    await vi.waitFor(() => screen.getByTestId(TRANSCRIPT));

    await vi.advanceTimersByTimeAsync(60_000);

    expect(mocks.getAutomationSummary).toHaveBeenCalledTimes(1);
    vi.useRealTimers();
  });

  it("says so and offers a retry when the automation cannot be loaded", async () => {
    mocks.getAutomation.mockRejectedValue(new Error("automation not found"));

    render(<AutomationDetailPage automationId={AUTOMATION_ID} tab="activity" />);

    await waitFor(() => screen.getByTestId("automation-error"));
    expect(screen.getByTestId("automation-error").textContent).toContain("automation not found");
    // A blank Activity list under an error would read as "it has never run".
    expect(screen.queryByTestId(ACTIVITY)).toBeNull();
  });
});

/**
 * The page has to keep itself current across a manual fire — the run row is
 * written after the fire returns, so anything it renders from a pre-trigger
 * snapshot is stale the moment the run exists.
 */
describe("AutomationDetailPage freshness", () => {
  it("clears the paused note once the triggered run settles", async () => {
    // The reported bug: the amber note appeared when a run was fired and stayed
    // there. It has to come down on its own, from the page's own polling, with
    // nobody pressing Refresh.
    vi.useFakeTimers();
    mocks.getAutomation.mockResolvedValue({
      ...AUTOMATION,
      max_concurrent_runs: 2,
      // A real scheduled trigger: without one the note reads "No schedule",
      // which would pass a "not Paused" assertion for the wrong reason.
      triggers: [
        {
          id: "trigger-1",
          automation_id: AUTOMATION_ID,
          type: "scheduled",
          enabled: true,
          config: { cron_expression: "0 0 * * *", timezone: "Asia/Singapore" },
        },
      ],
    });
    mocks.getAutomationSummary.mockResolvedValue({ automation_id: AUTOMATION_ID, open_runs: 2 });

    render(<AutomationDetailPage automationId={AUTOMATION_ID} tab="activity" />);
    await vi.waitFor(() => expect(screen.getByTestId(NEXT_RUN).textContent).toContain("Paused"));

    mocks.getAutomationSummary.mockResolvedValue({ automation_id: AUTOMATION_ID, open_runs: 0 });
    await vi.advanceTimersByTimeAsync(10_000);

    await vi.waitFor(() =>
      expect(screen.getByTestId(NEXT_RUN).textContent).not.toContain("Paused"),
    );
    vi.useRealTimers();
  });

  it("keeps re-asking after a manual fire even before the run row exists", async () => {
    // The run row is written on the orchestrator's own goroutine after the fire
    // returns. A page that only polls while it can already see an open run
    // would stop asking here, and then render its pre-trigger snapshot — and
    // everything derived from it — until someone reloaded.
    vi.useFakeTimers();

    render(<AutomationDetailPage automationId={AUTOMATION_ID} tab="activity" />);
    await vi.waitFor(() => screen.getByTestId(TRANSCRIPT));

    fireEvent.click(screen.getByTestId(RUN_NOW));
    await vi.advanceTimersByTimeAsync(4_000);
    const afterRetryBurst = mocks.getAutomationSummary.mock.calls.length;

    await vi.advanceTimersByTimeAsync(20_000);

    expect(mocks.getAutomationSummary.mock.calls.length).toBeGreaterThan(afterRetryBurst);
    vi.useRealTimers();
  });

  it("stops re-asking once the settling window closes with nothing open", async () => {
    // The window is a bounded hedge against a slow write, not a licence to poll
    // an idle automation forever.
    vi.useFakeTimers();

    render(<AutomationDetailPage automationId={AUTOMATION_ID} tab="activity" />);
    await vi.waitFor(() => screen.getByTestId(TRANSCRIPT));

    fireEvent.click(screen.getByTestId(RUN_NOW));
    await vi.advanceTimersByTimeAsync(70_000);
    const afterWindow = mocks.getAutomationSummary.mock.calls.length;

    await vi.advanceTimersByTimeAsync(60_000);

    expect(mocks.getAutomationSummary.mock.calls.length).toBe(afterWindow);
    vi.useRealTimers();
  });
});

describe("AutomationDetailPage visible run freshness", () => {
  it("keeps polling when a visible running row has no open summary count", async () => {
    vi.useFakeTimers();
    mocks.listAutomationRuns
      .mockResolvedValueOnce([run({ status: "task_created" })])
      .mockResolvedValue([run({ status: "succeeded" })]);
    mocks.getAutomationSummary.mockResolvedValue({
      automation_id: AUTOMATION_ID,
      open_runs: 0,
    });

    try {
      render(<AutomationDetailPage automationId={AUTOMATION_ID} tab="activity" />);
      await vi.waitFor(() => expect(screen.getByTestId("run-group-running")).toBeTruthy());

      await vi.advanceTimersByTimeAsync(10_000);

      await vi.waitFor(() => expect(screen.getByTestId("run-group-completed")).toBeTruthy());
      expect(screen.queryByTestId("run-group-running")).toBeNull();
    } finally {
      vi.useRealTimers();
    }
  });
});
