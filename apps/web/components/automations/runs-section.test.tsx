import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { AutomationRun } from "@/lib/types/automation";

const mockUseAutomationRuns = vi.fn();

vi.mock("@/hooks/domains/settings/use-automation-runs", () => ({
  useAutomationRuns: (...args: unknown[]) => mockUseAutomationRuns(...args),
}));

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
  render(<RunsSection automationId="auto-1" executionMode="task" workspaceId="ws-1" />);
  // The runs table only renders once the "Recent Runs" disclosure is expanded.
  fireEvent.click(screen.getByText(/Recent Runs/));
}

describe("RunsSection status badges", () => {
  afterEach(() => {
    cleanup();
    vi.clearAllMocks();
  });

  it("labels an archived-task run as Archived, not Cancelled", () => {
    setup([mkRun({ id: "run-archived", status: "archived" })]);

    expect(screen.getByText("Archived")).toBeTruthy();
    expect(screen.queryByText("Cancelled")).toBeNull();
  });

  it("labels a genuinely cancelled-task run as Cancelled", () => {
    setup([mkRun({ id: "run-cancelled", status: "cancelled" })]);

    expect(screen.getByText("Cancelled")).toBeTruthy();
    expect(screen.queryByText("Archived")).toBeNull();
  });

  it("distinguishes archived and cancelled runs side by side", () => {
    setup([
      mkRun({ id: "run-archived", status: "archived" }),
      mkRun({ id: "run-cancelled", status: "cancelled" }),
    ]);

    expect(screen.getByText("Archived")).toBeTruthy();
    expect(screen.getByText("Cancelled")).toBeTruthy();
  });
});
