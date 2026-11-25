import { renderHook } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { Automation, AutomationRun, AutomationSummary } from "@/lib/types/automation";

const WORKSPACE_ID = "workspace-1";

const summaryState = vi.hoisted(() => ({
  value: {
    summaries: [] as AutomationSummary[],
    loading: true,
    error: null as string | null,
    refresh: vi.fn(),
  },
}));

vi.mock("@/components/runs/use-automation-summaries", () => ({
  useAutomationSummaries: () => summaryState.value,
}));

vi.mock("@/components/runs/use-live-refresh", () => ({
  useLiveRefresh: vi.fn(),
}));

vi.mock("@/hooks/use-foreground-refresh", () => ({
  useForegroundRefresh: vi.fn(),
}));

import { useShortcutActivity } from "./use-shortcut-activity";

function automation(id: string, enabled = true): Automation {
  return {
    id,
    workspace_id: WORKSPACE_ID,
    name: id,
    enabled,
    max_concurrent_runs: 1,
    triggers: [],
    created_at: "2026-01-01T00:00:00Z",
    description: "",
    workflow_id: "workflow-1",
    workflow_step_id: "step-1",
    agent_profile_id: "agent-1",
    executor_profile_id: "executor-1",
    repository_ids: [],
    prompt: "",
    task_title_template: "",
    last_triggered_at: null,
    updated_at: "2026-01-01T00:00:00Z",
  };
}

function summary(id: string, openRuns: number): AutomationSummary {
  return { automation_id: id, open_runs: openRuns, last_run: undefined };
}

function mkRun(overrides: Partial<AutomationRun> = {}): AutomationRun {
  return {
    id: "failed-run",
    automation_id: "a",
    trigger_id: "",
    trigger_type: "manual",
    task_id: "",
    status: "failed",
    dedup_key: "",
    trigger_data: {},
    error_message: "",
    created_at: "2026-11-05T10:00:00Z",
    ...overrides,
  };
}

describe("useShortcutActivity", () => {
  beforeEach(() => {
    summaryState.value = {
      summaries: [],
      loading: true,
      error: null,
      refresh: vi.fn(),
    };
  });

  it("does not claim idle while the summary collection is loading", () => {
    const { result } = renderHook(() =>
      useShortcutActivity({
        workspaceId: WORKSPACE_ID,
        automations: [automation("automation-1")],
        automationIds: ["automation-1"],
      }),
    );

    expect(result.current.getActivity("automation-1").state).toBe("unknown");
  });

  it("derives running, idle, and paused from the existing automation state semantics", () => {
    summaryState.value = {
      ...summaryState.value,
      loading: false,
      summaries: [summary("running", 1), summary("idle", 0), summary("paused", 0)],
    };
    const automations = [automation("running"), automation("idle"), automation("paused", false)];
    const { result } = renderHook(() =>
      useShortcutActivity({
        workspaceId: WORKSPACE_ID,
        automations,
        automationIds: automations.map(({ id }) => id),
      }),
    );

    expect(result.current.getActivity("running").state).toBe("running");
    expect(result.current.getActivity("idle").state).toBe("idle");
    expect(result.current.getActivity("paused").state).toBe("paused");
  });

  it("marks a failed refresh unknown and aggregates only known running pins", () => {
    summaryState.value = {
      ...summaryState.value,
      loading: false,
      error: "offline",
      summaries: [summary("running", 1)],
    };
    const { result } = renderHook(() =>
      useShortcutActivity({
        workspaceId: WORKSPACE_ID,
        automations: [automation("running")],
        automationIds: ["running", "missing"],
      }),
    );

    expect(result.current.getActivity("running").state).toBe("unknown");
    expect(result.current.hasRunningActivity(["running", "missing"])).toBe(false);
  });

  it("keeps workspace-scoped definitions and summaries from becoming idle when the pin is missing", () => {
    summaryState.value = { ...summaryState.value, loading: false, summaries: [] };
    const { result } = renderHook(() =>
      useShortcutActivity({
        workspaceId: "workspace-2",
        automations: [],
        automationIds: ["workspace-1-automation"],
      }),
    );

    expect(result.current.getActivity("workspace-1-automation").state).toBe("unknown");
  });
});

// @covers AC-OFFICE-AUTOMATION-RUNS-001.20 AC-OFFICE-AUTOMATION-RUNS-001.22
it("failure survives the active phone projection without becoming running", () => {
  const lastRun = mkRun();
  summaryState.value = {
    ...summaryState.value,
    loading: false,
    error: null,
    summaries: [{ automation_id: "a", open_runs: 0, last_run: lastRun }],
  };
  const { result } = renderHook(() =>
    useShortcutActivity({
      workspaceId: WORKSPACE_ID,
      automations: [automation("a")],
      automationIds: ["a"],
    }),
  );
  expect(result.current.getActivity("a")).toMatchObject({ state: "idle", lastRun });
  expect(result.current.hasRunningActivity(["a"])).toBe(false);
});

it.each(["loading", "error", "definitions", "missing"])(
  "unavailable %s activity does not expose an outcome",
  (unavailable) => {
    summaryState.value = {
      ...summaryState.value,
      loading: unavailable === "loading",
      error: unavailable === "error" ? "offline" : null,
      summaries: [
        {
          automation_id: "a",
          open_runs: 0,
          last_run: mkRun({ id: "stale" }),
        },
      ],
    };
    const { result } = renderHook(() =>
      useShortcutActivity({
        workspaceId: WORKSPACE_ID,
        automations: unavailable === "missing" ? [] : [automation("a")],
        automationIds: ["a"],
        definitionsLoading: unavailable === "definitions",
      }),
    );
    expect(result.current.getActivity("a")).toMatchObject({ state: "unknown" });
    expect(result.current.getActivity("a")).not.toHaveProperty("lastRun");
  },
);
