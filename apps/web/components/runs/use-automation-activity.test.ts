import { act, renderHook, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const mockGet = vi.fn();
const mockRuns = vi.fn();
const mockSummary = vi.fn();
vi.mock("@/lib/api/domains/automation-api", () => ({
  getAutomation: (...args: unknown[]) => mockGet(...args),
  listAutomationRuns: (...args: unknown[]) => mockRuns(...args),
  getAutomationSummary: (...args: unknown[]) => mockSummary(...args),
}));

import { AUTOMATION_RUNS_LIMIT, useAutomationActivity } from "./use-automation-activity";
import type { Automation, AutomationRun } from "@/lib/types/automation";

const AUTO_1 = "auto-1";
const AUTO_2 = "auto-2";

function mkAutomation(id: string): Automation {
  return {
    id,
    workspace_id: "ws-1",
    name: `Automation ${id}`,
    description: "",
    workflow_id: "",
    workflow_step_id: "",
    agent_profile_id: "",
    executor_profile_id: "",
    repository_ids: [],
    prompt: "",
    task_title_template: "",
    enabled: true,
    max_concurrent_runs: 1,
    last_triggered_at: null,
    created_at: "2026-07-01T00:00:00Z",
    updated_at: "2026-07-01T00:00:00Z",
    triggers: [],
  };
}

function mkRun(id: string, automationId = AUTO_1): AutomationRun {
  return {
    id,
    automation_id: automationId,
    trigger_id: "trig-1",
    trigger_type: "scheduled",
    task_id: "task-1",
    status: "succeeded",
    dedup_key: "",
    trigger_data: {},
    error_message: "",
    created_at: "2026-07-30T00:00:00Z",
  };
}

beforeEach(() => {
  mockGet.mockResolvedValue(mkAutomation(AUTO_1));
  mockRuns.mockResolvedValue([]);
  mockSummary.mockResolvedValue({ automation_id: AUTO_1, open_runs: 0 });
});

afterEach(() => {
  vi.clearAllMocks();
});

describe("useAutomationActivity", () => {
  it("loads the automation and its runs under a capped limit", async () => {
    mockRuns.mockResolvedValue([mkRun("r1")]);

    const { result } = renderHook(() => useAutomationActivity(AUTO_1));

    await waitFor(() => expect(result.current.loading).toBe(false));
    expect(mockGet).toHaveBeenCalledWith(AUTO_1);
    expect(mockRuns).toHaveBeenCalledWith(AUTO_1, AUTOMATION_RUNS_LIMIT);
    expect(result.current.automation?.id).toBe(AUTO_1);
    expect(result.current.runs.map((run) => run.id)).toEqual(["r1"]);
  });

  it("does not call the API without an automation id", async () => {
    const { result } = renderHook(() => useAutomationActivity(""));

    await waitFor(() => expect(result.current.loading).toBe(false));
    expect(mockGet).not.toHaveBeenCalled();
    expect(result.current.automation).toBeNull();
  });

  it("surfaces a failure and keeps nothing stale on screen", async () => {
    mockGet.mockRejectedValue(new Error("no such automation"));

    const { result } = renderHook(() => useAutomationActivity(AUTO_1));

    await waitFor(() => expect(result.current.error).toBe("no such automation"));
    expect(result.current.automation).toBeNull();
    expect(result.current.runs).toEqual([]);
  });

  it("recovers on refresh after a failure", async () => {
    mockGet.mockRejectedValueOnce(new Error("boom")).mockResolvedValue(mkAutomation(AUTO_1));
    mockRuns.mockResolvedValue([mkRun("r9")]);

    const { result } = renderHook(() => useAutomationActivity(AUTO_1));
    await waitFor(() => expect(result.current.error).toBe("boom"));

    act(() => result.current.refresh());

    await waitFor(() => expect(result.current.runs.map((run) => run.id)).toEqual(["r9"]));
    expect(result.current.error).toBeNull();
  });

  it("never shows one automation's runs under another's name", async () => {
    mockRuns.mockResolvedValue([mkRun("r1")]);
    const { result, rerender } = renderHook(
      ({ automationId }: { automationId: string }) => useAutomationActivity(automationId),
      { initialProps: { automationId: AUTO_1 } },
    );
    await waitFor(() => expect(result.current.runs.map((run) => run.id)).toEqual(["r1"]));

    mockGet.mockReturnValue(new Promise(() => {}));
    mockRuns.mockReturnValue(new Promise(() => {}));
    rerender({ automationId: AUTO_2 });

    expect(result.current.automation).toBeNull();
    expect(result.current.runs).toEqual([]);
  });

  it("drops a response that arrives after the automation already changed", async () => {
    let resolveFirst: ((value: Automation) => void) | undefined;
    mockGet.mockImplementationOnce(
      () =>
        new Promise<Automation>((resolve) => {
          resolveFirst = resolve;
        }),
    );

    const { result, rerender } = renderHook(
      ({ automationId }: { automationId: string }) => useAutomationActivity(automationId),
      { initialProps: { automationId: AUTO_1 } },
    );

    mockGet.mockResolvedValue(mkAutomation(AUTO_2));
    mockRuns.mockResolvedValue([mkRun("r2", AUTO_2)]);
    rerender({ automationId: AUTO_2 });
    await waitFor(() => expect(result.current.automation?.id).toBe(AUTO_2));

    await act(async () => {
      resolveFirst?.(mkAutomation(AUTO_1));
    });

    expect(result.current.automation?.id).toBe(AUTO_2);
    expect(result.current.runs.map((run) => run.id)).toEqual(["r2"]);
  });
});

describe("useAutomationActivity open-run count", () => {
  it("reports the server's count, not one counted from the capped window", async () => {
    // The window holds only finished runs; the automation still has one open.
    // Counting from the window would report nothing in flight, which is what
    // gates both the "still running" reason and the page's polling.
    mockRuns.mockResolvedValue([mkRun("finished")]);
    mockSummary.mockResolvedValue({ automation_id: AUTO_1, open_runs: 2 });

    const { result } = renderHook(() => useAutomationActivity(AUTO_1));

    await waitFor(() => expect(result.current.openRuns).toBe(2));
    expect(mockSummary).toHaveBeenCalledWith(AUTO_1);
  });

  it("reads no open runs when the automation has never run", async () => {
    mockSummary.mockResolvedValue(null);

    const { result } = renderHook(() => useAutomationActivity(AUTO_1));

    await waitFor(() => expect(result.current.loading).toBe(false));
    expect(result.current.openRuns).toBe(0);
  });

  it("shows nothing from the previous automation while switching", async () => {
    mockSummary.mockResolvedValue({ automation_id: AUTO_1, open_runs: 3 });
    const { result, rerender } = renderHook(({ id }) => useAutomationActivity(id), {
      initialProps: { id: AUTO_1 },
    });
    await waitFor(() => expect(result.current.openRuns).toBe(3));

    mockSummary.mockImplementation(() => new Promise(() => {}));
    rerender({ id: AUTO_2 });

    expect(result.current.openRuns).toBe(0);
  });
});
