import { describe, it, expect, vi, beforeEach } from "vitest";
import { renderHook } from "@testing-library/react";
import type { TaskPlan } from "@/lib/types/http";

/**
 * Regression test for the plan-tab "unseen update" indicator desync.
 *
 * Bug: after the taskPlans server state migrated to TanStack Query, useTaskPlan
 * marked the plan "seen" on every successful load (effect keyed on
 * `plan.updated_at`). The Plan panel mounts useTaskPlan even while its tab is
 * inactive and only AFTER the agent's plan is already cached, so this fired the
 * instant an agent wrote a plan — wiping `lastSeen` to the new `updated_at` and
 * suppressing the indicator entirely. Seen-state must be owned only by the tab
 * UIs (PlanTab / usePlanPanelAutoOpen / mobile badge), never by useTaskPlan.
 */

const mockMarkTaskPlanSeen = vi.fn();
const mockSetTaskPlanSaving = vi.fn();
const mockCachePlanRevisionContent = vi.fn();

let mockPlan: TaskPlan | null = null;

const TS = "2026-04-20T00:00:00Z";
const TS_LATER = "2026-04-20T01:00:00Z";

function agentPlan(updated_at = TS): TaskPlan {
  return {
    id: "plan-1",
    task_id: "task-1",
    title: "Plan",
    content: "# Plan",
    created_by: "agent",
    created_at: TS,
    updated_at,
  };
}

function buildState() {
  return {
    taskPlans: {
      savingByTaskId: {},
      previewRevisionIdByTaskId: {},
      comparePairByTaskId: {},
      revisionContentCache: {},
    },
    markTaskPlanSeen: mockMarkTaskPlanSeen,
    setTaskPlanSaving: mockSetTaskPlanSaving,
    cachePlanRevisionContent: mockCachePlanRevisionContent,
    setPreviewRevision: vi.fn(),
    toggleCompareSelection: vi.fn(),
    clearComparePair: vi.fn(),
  };
}

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: Record<string, unknown>) => unknown) => selector(buildState()),
  useAppStoreApi: () => ({ getState: () => buildState() }),
}));

// Server plan + revisions come from TanStack Query; the bridge keeps them live.
vi.mock("@tanstack/react-query", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@tanstack/react-query")>()),
  useQueryClient: () => ({ getQueryData: () => undefined, setQueryData: vi.fn() }),
  useQuery: (opts: { queryKey?: readonly unknown[] }) => {
    const isRevisions = Array.isArray(opts.queryKey) && opts.queryKey.includes("revisions");
    if (isRevisions) return { data: { revisions: [] }, isLoading: false, isSuccess: true };
    return {
      data: mockPlan ? { plan: mockPlan, lastSeenUpdatedAt: null } : { plan: null },
      isLoading: false,
      isSuccess: true,
    };
  },
}));

import { useTaskPlan } from "./use-task-plan";

describe("useTaskPlan — seen-state ownership", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockPlan = null;
  });

  it("does NOT mark the plan seen when an agent plan first loads into the cache", () => {
    mockPlan = agentPlan();
    renderHook(() => useTaskPlan("task-1"));
    expect(mockMarkTaskPlanSeen).not.toHaveBeenCalled();
  });

  it("does NOT mark the plan seen when the plan updates via a live WS write", () => {
    mockPlan = agentPlan();
    const { rerender } = renderHook(() => useTaskPlan("task-1"));
    mockMarkTaskPlanSeen.mockClear();

    // Agent updates the plan — updated_at advances.
    mockPlan = agentPlan(TS_LATER);
    rerender();

    expect(mockMarkTaskPlanSeen).not.toHaveBeenCalled();
  });

  it("does NOT mark seen on an initial load with no plan", () => {
    mockPlan = null;
    renderHook(() => useTaskPlan("task-1"));
    expect(mockMarkTaskPlanSeen).not.toHaveBeenCalled();
  });
});
