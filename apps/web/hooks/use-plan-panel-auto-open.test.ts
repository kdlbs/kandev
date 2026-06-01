import { describe, it, expect, vi, beforeEach } from "vitest";
import { renderHook } from "@testing-library/react";
import type { TaskPlan } from "@/lib/types/http";

const mockAddPlanPanel = vi.fn();
const mockGetPanel = vi.fn();
const mockMarkTaskPlanSeen = vi.fn();

let mockActiveTaskId: string | null = "task-1";
let mockPlan: TaskPlan | null = null;
let mockLastSeen: string | undefined = undefined;
let mockIsRestoringLayout = false;
let mockApi: { getPanel: typeof mockGetPanel } | null = { getPanel: mockGetPanel };

function buildState() {
  return {
    tasks: { activeTaskId: mockActiveTaskId },
    taskPlans: {
      lastSeenUpdatedAtByTaskId:
        mockActiveTaskId && mockLastSeen !== undefined ? { [mockActiveTaskId]: mockLastSeen } : {},
    },
    markTaskPlanSeen: mockMarkTaskPlanSeen,
  };
}

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: Record<string, unknown>) => unknown) => selector(buildState()),
}));

// Plan now comes from the TanStack Query cache; mounting the query doubles as
// the eager fetch (TanStack Query owns fetching/retry/reconnect), so the hook
// no longer drives getTaskPlan / setTaskPlan / race-guard logic directly.
vi.mock("@tanstack/react-query", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@tanstack/react-query")>()),
  useQuery: () => ({ data: mockPlan ? { plan: mockPlan, lastSeenUpdatedAt: null } : undefined }),
}));

vi.mock("@/lib/state/dockview-store", () => ({
  useDockviewStore: (selector: (state: Record<string, unknown>) => unknown) =>
    selector({
      api: mockApi,
      isRestoringLayout: mockIsRestoringLayout,
      addPlanPanel: mockAddPlanPanel,
    }),
}));

import { usePlanPanelAutoOpen } from "./use-plan-panel-auto-open";

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

describe("usePlanPanelAutoOpen", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockActiveTaskId = "task-1";
    mockPlan = agentPlan();
    mockLastSeen = undefined;
    mockIsRestoringLayout = false;
    mockApi = { getPanel: mockGetPanel };
    mockGetPanel.mockReturnValue(null);
  });

  it("opens plan panel for unseen agent plan", () => {
    renderHook(() => usePlanPanelAutoOpen());
    expect(mockAddPlanPanel).toHaveBeenCalledWith({ quiet: true, inCenter: true });
  });

  it("does not open when isRestoringLayout is true", () => {
    mockIsRestoringLayout = true;
    renderHook(() => usePlanPanelAutoOpen());
    expect(mockAddPlanPanel).not.toHaveBeenCalled();
  });

  it("does not open when api is null", () => {
    mockApi = null;
    renderHook(() => usePlanPanelAutoOpen());
    expect(mockAddPlanPanel).not.toHaveBeenCalled();
  });

  it("does not open when plan created_by is user", () => {
    mockPlan = { ...agentPlan(), created_by: "user" };
    renderHook(() => usePlanPanelAutoOpen());
    expect(mockAddPlanPanel).not.toHaveBeenCalled();
  });

  it("does not open when plan is already seen (lastSeen === updated_at)", () => {
    mockLastSeen = TS;
    renderHook(() => usePlanPanelAutoOpen());
    expect(mockAddPlanPanel).not.toHaveBeenCalled();
  });

  it("opens again when plan is updated after being seen", () => {
    mockLastSeen = TS;
    mockPlan = agentPlan(TS_LATER);
    renderHook(() => usePlanPanelAutoOpen());
    expect(mockAddPlanPanel).toHaveBeenCalledWith({ quiet: true, inCenter: true });
  });

  it("does not open when plan panel already exists in layout", () => {
    mockGetPanel.mockReturnValue({ id: "plan" });
    renderHook(() => usePlanPanelAutoOpen());
    expect(mockAddPlanPanel).not.toHaveBeenCalled();
  });

  it("does not open when plan is null", () => {
    mockPlan = null;
    renderHook(() => usePlanPanelAutoOpen());
    expect(mockAddPlanPanel).not.toHaveBeenCalled();
  });

  it("acknowledges the plan on hydrate when the panel was restored", () => {
    mockGetPanel.mockReturnValue({ id: "plan" });
    mockLastSeen = undefined;
    renderHook(() => usePlanPanelAutoOpen());
    expect(mockMarkTaskPlanSeen).toHaveBeenCalledWith("task-1", TS);
    expect(mockAddPlanPanel).not.toHaveBeenCalled();
  });

  it("does not acknowledge a live update when lastSeen is already recorded", () => {
    mockGetPanel.mockReturnValue({ id: "plan" });
    mockLastSeen = TS;
    mockPlan = agentPlan(TS_LATER);
    renderHook(() => usePlanPanelAutoOpen());
    expect(mockMarkTaskPlanSeen).not.toHaveBeenCalled();
    expect(mockAddPlanPanel).not.toHaveBeenCalled();
  });
});
