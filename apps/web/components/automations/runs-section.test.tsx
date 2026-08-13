import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { AutomationRun } from "@/lib/types/automation";

const mockUseAutomationRuns = vi.fn();

vi.mock("@/hooks/domains/settings/use-automation-runs", () => ({
  useAutomationRuns: (...args: unknown[]) => mockUseAutomationRuns(...args),
}));

const RUN_ARCHIVED = "run-archived";
const RUN_CANCELLED = "run-cancelled";
const ARCHIVED = "Archived";
const CANCELLED = "Cancelled";

const mockPush = vi.fn();
vi.mock("@/lib/routing/client-router", () => ({
  useRouter: () => ({ push: mockPush }),
}));

import { RunsSection } from "./runs-section";

function mkRun(overrides: Partial<AutomationRun> = {}): AutomationRun {
  return {
    id: "run-1",
    automation_id: "auto-1",
    trigger_id: "trig-1",
    trigger_type: "scheduled",
    task_id: "task-1",
    status: "task_created",
    dedup_key: "",
    trigger_data: {},
    error_message: "",
    created_at: new Date().toISOString(),
    ...overrides,
  };
}

type RunsSectionHook = {
  runs: AutomationRun[];
  loading: boolean;
  deleting: boolean;
  refresh: () => void;
  deleteRun: (id: string) => void;
  deleteAllRuns: (runIds?: string[]) => void;
};

function setup(
  runs: AutomationRun[],
  overrides: Partial<RunsSectionHook> = {},
): ReturnType<typeof render> {
  mockUseAutomationRuns.mockReturnValue({
    runs,
    loading: false,
    deleting: false,
    refresh: vi.fn(),
    deleteRun: vi.fn(),
    deleteAllRuns: vi.fn(),
    ...overrides,
  });
  const view = render(<RunsSection automationId="auto-1" workspaceId="ws-1" />);
  // The runs table only renders once the "Recent Runs" disclosure is expanded.
  // The heading is count-aware ("Recent Run (1)" / "Recent Runs (2)"), so the
  // matcher must tolerate both forms rather than pinning the plural.
  fireEvent.click(screen.getByText(/Recent Runs?\s*\(/));
  return view;
}

describe("RunsSection status badges", () => {
  afterEach(() => {
    cleanup();
    vi.clearAllMocks();
  });

  // The status filter chips carry the same words as the badges, so these
  // assertions read the badge inside the run's own row rather than any element
  // with that text.
  const badgeOf = (runId: string) =>
    screen.getByTestId(`run-row-${runId}`).querySelector("[data-slot='badge']")?.textContent ?? "";

  it("labels an archived-task run as Archived, not Cancelled", () => {
    setup([mkRun({ id: RUN_ARCHIVED, status: "archived" })]);

    expect(badgeOf(RUN_ARCHIVED)).toBe(ARCHIVED);
  });

  it("labels a genuinely cancelled-task run as Cancelled", () => {
    setup([mkRun({ id: RUN_CANCELLED, status: "cancelled" })]);

    expect(badgeOf(RUN_CANCELLED)).toBe(CANCELLED);
  });

  it("distinguishes archived and cancelled runs side by side", () => {
    setup([
      mkRun({ id: RUN_ARCHIVED, status: "archived" }),
      mkRun({ id: RUN_CANCELLED, status: "cancelled" }),
    ]);

    expect(badgeOf(RUN_ARCHIVED)).toBe(ARCHIVED);
    expect(badgeOf(RUN_CANCELLED)).toBe(CANCELLED);
  });
});

describe("RunsSection run log", () => {
  afterEach(() => {
    cleanup();
    vi.clearAllMocks();
  });

  it("links a run to its task even in run mode", () => {
    // Run mode keeps the task off the board. That is not a reason to withhold
    // the only route to what the run said — previously these rows were inert.
    setup([mkRun({ id: "run-x", task_id: "task-hidden", status: "succeeded" })]);

    fireEvent.click(screen.getByTestId("run-row-run-x"));

    expect(mockPush).toHaveBeenCalledWith("/tasks/task-hidden");
  });

  it("does not link a run that never produced a task", () => {
    setup([mkRun({ id: "run-skipped", task_id: "", status: "skipped" })]);

    fireEvent.click(screen.getByTestId("run-row-run-skipped"));

    expect(mockPush).not.toHaveBeenCalled();
  });

  it("shows what the run actually said", () => {
    setup([
      mkRun({ id: "run-s", status: "succeeded", summary: "Sweep complete across all 32 specs." }),
    ]);

    expect(screen.getByTestId("run-outcome").textContent).toContain("Sweep complete");
  });

  it("prefers the error over the summary when a run failed", () => {
    setup([
      mkRun({
        id: "run-f",
        status: "skipped",
        error_message: "max_concurrent_runs=1 reached",
        summary: "irrelevant",
      }),
    ]);

    expect(screen.getByTestId("run-outcome").textContent).toContain("max_concurrent_runs=1");
  });

  it("can filter down to skipped runs, which are otherwise silent", () => {
    setup([
      mkRun({ id: "run-ok", status: "succeeded", summary: "all good" }),
      mkRun({ id: "run-skip", status: "skipped", error_message: "max_concurrent_runs=1 reached" }),
    ]);

    fireEvent.click(screen.getByTestId("run-filter-skipped"));

    expect(screen.getByTestId("run-row-run-skip")).toBeTruthy();
    expect(screen.queryByTestId("run-row-run-ok")).toBeNull();
  });

  it("offers no filter for a status this automation has never produced", () => {
    setup([mkRun({ id: "run-ok", status: "succeeded" })]);

    expect(screen.queryByTestId("run-filter-failed")).toBeNull();
    expect(screen.getByTestId("run-filter-succeeded")).toBeTruthy();
  });
});

const DELETE_ALL_BTN = "delete-all-runs";
const DELETE_ALL_CONFIRM = "delete-all-runs-confirm";
const FILTER_SKIPPED = "run-filter-skipped";

describe("RunsSection delete-all scope", () => {
  afterEach(() => {
    cleanup();
    vi.clearAllMocks();
  });

  it("renders delete-all in the rightmost table header cell, not beside the heading", () => {
    setup([mkRun({ id: "run-solo", status: "skipped" })]);

    const button = screen.getByTestId(DELETE_ALL_BTN);
    const headerRow = button.closest("thead")?.querySelector("tr");
    expect(headerRow).toBeTruthy();
    const cells = headerRow!.querySelectorAll("th");
    // Last header cell holds the control, aligned with the per-row delete
    // buttons in the same column.
    expect(cells[cells.length - 1].querySelector(`[data-testid="${DELETE_ALL_BTN}"]`)).toBeTruthy();
    // It no longer lives in the section header beside Refresh.
    expect(screen.getByTestId(DELETE_ALL_BTN).closest("thead")).toBeTruthy();
    // Unfiltered: the accessible name matches the full scope.
    expect(screen.getByTestId(DELETE_ALL_BTN).getAttribute("title")).toBe("Delete all runs");
  });

  it("deletes only the runs in the active status view", () => {
    const deleteAllRuns = vi.fn();
    setup(
      [
        mkRun({ id: "run-pass", status: "succeeded" }),
        mkRun({ id: "run-skip-1", status: "skipped" }),
        mkRun({ id: "run-skip-2", status: "skipped" }),
      ],
      { deleteAllRuns },
    );

    fireEvent.click(screen.getByTestId(FILTER_SKIPPED));
    fireEvent.click(screen.getByTestId(DELETE_ALL_BTN));
    fireEvent.click(screen.getByTestId(DELETE_ALL_CONFIRM));

    expect(deleteAllRuns).toHaveBeenCalledWith(["run-skip-1", "run-skip-2"]);
  });

  it("deletes every run from the All view with no id scope", () => {
    const deleteAllRuns = vi.fn();
    setup(
      [mkRun({ id: "run-a", status: "succeeded" }), mkRun({ id: "run-b", status: "skipped" })],
      { deleteAllRuns },
    );

    fireEvent.click(screen.getByTestId(DELETE_ALL_BTN));
    fireEvent.click(screen.getByTestId(DELETE_ALL_CONFIRM));

    expect(deleteAllRuns).toHaveBeenCalledWith();
  });

  it("hides delete-all when the active filter's runs are gone", () => {
    const { rerender } = setup([
      mkRun({ id: "run-temp", status: "skipped" }),
      mkRun({ id: "run-stay", status: "succeeded" }),
    ]);

    fireEvent.click(screen.getByTestId(FILTER_SKIPPED));
    expect(screen.getByTestId(DELETE_ALL_BTN)).toBeTruthy();

    // The runs refresh and the skipped one disappears server-side while the
    // filter is still active — the empty view must not offer delete-all.
    mockUseAutomationRuns.mockReturnValue({
      runs: [mkRun({ id: "run-stay", status: "succeeded" })],
      loading: false,
      refresh: vi.fn(),
      deleteRun: vi.fn(),
      deleteAllRuns: vi.fn(),
    });
    rerender(<RunsSection automationId="auto-1" workspaceId="ws-1" />);

    expect(screen.queryByTestId(DELETE_ALL_BTN)).toBeNull();
  });

  it("disables the delete controls while a delete is in flight", () => {
    setup(
      [mkRun({ id: "run-a", status: "succeeded" }), mkRun({ id: "run-b", status: "skipped" })],
      { deleting: true },
    );

    expect((screen.getByTestId(DELETE_ALL_BTN) as HTMLButtonElement).disabled).toBe(true);
    expect((screen.getAllByTestId("delete-run")[0] as HTMLButtonElement).disabled).toBe(true);
  });

  it("names the active status in the confirmation dialog", () => {
    setup([
      mkRun({ id: "run-stay", status: "succeeded" }),
      mkRun({ id: "run-filtered", status: "skipped" }),
    ]);

    fireEvent.click(screen.getByTestId(FILTER_SKIPPED));
    fireEvent.click(screen.getByTestId(DELETE_ALL_BTN));

    // The button's accessible name names the active status, matching the
    // dialog copy.
    expect(screen.getByTestId(DELETE_ALL_BTN).getAttribute("title")).toBe(
      "Delete all Skipped runs",
    );
    expect(
      screen.getByText(
        "This will permanently remove the Skipped runs shown in this view and their associated tasks. This cannot be undone.",
      ),
    ).toBeTruthy();
  });
});

describe("RunsSection derived statuses", () => {
  afterEach(() => {
    cleanup();
    vi.clearAllMocks();
  });

  it("can filter to archived and cancelled runs", () => {
    // Both are displayed statuses derived at read time, so leaving them out of
    // the filters made those runs visible but unreachable.
    setup([
      mkRun({ id: "run-ok", status: "succeeded" }),
      mkRun({ id: "run-arch", status: "archived" }),
      mkRun({ id: "run-canc", status: "cancelled" }),
    ]);

    fireEvent.click(screen.getByTestId("run-filter-archived"));
    expect(screen.getByTestId("run-row-run-arch")).toBeTruthy();
    expect(screen.queryByTestId("run-row-run-ok")).toBeNull();

    fireEvent.click(screen.getByTestId("run-filter-cancelled"));
    expect(screen.getByTestId("run-row-run-canc")).toBeTruthy();
    expect(screen.queryByTestId("run-row-run-arch")).toBeNull();
  });
});
