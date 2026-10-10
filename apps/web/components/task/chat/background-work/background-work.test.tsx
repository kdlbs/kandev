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
const WORK_TITLE = "Build Watcher";

let mockWorkloads: WorkloadRunObservation[] = [];
const mockLoading = false;
const executeActionMock = vi.fn().mockResolvedValue({ success: true });

vi.mock("@/hooks/domains/session/use-background-work", () => ({
  useBackgroundWork: () => ({
    workloads: mockWorkloads,
    totalCount: mockWorkloads.length,
    runningCount: mockWorkloads.filter((w) => w.state === "running").length,
    waitingCount: mockWorkloads.filter((w) => w.state === "waiting").length,
    hasActiveWork: mockWorkloads.length > 0,
    isLoading: mockLoading,
    executeAction: executeActionMock,
    setActiveWorkload: vi.fn(),
    fetchWorkloads: vi.fn(),
  }),
}));

vi.mock("@/components/state-provider", () => ({
  useAppStore: (
    selector: (state: {
      tasks: { activeSessionId: string };
      features: { agentBackgroundWork: boolean };
    }) => unknown,
  ) =>
    selector({ tasks: { activeSessionId: "session-1" }, features: { agentBackgroundWork: true } }),
}));

describe("BackgroundWorkChip", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockWorkloads = [
      {
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
      },
    ];
  });

  afterEach(() => {
    cleanup();
  });

  it("renders chip when workloads exist and opens popover", async () => {
    render(<BackgroundWorkChip sessionId="session-1" />);

    const chip = screen.getByTestId("background-work-chip");
    expect(chip).toBeTruthy();

    fireEvent.click(chip);

    const summary = await screen.findByTestId("background-work-summary-list");
    expect(summary.textContent).toContain("Build Watcher");

    const openBtn = screen.getByTestId("background-work-open-button-work-1");
    fireEvent.click(openBtn);

    expect(addBackgroundWorkPanel).toHaveBeenCalledWith({
      sessionId: "session-1",
      workId: WORK_ID,
      title: WORK_TITLE,
      inCenter: true,
    });
  });

  it("hides chip when there are 0 workloads", () => {
    mockWorkloads = [];
    render(<BackgroundWorkChip sessionId="session-1" />);
    expect(screen.queryByTestId("background-work-chip")).toBeNull();
  });
});

describe("BackgroundWorkPanel", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockWorkloads = [
      {
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
      },
    ];
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
});
