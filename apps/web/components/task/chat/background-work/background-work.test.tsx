import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import type { WorkloadRunObservation } from "@/lib/types/background-work";
import { BackgroundWorkChip } from "./background-work-chip";
import { BackgroundWorkMobilePanel } from "./background-work-mobile-surface";
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
const CHIP_TEST_ID = "background-work-chip";
const SUMMARY_TEST_ID = "background-work-summary-list";
const TAB_ACTIVE_TEST_ID = "background-work-tab-active";
const TAB_HISTORY_TEST_ID = "background-work-tab-history";
const MOBILE_OVERVIEW_TEST_ID = "background-work-mobile-overview";
const RUNNING_TITLE = "Running work (1)";
const WAITING_TITLE = "Waiting subagent";
const UNKNOWN_TITLE = "Unknown poller";
const CARD_WORK_1_TEST_ID = "background-workload-card-work-1";
const DETAIL_COMMAND_TEST_ID = "background-work-detail-command";
const SUMMARY_OPEN_BUTTON_TEST_ID = "background-work-open-button-work-1";
const DRAWER_DETAIL_TEST_ID = "background-work-drawer-detail";
const STOP_BUTTON_TEST_ID = "stop-workload-button";
const INTERRUPT_BUTTON_TEST_ID = "interrupt-workload-button";
const ACTIVE_TAB_STATE = "active";
const DATA_STATE_ATTR = "data-state";

let mockWorkloads: WorkloadRunObservation[] = [];
const mockLoading = false;
const executeActionMock = vi.fn().mockResolvedValue({ success: true });

vi.mock("@/hooks/domains/session/use-background-work", () => ({
  useBackgroundWork: () => {
    const active = mockWorkloads.filter((w) => ["running", "waiting"].includes(w.state)).length;
    return {
      workloads: mockWorkloads,
      runningWorkloads: mockWorkloads.filter((w) => w.state === "running"),
      activeWorkloads: mockWorkloads.filter((w) => ["running", "waiting"].includes(w.state)),
      historyWorkloads: mockWorkloads.filter((w) => !["running", "waiting"].includes(w.state)),
      totalCount: active,
      totalJobCount: mockWorkloads.length,
      runningCount: mockWorkloads.filter((w) => w.state === "running").length,
      waitingCount: mockWorkloads.filter((w) => w.state === "waiting").length,
      unknownCount: mockWorkloads.filter((w) => w.state === "unknown").length,
      hasActiveWork: active > 0,
      hasHistory: mockWorkloads.length > 0,
      isLoading: mockLoading,
      isInitialLoading: mockLoading,
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

  it("renders a running-only chip and opens the running summary", async () => {
    mockWorkloads = [
      workload({ work_id: WORK_ID, state: "running" }),
      workload({ work_id: "work-2", state: "completed", title: "Finished job" }),
    ];
    render(<BackgroundWorkChip sessionId={SESSION_ID} />);

    const chip = screen.getByTestId(CHIP_TEST_ID);
    expect(chip.getAttribute("aria-label")).toBe("1 Background Job");
    expect(chip.getAttribute("aria-controls")).toBeTruthy();

    fireEvent.click(chip);

    const summary = await screen.findByTestId(SUMMARY_TEST_ID);
    expect(summary.textContent).toContain(RUNNING_TITLE);
    expect(summary.textContent).toContain(WORK_TITLE);
    expect(summary.textContent).not.toContain("Finished job");
    expect(screen.queryByTestId("background-work-summary-counts")).toBeNull();
    expect(screen.getByTestId(SUMMARY_OPEN_BUTTON_TEST_ID).getAttribute("aria-label")).toContain(
      WORK_TITLE,
    );

    fireEvent.click(screen.getByTestId(SUMMARY_OPEN_BUTTON_TEST_ID));
    expect(addBackgroundWorkPanel).toHaveBeenCalledWith({
      sessionId: SESSION_ID,
      workId: WORK_ID,
      title: WORK_TITLE,
      inCenter: true,
    });
  });

  it("hides the chip when there are no running jobs", () => {
    mockWorkloads = [workload({ state: "completed" })];
    render(<BackgroundWorkChip sessionId={SESSION_ID} />);
    expect(screen.queryByTestId(CHIP_TEST_ID)).toBeNull();
    expect(screen.queryByTestId("background-work-history-chip")).toBeNull();
  });

  it.each(["waiting", "unknown", "completed", "failed", "interrupted", "ended"] as const)(
    "hides the chip when only %s work remains",
    (state) => {
      mockWorkloads = [workload({ state })];
      render(<BackgroundWorkChip sessionId={SESSION_ID} />);
      expect(screen.queryByTestId(CHIP_TEST_ID)).toBeNull();
    },
  );

  it("keeps waiting and unknown records out of the running summary", async () => {
    mockWorkloads = [
      workload({ work_id: WORK_ID, state: "running" }),
      workload({ work_id: "work-2", state: "waiting", title: WAITING_TITLE }),
      workload({ work_id: "work-3", state: "unknown", title: UNKNOWN_TITLE }),
    ];
    render(<BackgroundWorkChip sessionId={SESSION_ID} />);
    fireEvent.click(screen.getByTestId(CHIP_TEST_ID));

    const summary = await screen.findByTestId(SUMMARY_TEST_ID);
    expect(summary.textContent).toContain(RUNNING_TITLE);
    expect(summary.textContent).not.toContain(WAITING_TITLE);
    expect(summary.textContent).not.toContain(UNKNOWN_TITLE);
    expect(screen.queryByTestId("background-work-state-work-2")).toBeNull();
    expect(screen.queryByTestId("background-work-state-work-3")).toBeNull();
  });

  it("closes the open summary and focuses the composer when the last running job leaves", async () => {
    const onRequestComposerFocus = vi.fn();
    const { rerender } = render(
      <BackgroundWorkChip sessionId={SESSION_ID} onRequestComposerFocus={onRequestComposerFocus} />,
    );
    fireEvent.click(screen.getByTestId(CHIP_TEST_ID));
    expect(await screen.findByTestId(SUMMARY_TEST_ID)).toBeTruthy();

    mockWorkloads = [workload({ state: "completed" })];
    rerender(
      <BackgroundWorkChip sessionId={SESSION_ID} onRequestComposerFocus={onRequestComposerFocus} />,
    );

    expect(screen.queryByTestId(CHIP_TEST_ID)).toBeNull();
    expect(screen.queryByTestId(SUMMARY_TEST_ID)).toBeNull();
    expect(onRequestComposerFocus).toHaveBeenCalled();
  });

  it("opens the overview from View all without history rows in the summary", async () => {
    mockWorkloads = [workload({ state: "running" })];
    render(<BackgroundWorkChip sessionId={SESSION_ID} />);
    fireEvent.click(screen.getByTestId(CHIP_TEST_ID));
    fireEvent.click(await screen.findByTestId("background-work-view-all-button"));

    expect(addBackgroundWorkPanel).toHaveBeenCalledWith({
      sessionId: SESSION_ID,
      inCenter: true,
    });
  });
});

describe("BackgroundWorkChip phone details", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  afterEach(() => {
    cleanup();
  });

  it("lands on History when the inspected detail finishes while another job keeps running", async () => {
    const previousWidth = window.innerWidth;
    Object.defineProperty(window, "innerWidth", { configurable: true, value: 390 });
    try {
      mockWorkloads = [
        workload({ work_id: WORK_ID, state: "running" }),
        workload({ work_id: "work-2", state: "running", title: "Second job" }),
      ];
      const { rerender } = render(<BackgroundWorkChip sessionId={SESSION_ID} />);

      fireEvent.click(screen.getByTestId(CHIP_TEST_ID));
      fireEvent.click(await screen.findByTestId(SUMMARY_OPEN_BUTTON_TEST_ID));
      expect(await screen.findByTestId(DRAWER_DETAIL_TEST_ID)).toBeTruthy();

      mockWorkloads = [
        workload({ work_id: WORK_ID, state: "completed" }),
        workload({ work_id: "work-2", state: "running", title: "Second job" }),
      ];
      rerender(<BackgroundWorkChip sessionId={SESSION_ID} />);
      fireEvent.click(screen.getByTestId("background-work-drawer-back"));

      await waitFor(() => {
        expect(screen.getByTestId(TAB_HISTORY_TEST_ID).getAttribute(DATA_STATE_ATTR)).toBe(
          ACTIVE_TAB_STATE,
        );
      });
      expect(screen.getByTestId("background-workload-card-work-1")).toBeTruthy();
      await waitFor(() => {
        expect(screen.getByTestId("open-workload-work-1")).toBe(document.activeElement);
      });
    } finally {
      cleanup();
      Object.defineProperty(window, "innerWidth", { configurable: true, value: previousWidth });
    }
  });

  it("falls back to History and restores row focus after a summary detail finishes", async () => {
    const previousWidth = window.innerWidth;
    Object.defineProperty(window, "innerWidth", { configurable: true, value: 390 });
    try {
      mockWorkloads = [workload({ work_id: WORK_ID, state: "running" })];
      const { rerender } = render(<BackgroundWorkChip sessionId={SESSION_ID} />);

      fireEvent.click(screen.getByTestId(CHIP_TEST_ID));
      fireEvent.click(await screen.findByTestId(SUMMARY_OPEN_BUTTON_TEST_ID));
      expect(await screen.findByTestId(DRAWER_DETAIL_TEST_ID)).toBeTruthy();

      mockWorkloads = [workload({ work_id: WORK_ID, state: "completed" })];
      rerender(<BackgroundWorkChip sessionId={SESSION_ID} />);
      expect(screen.getByTestId(DRAWER_DETAIL_TEST_ID)).toBeTruthy();

      fireEvent.click(screen.getByTestId("background-work-drawer-back"));

      await waitFor(() => {
        expect(screen.getByTestId(TAB_HISTORY_TEST_ID).getAttribute(DATA_STATE_ATTR)).toBe(
          ACTIVE_TAB_STATE,
        );
      });
      const historyCard = screen.getByTestId("background-workload-card-work-1");
      expect(historyCard).toBeTruthy();
      await waitFor(() => {
        expect(screen.getByTestId("open-workload-work-1")).toBe(document.activeElement);
      });
    } finally {
      cleanup();
      Object.defineProperty(window, "innerWidth", { configurable: true, value: previousWidth });
    }
  });
});
describe("BackgroundWorkChip phone overview", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  afterEach(() => {
    cleanup();
  });

  it("keeps the phone summary and full-height overview on the shared tabs", async () => {
    const previousWidth = window.innerWidth;
    Object.defineProperty(window, "innerWidth", { configurable: true, value: 390 });
    try {
      mockWorkloads = [
        workload({ work_id: WORK_ID, state: "running" }),
        workload({ work_id: "work-2", state: "waiting", title: WAITING_TITLE }),
        workload({ work_id: "work-3", state: "unknown", title: UNKNOWN_TITLE }),
      ];
      render(<BackgroundWorkChip sessionId={SESSION_ID} />);

      fireEvent.click(screen.getByTestId(CHIP_TEST_ID));
      const summary = await screen.findByTestId(SUMMARY_TEST_ID);
      expect(summary.textContent).toContain(RUNNING_TITLE);
      expect(summary.textContent).not.toContain(WAITING_TITLE);

      fireEvent.click(screen.getByTestId("background-work-view-all-button"));
      expect(await screen.findByTestId(MOBILE_OVERVIEW_TEST_ID)).toBeTruthy();
      expect(screen.getByTestId(TAB_ACTIVE_TEST_ID).textContent).toBe("Active (2)");
      expect(screen.getByTestId(TAB_HISTORY_TEST_ID).textContent).toBe("History (1)");

      fireEvent.mouseDown(screen.getByTestId(TAB_HISTORY_TEST_ID));
      fireEvent.click(screen.getByTestId("open-workload-work-3"));
      const detail = await screen.findByTestId(DRAWER_DETAIL_TEST_ID);
      expect(detail).toBeTruthy();
      expect(screen.getByTestId(DETAIL_COMMAND_TEST_ID).textContent).toContain(UNKNOWN_TITLE);

      fireEvent.click(screen.getByTestId("background-work-drawer-back"));
      expect(screen.getByTestId(MOBILE_OVERVIEW_TEST_ID)).toBeTruthy();
      expect(screen.getByTestId(TAB_HISTORY_TEST_ID).getAttribute(DATA_STATE_ATTR)).toBe(
        ACTIVE_TAB_STATE,
      );

      fireEvent.click(screen.getByTestId("background-work-overview-close"));
      expect(
        screen.getByTestId("background-work-drawer-content").getAttribute(DATA_STATE_ATTR),
      ).not.toBe("open");
    } finally {
      cleanup();
      Object.defineProperty(window, "innerWidth", { configurable: true, value: previousWidth });
    }
  });
});

describe("BackgroundWorkChip phone actions", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  afterEach(() => {
    cleanup();
  });

  it("offers capability-gated stop and interrupt actions in the phone detail", async () => {
    const previousWidth = window.innerWidth;
    Object.defineProperty(window, "innerWidth", { configurable: true, value: 390 });
    try {
      mockWorkloads = [
        workload({
          work_id: WORK_ID,
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
              interrupt: { supported: false, available: false },
            },
          },
        }),
      ];
      render(<BackgroundWorkChip sessionId={SESSION_ID} />);

      fireEvent.click(screen.getByTestId(CHIP_TEST_ID));
      fireEvent.click(await screen.findByTestId(SUMMARY_OPEN_BUTTON_TEST_ID));
      expect(await screen.findByTestId(DRAWER_DETAIL_TEST_ID)).toBeTruthy();
      expect(screen.queryByTestId(INTERRUPT_BUTTON_TEST_ID)).toBeNull();
      fireEvent.click(screen.getByTestId(STOP_BUTTON_TEST_ID));
      expect(executeActionMock).toHaveBeenCalledWith({
        work_id: WORK_ID,
        action: "stop",
      });
    } finally {
      cleanup();
      Object.defineProperty(window, "innerWidth", { configurable: true, value: previousWidth });
    }
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
    expect(screen.getByTestId(CARD_WORK_1_TEST_ID)).toBeTruthy();
  });

  it("renders detail view with actions and output when workId is given", async () => {
    render(<BackgroundWorkPanel panelId="background-work:work-1" params={{ workId: WORK_ID }} />);

    expect(screen.getByTestId("background-work-detail-work-1")).toBeTruthy();
    expect(screen.getByText("Listening for changes...")).toBeTruthy();

    const stopBtn = screen.getByTestId(STOP_BUTTON_TEST_ID);
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

    expect(screen.queryByTestId(STOP_BUTTON_TEST_ID)).toBeNull();
    expect(screen.queryByTestId(INTERRUPT_BUTTON_TEST_ID)).toBeNull();
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
    expect(screen.queryByTestId(STOP_BUTTON_TEST_ID)).toBeNull();
  });

  it("keeps an explicit supported write-input control", () => {
    render(<BackgroundWorkPanel panelId="background-work:work-1" params={{ workId: WORK_ID }} />);
    expect(screen.getByPlaceholderText("Input...")).toBeTruthy();
  });
});

describe("BackgroundWorkPanel overview", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockWorkloads = [workload()];
  });

  afterEach(() => {
    cleanup();
  });

  it("defaults the overview to Active and separates History by state", () => {
    mockWorkloads = [
      workload({ work_id: WORK_ID, state: "running" }),
      workload({ work_id: "work-2", state: "waiting", title: "Waiting agent", kind: "subagent" }),
      workload({ work_id: "work-3", state: "unknown", title: UNKNOWN_TITLE }),
      workload({ work_id: "work-4", state: "completed", title: "Finished job" }),
    ];
    render(<BackgroundWorkPanel panelId="background-work" params={{}} />);

    expect(screen.getByTestId(TAB_ACTIVE_TEST_ID).textContent).toBe("Active (2)");
    expect(screen.getByTestId(TAB_HISTORY_TEST_ID).textContent).toBe("History (2)");
    expect(screen.getByTestId(CARD_WORK_1_TEST_ID)).toBeTruthy();
    expect(screen.getByTestId("background-workload-card-work-2")).toBeTruthy();
    expect(screen.queryByTestId("background-workload-card-work-3")).toBeNull();
    expect(screen.queryByTestId("background-workload-card-work-4")).toBeNull();

    fireEvent.mouseDown(screen.getByTestId(TAB_HISTORY_TEST_ID));
    expect(screen.getByTestId("background-workload-card-work-4")).toBeTruthy();
    expect(screen.getByTestId("background-work-state-work-3").textContent).toBe("Unknown");
    expect(screen.getByTestId("background-work-state-work-4").textContent).toBe("Completed");
  });

  it("opens History first when no active work exists", () => {
    mockWorkloads = [
      workload({ work_id: WORK_ID, state: "unknown", title: UNKNOWN_TITLE }),
      workload({ work_id: "work-2", state: "failed", title: "Failed job" }),
    ];
    render(<BackgroundWorkPanel panelId="background-work" params={{}} />);

    expect(screen.getByTestId(TAB_HISTORY_TEST_ID).getAttribute(DATA_STATE_ATTR)).toBe(
      ACTIVE_TAB_STATE,
    );
    expect(screen.getByTestId(CARD_WORK_1_TEST_ID)).toBeTruthy();
    expect(screen.getByTestId("background-workload-card-work-2")).toBeTruthy();
  });

  it("searches the selected tab and keeps counts independent from the query", () => {
    mockWorkloads = [
      workload({ work_id: WORK_ID, state: "running", title: "Build watcher" }),
      workload({ work_id: "work-2", state: "running", title: "Review auth", kind: "subagent" }),
      workload({ work_id: "work-3", state: "completed", title: "Build once more" }),
    ];
    render(<BackgroundWorkPanel panelId="background-work" params={{}} />);

    fireEvent.change(screen.getByTestId("background-work-search"), {
      target: { value: "review" },
    });

    expect(screen.getByTestId(TAB_ACTIVE_TEST_ID).textContent).toBe("Active (2)");
    expect(screen.getByTestId(TAB_HISTORY_TEST_ID).textContent).toBe("History (1)");
    expect(screen.getByTestId("background-workload-card-work-2")).toBeTruthy();
    expect(screen.queryByTestId(CARD_WORK_1_TEST_ID)).toBeNull();

    fireEvent.change(screen.getByTestId("background-work-search"), {
      target: { value: "missing" },
    });
    expect(screen.getByTestId("background-work-empty").textContent).toBe(
      "No matching background work.",
    );
  });

  it("keeps the selected tab after live membership changes", () => {
    mockWorkloads = [workload({ work_id: WORK_ID, state: "running" })];
    const { rerender } = render(<BackgroundWorkPanel panelId="background-work" params={{}} />);
    fireEvent.mouseDown(screen.getByTestId(TAB_HISTORY_TEST_ID));
    expect(screen.getByTestId("background-work-empty").textContent).toBe(
      "No background work history.",
    );

    mockWorkloads = [workload({ work_id: WORK_ID, state: "completed" })];
    rerender(<BackgroundWorkPanel panelId="background-work" params={{}} />);

    expect(screen.getByTestId(TAB_HISTORY_TEST_ID).getAttribute(DATA_STATE_ATTR)).toBe(
      ACTIVE_TAB_STATE,
    );
    expect(screen.getByTestId(CARD_WORK_1_TEST_ID)).toBeTruthy();
  });

  it("shows the complete command in the detail surface", () => {
    const command = "npm run build --watch --filter=@kandev/a-very-long-unbroken-package-name";
    mockWorkloads = [workload({ title: command, state: "running" })];
    render(<BackgroundWorkPanel panelId="background-work:work-1" params={{ workId: WORK_ID }} />);

    const detailCommand = screen.getByTestId(DETAIL_COMMAND_TEST_ID);
    expect(detailCommand.textContent).toContain(command);
    expect(detailCommand.className).toContain("[overflow-wrap:anywhere]");
    const scroll = screen.getByTestId("background-work-detail-scroll");
    expect(scroll.contains(detailCommand)).toBe(true);
    expect(screen.getByTestId("background-work-detail-header").contains(detailCommand)).toBe(false);
  });
});

describe("BackgroundWorkPanel tab semantics", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockWorkloads = [
      workload({ work_id: WORK_ID, state: "running" }),
      workload({ work_id: "work-2", state: "completed" }),
    ];
  });

  afterEach(() => {
    cleanup();
  });

  it("links each tab to its panel and supports keyboard navigation", async () => {
    mockWorkloads = [
      workload({ work_id: WORK_ID, state: "running" }),
      workload({ work_id: "work-2", state: "completed" }),
    ];
    render(<BackgroundWorkPanel panelId="background-work" params={{}} />);

    const activeTab = screen.getByTestId(TAB_ACTIVE_TEST_ID);
    const activePanelId = activeTab.getAttribute("aria-controls");
    expect(activePanelId).toBeTruthy();
    const activePanel = document.getElementById(activePanelId!);
    expect(activePanel?.getAttribute("role")).toBe("tabpanel");
    expect(activePanel?.getAttribute("aria-labelledby")).toBe(activeTab.id);

    activeTab.focus();
    fireEvent.keyDown(activeTab, { key: "ArrowRight" });

    const historyTab = screen.getByTestId(TAB_HISTORY_TEST_ID);
    await waitFor(() => {
      expect(historyTab.getAttribute("data-state")).toBe(ACTIVE_TAB_STATE);
    });
    const historyPanelId = historyTab.getAttribute("aria-controls");
    expect(historyPanelId).toBeTruthy();
    const historyPanel = document.getElementById(historyPanelId!);
    expect(historyPanel?.getAttribute("role")).toBe("tabpanel");
    expect(historyPanel?.getAttribute("aria-labelledby")).toBe(historyTab.id);
  });
});

describe("BackgroundWorkMobilePanel", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  afterEach(() => {
    cleanup();
  });

  it("opens the independent Panels surface on History and closes through its invoker", () => {
    mockWorkloads = [workload({ work_id: WORK_ID, state: "unknown", title: UNKNOWN_TITLE })];
    const onClose = vi.fn();
    render(<BackgroundWorkMobilePanel sessionId={SESSION_ID} onClose={onClose} />);

    expect(screen.getByTestId(MOBILE_OVERVIEW_TEST_ID)).toBeTruthy();
    expect(screen.getByTestId(TAB_HISTORY_TEST_ID).getAttribute(DATA_STATE_ATTR)).toBe(
      ACTIVE_TAB_STATE,
    );
    expect(screen.getByTestId(CARD_WORK_1_TEST_ID)).toBeTruthy();

    fireEvent.click(screen.getByTestId("background-work-overview-close"));
    expect(onClose).toHaveBeenCalledOnce();
  });
});
