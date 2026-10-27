import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import type { WorkloadRunObservation } from "@/lib/types/background-work";
import { BackgroundWorkChip } from "./background-work-chip";
import { BackgroundWorkPanel } from "./background-work-panel";

const addBackgroundWorkPanel = vi.fn();
vi.mock("@/lib/state/dockview-store", () => ({
  useDockviewStore: {
    getState: () => ({
      addBackgroundWorkPanel,
    }),
  },
}));

const WORK_ID = "work-1";
const SESSION_ID = "session-1";
const WORK_TITLE = "Build Watcher";
const HISTORY_TITLE = "Background history";
const CHIP_TEST_ID = "background-work-chip";

let mockWorkloads: WorkloadRunObservation[] = [];
const mockLoading = false;
const executeActionMock = vi.fn().mockResolvedValue({ success: true });

vi.mock("@/hooks/domains/session/use-background-work", () => ({
  useBackgroundWork: () => {
    const active = mockWorkloads.filter((w) =>
      ["running", "waiting", "unknown"].includes(w.state),
    ).length;
    return {
      workloads: mockWorkloads,
      totalCount: active,
      totalJobCount: mockWorkloads.length,
      runningCount: mockWorkloads.filter((w) => w.state === "running").length,
      waitingCount: mockWorkloads.filter((w) => w.state === "waiting").length,
      unknownCount: mockWorkloads.filter((w) => w.state === "unknown").length,
      hasActiveWork: active > 0,
      hasHistory: mockWorkloads.length > 0,
      isLoading: mockLoading,
      executeAction: executeActionMock,
      setActiveWorkload: vi.fn(),
      fetchWorkloads: vi.fn(),
    };
  },
}));

vi.mock("@/components/state-provider", () => ({
  useAppStore: (
    selector: (state: {
      tasks: { activeSessionId: string };
      features: { agentBackgroundWork: boolean };
    }) => unknown,
  ) =>
    selector({ tasks: { activeSessionId: SESSION_ID }, features: { agentBackgroundWork: true } }),
}));

function workload(overrides: Partial<WorkloadRunObservation> = {}): WorkloadRunObservation {
  return {
    work_id: WORK_ID,
    title: WORK_TITLE,
    kind: "shell",
    state: "running",
    capabilities: {
      discovery: "snapshot",
      output: "stream",
      transcript: false,
      parentage: false,
      reasoning_summary: false,
      attributable_usage: false,
      actions: {
        stop: { supported: true, available: true },
        write_input: { supported: true, available: true },
      },
    },
    output: "Listening for changes...",
    revision: 1,
    ...overrides,
  };
}

describe("BackgroundWorkChip", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockWorkloads = [workload()];
  });

  afterEach(() => {
    cleanup();
  });

  it("renders chip when workloads exist and opens popover", async () => {
    render(<BackgroundWorkChip sessionId={SESSION_ID} />);

    const chip = screen.getByTestId(CHIP_TEST_ID);
    expect(chip).toBeTruthy();
    expect(chip.getAttribute("aria-label")).toBe("1 Background Job");

    fireEvent.click(chip);

    const summary = await screen.findByTestId("background-work-summary-list");
    expect(summary.textContent).toContain("Build Watcher");

    const openBtn = screen.getByTestId("background-work-open-button-work-1");
    fireEvent.click(openBtn);

    expect(addBackgroundWorkPanel).toHaveBeenCalledWith({
      sessionId: SESSION_ID,
      workId: WORK_ID,
      title: WORK_TITLE,
      inCenter: true,
    });
  });

  it("hides chip when there are 0 workloads", () => {
    mockWorkloads = [];
    render(<BackgroundWorkChip sessionId={SESSION_ID} />);
    expect(screen.queryByTestId(CHIP_TEST_ID)).toBeNull();
    expect(screen.queryByTestId("background-work-history-chip")).toBeNull();
  });

  it("keeps completed history reachable when the active count reaches zero", async () => {
    mockWorkloads = [workload({ state: "completed" })];
    render(<BackgroundWorkChip sessionId={SESSION_ID} />);

    expect(screen.queryByTestId(CHIP_TEST_ID)).toBeNull();
    const history = screen.getByTestId("background-work-history-chip");
    expect(history.getAttribute("aria-label")).toBe(HISTORY_TITLE);

    fireEvent.click(history);

    const summary = await screen.findByTestId("background-work-summary-list");
    expect(summary.textContent).toContain(HISTORY_TITLE);
    expect(summary.textContent).toContain("Build Watcher");
    expect(summary.textContent).toContain("Completed");
  });

  it("reports separate running, waiting, and unknown counts", async () => {
    mockWorkloads = [
      workload({ work_id: "work-1", state: "running" }),
      workload({ work_id: "work-2", state: "waiting", title: "Subagent" }),
      workload({ work_id: "work-3", state: "unknown", title: "CI poller" }),
    ];
    render(<BackgroundWorkChip sessionId={SESSION_ID} />);
    fireEvent.click(screen.getByTestId(CHIP_TEST_ID));

    const counts = await screen.findByTestId("background-work-summary-counts");
    expect(counts.textContent).toContain("1 running");
    expect(counts.textContent).toContain("1 waiting");
    expect(counts.textContent).toContain("1 unknown");
  });

  it("labels unknown activity distinctly from running", () => {
    mockWorkloads = [workload({ state: "unknown" })];
    render(<BackgroundWorkChip sessionId={SESSION_ID} />);
    fireEvent.click(screen.getByTestId(CHIP_TEST_ID));
    const badge = screen.getByTestId(`background-work-state-${WORK_ID}`);
    expect(badge.textContent).toBe("Unknown");
    expect(badge.getAttribute("data-state")).toBe("unknown");
  });
});

describe("BackgroundWorkPanel", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockWorkloads = [workload()];
  });

  afterEach(() => {
    cleanup();
  });

  it("renders overview when no workId is passed", () => {
    render(<BackgroundWorkPanel panelId="background-work" params={{}} />);

    expect(screen.getByTestId("background-work-overview-panel")).toBeTruthy();
    expect(screen.getByTestId("background-workload-card-work-1")).toBeTruthy();
  });

  it("renders detail view with actions and output when workId is given", async () => {
    render(<BackgroundWorkPanel panelId="background-work:work-1" params={{ workId: WORK_ID }} />);

    expect(screen.getByTestId("background-work-detail-work-1")).toBeTruthy();
    expect(screen.getByText("Listening for changes...")).toBeTruthy();

    const stopBtn = screen.getByTestId("stop-workload-button");
    fireEvent.click(stopBtn);

    expect(executeActionMock).toHaveBeenCalledWith({
      work_id: WORK_ID,
      action: "stop",
    });
  });

  it("hides unsupported controls instead of inferring them", () => {
    mockWorkloads = [
      workload({
        capabilities: {
          discovery: "events_only",
          output: "none",
          transcript: false,
          parentage: false,
          reasoning_summary: false,
          attributable_usage: false,
        },
        output: undefined,
      }),
    ];
    render(<BackgroundWorkPanel panelId="background-work:work-1" params={{ workId: WORK_ID }} />);

    expect(screen.queryByTestId("stop-workload-button")).toBeNull();
    expect(screen.queryByTestId("interrupt-workload-button")).toBeNull();
    expect(screen.getByTestId("background-work-actions-unavailable")).toBeTruthy();
    expect(screen.getByText("Output is not available for this job")).toBeTruthy();
  });

  it("shows the retained exit status for a failed job", () => {
    mockWorkloads = [
      workload({
        state: "failed",
        exit_code: 2,
        output: "checks failed",
        capabilities: {
          discovery: "snapshot",
          output: "snapshot",
          transcript: false,
          parentage: false,
          reasoning_summary: false,
          attributable_usage: false,
          actions: {
            stop: { supported: false, available: false },
          },
        },
      }),
    ];
    render(<BackgroundWorkPanel panelId="background-work:work-1" params={{ workId: WORK_ID }} />);

    expect(screen.getByTestId("background-work-state-work-1").textContent).toBe("Failed");
    expect(screen.getByText("Exit 2")).toBeTruthy();
    expect(screen.getByText("checks failed")).toBeTruthy();
    expect(screen.queryByTestId("stop-workload-button")).toBeNull();
  });

  it("keeps an explicit supported write-input control", () => {
    render(<BackgroundWorkPanel panelId="background-work:work-1" params={{ workId: WORK_ID }} />);
    expect(screen.getByPlaceholderText("Input...")).toBeTruthy();
  });
});
