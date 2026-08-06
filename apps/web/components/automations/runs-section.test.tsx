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

function setup(runs: AutomationRun[]) {
  mockUseAutomationRuns.mockReturnValue({
    runs,
    loading: false,
    refresh: vi.fn(),
    deleteRun: vi.fn(),
    deleteAllRuns: vi.fn(),
  });
  render(<RunsSection automationId="auto-1" workspaceId="ws-1" />);
  // The runs table only renders once the "Recent Runs" disclosure is expanded.
  fireEvent.click(screen.getByText(/Recent Runs/));
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
