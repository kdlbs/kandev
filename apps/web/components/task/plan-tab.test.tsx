import { describe, it, expect, vi, beforeEach } from "vitest";
import { render } from "@testing-library/react";
import type { TaskPlan } from "@/lib/types/http";

const mockMarkTaskPlanSeen = vi.fn();

let mockActiveTaskId: string | null = "task-1";
let mockPlan: TaskPlan | null = null;
let mockLastSeen: string | undefined = undefined;

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: Record<string, unknown>) => unknown) =>
    selector({
      tasks: { activeTaskId: mockActiveTaskId },
      taskPlans: {
        byTaskId: mockActiveTaskId && mockPlan ? { [mockActiveTaskId]: mockPlan } : {},
        lastSeenUpdatedAtByTaskId:
          mockActiveTaskId && mockLastSeen !== undefined
            ? { [mockActiveTaskId]: mockLastSeen }
            : {},
      },
      markTaskPlanSeen: mockMarkTaskPlanSeen,
    }),
}));

vi.mock("dockview-react", () => ({
  DockviewDefaultTab: () => null,
}));

import { PlanTab } from "./plan-tab";

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

type TabApi = {
  isActive: boolean;
  onDidActiveChange: (listener: (e: { isActive: boolean }) => void) => { dispose: () => void };
};

function makeApi(isActive: boolean): TabApi {
  return {
    isActive,
    onDidActiveChange: () => ({ dispose: () => {} }),
  };
}

describe("PlanTab", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockActiveTaskId = "task-1";
    mockPlan = agentPlan();
    mockLastSeen = undefined;
  });

  it("renders the indicator when an agent-authored plan is unseen", () => {
    const { container } = render(
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      <PlanTab {...({ api: makeApi(false) } as any)} />,
    );
    expect(container.querySelector('[data-testid="plan-tab-indicator"]')).toBeTruthy();
  });

  it("does not render the indicator when the plan is user-authored", () => {
    mockPlan = { ...agentPlan(), created_by: "user" };
    const { container } = render(
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      <PlanTab {...({ api: makeApi(false) } as any)} />,
    );
    expect(container.querySelector('[data-testid="plan-tab-indicator"]')).toBeNull();
  });

  it("does not render the indicator when lastSeen matches plan.updated_at", () => {
    mockLastSeen = TS;
    const { container } = render(
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      <PlanTab {...({ api: makeApi(false) } as any)} />,
    );
    expect(container.querySelector('[data-testid="plan-tab-indicator"]')).toBeNull();
  });

  it("marks seen on initial render when api.isActive is true", () => {
    render(
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      <PlanTab {...({ api: makeApi(true) } as any)} />,
    );
    expect(mockMarkTaskPlanSeen).toHaveBeenCalledWith("task-1");
  });

  it("marks seen synchronously when an update lands while the tab is already active", () => {
    // Initial render with the tab active and plan already seen
    mockLastSeen = TS;
    const { rerender } = render(
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      <PlanTab {...({ api: makeApi(true) } as any)} />,
    );
    mockMarkTaskPlanSeen.mockClear();

    // Plan updates while the tab is still active — re-render with new updated_at.
    mockPlan = agentPlan(TS_LATER);
    rerender(
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      <PlanTab {...({ api: makeApi(true) } as any)} />,
    );

    // useLayoutEffect must have fired markTaskPlanSeen — keeps the dot from flashing.
    expect(mockMarkTaskPlanSeen).toHaveBeenCalledWith("task-1");
  });

  it("does not mark seen on update when the tab is not active", () => {
    mockLastSeen = TS;
    const { rerender } = render(
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      <PlanTab {...({ api: makeApi(false) } as any)} />,
    );
    mockMarkTaskPlanSeen.mockClear();

    mockPlan = agentPlan(TS_LATER);
    rerender(
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      <PlanTab {...({ api: makeApi(false) } as any)} />,
    );

    expect(mockMarkTaskPlanSeen).not.toHaveBeenCalled();
  });
});
