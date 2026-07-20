import { describe, it, expect, vi, beforeEach } from "vitest";
import { renderHook } from "@testing-library/react";
import { createElement, type ReactNode } from "react";
import { QueryClientProvider } from "@tanstack/react-query";
import { makeQueryClient } from "@/lib/query/client";
import { qk } from "@/lib/query/keys";
import type { TaskPlan } from "@/lib/types/http";

const mockAddPlanPanel = vi.fn();
const mockGetPanel = vi.fn();
const mockHydrateTaskPlanLastSeen = vi.fn();
const mockMarkTaskPlanSeen = vi.fn();
const mockGetTaskPlan = vi.fn();

let mockActiveTaskId: string | null = "task-1";
let mockPlan: TaskPlan | null = null;
let mockLastSeen: string | undefined = undefined;
let mockIsLoaded = true;
let mockConnectionStatus = "connected";
let mockIsRestoringLayout = false;
let mockActivePanelId: string | null = "chat";
let mockApi: { getPanel: typeof mockGetPanel; activePanel?: { id: string } | null } | null = null;

function makeApi() {
  return {
    getPanel: mockGetPanel,
    activePanel: mockActivePanelId ? { id: mockActivePanelId } : null,
  };
}

function buildState() {
  return {
    tasks: { activeTaskId: mockActiveTaskId },
    taskPlans: {
      lastSeenUpdatedAtByTaskId:
        mockActiveTaskId && mockLastSeen !== undefined ? { [mockActiveTaskId]: mockLastSeen } : {},
    },
    connection: { status: mockConnectionStatus },
    hydrateTaskPlanLastSeen: mockHydrateTaskPlanLastSeen,
    markTaskPlanSeen: mockMarkTaskPlanSeen,
  };
}

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: Record<string, unknown>) => unknown) => selector(buildState()),
}));

vi.mock("@/lib/state/dockview-store", () => ({
  useDockviewStore: (selector: (state: Record<string, unknown>) => unknown) =>
    selector({
      api: mockApi,
      isRestoringLayout: mockIsRestoringLayout,
      addPlanPanel: mockAddPlanPanel,
    }),
}));

vi.mock("@/lib/api/domains/plan-api", () => ({
  createTaskPlan: vi.fn(),
  deleteTaskPlan: vi.fn(),
  getPlanRevision: vi.fn(),
  getTaskPlan: (...args: unknown[]) => mockGetTaskPlan(...args),
  listPlanRevisions: vi.fn(),
  revertPlanRevision: vi.fn(),
  updateTaskPlan: vi.fn(),
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

function renderPlanPanelAutoOpen() {
  const client = makeQueryClient();
  if (mockActiveTaskId && mockIsLoaded) {
    client.setQueryData(qk.taskPlan.detail(mockActiveTaskId), mockPlan);
  }
  const wrapper = ({ children }: { children: ReactNode }) =>
    createElement(QueryClientProvider, { client }, children);
  return { ...renderHook(() => usePlanPanelAutoOpen(), { wrapper }), client };
}

describe("usePlanPanelAutoOpen", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockActiveTaskId = "task-1";
    mockPlan = agentPlan();
    mockLastSeen = undefined;
    mockIsLoaded = true;
    mockConnectionStatus = "connected";
    mockIsRestoringLayout = false;
    mockActivePanelId = "chat";
    mockApi = makeApi();
    mockGetPanel.mockReturnValue(null);
    mockGetTaskPlan.mockResolvedValue(null);
  });

  it("opens plan panel for unseen agent plan", () => {
    renderPlanPanelAutoOpen();
    expect(mockAddPlanPanel).toHaveBeenCalledWith({ quiet: true, inCenter: true });
  });

  it("does not open when isRestoringLayout is true", () => {
    mockIsRestoringLayout = true;
    renderPlanPanelAutoOpen();
    expect(mockAddPlanPanel).not.toHaveBeenCalled();
  });

  it("does not open when api is null", () => {
    mockApi = null;
    renderPlanPanelAutoOpen();
    expect(mockAddPlanPanel).not.toHaveBeenCalled();
  });

  it("does not open when plan created_by is user", () => {
    mockPlan = { ...agentPlan(), created_by: "user" };
    renderPlanPanelAutoOpen();
    expect(mockAddPlanPanel).not.toHaveBeenCalled();
  });

  it("does not open when plan is already seen (lastSeen === updated_at)", () => {
    mockLastSeen = TS;
    renderPlanPanelAutoOpen();
    expect(mockAddPlanPanel).not.toHaveBeenCalled();
  });

  it("opens again when plan is updated after being seen", () => {
    mockLastSeen = TS;
    mockPlan = agentPlan(TS_LATER);
    renderPlanPanelAutoOpen();
    expect(mockAddPlanPanel).toHaveBeenCalledWith({ quiet: true, inCenter: true });
  });

  it("does not open when plan panel already exists in layout", () => {
    mockGetPanel.mockReturnValue({ id: "plan" });
    renderPlanPanelAutoOpen();
    expect(mockAddPlanPanel).not.toHaveBeenCalled();
  });

  it("does not open when plan is null", () => {
    mockPlan = null;
    renderPlanPanelAutoOpen();
    expect(mockAddPlanPanel).not.toHaveBeenCalled();
  });
});

describe("usePlanPanelAutoOpen — eager fetch", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockActiveTaskId = "task-1";
    mockPlan = agentPlan();
    mockLastSeen = undefined;
    mockIsLoaded = true;
    mockConnectionStatus = "connected";
    mockIsRestoringLayout = false;
    mockActivePanelId = "chat";
    mockApi = makeApi();
    mockGetPanel.mockReturnValue(null);
    mockGetTaskPlan.mockResolvedValue(null);
  });

  it("eagerly fetches the plan when not yet loaded", () => {
    mockIsLoaded = false;
    mockPlan = null;
    renderPlanPanelAutoOpen();
    expect(mockGetTaskPlan).toHaveBeenCalledWith("task-1");
  });

  it("does not fetch when WS is disconnected", () => {
    mockIsLoaded = false;
    mockConnectionStatus = "connecting";
    renderPlanPanelAutoOpen();
    expect(mockGetTaskPlan).not.toHaveBeenCalled();
  });

  it("acknowledges the plan on hydrate when the restored panel is active", () => {
    mockGetPanel.mockReturnValue({ id: "plan" });
    mockActivePanelId = "plan";
    mockApi = makeApi();
    mockLastSeen = undefined;
    renderPlanPanelAutoOpen();
    expect(mockMarkTaskPlanSeen).toHaveBeenCalledWith("task-1", TS);
    expect(mockAddPlanPanel).not.toHaveBeenCalled();
  });

  it("does not acknowledge the plan on hydrate when the restored panel is inactive", () => {
    mockGetPanel.mockReturnValue({ id: "plan" });
    mockActivePanelId = "chat";
    mockApi = makeApi();
    mockLastSeen = undefined;
    renderPlanPanelAutoOpen();
    expect(mockMarkTaskPlanSeen).not.toHaveBeenCalled();
    expect(mockAddPlanPanel).not.toHaveBeenCalled();
  });

  it("does not acknowledge a live update when lastSeen is already recorded", () => {
    mockGetPanel.mockReturnValue({ id: "plan" });
    mockLastSeen = TS;
    mockPlan = agentPlan(TS_LATER);
    renderPlanPanelAutoOpen();
    expect(mockMarkTaskPlanSeen).not.toHaveBeenCalled();
    expect(mockAddPlanPanel).not.toHaveBeenCalled();
  });

  it("does not acknowledge a panel it just auto-opened when the eager fetch re-applies the plan", () => {
    // First render: panel doesn't exist yet, so we auto-open it.
    mockGetPanel.mockReturnValue(null);
    const { client, rerender } = renderPlanPanelAutoOpen();
    expect(mockAddPlanPanel).toHaveBeenCalledTimes(1);

    // The eager getTaskPlan self-heal resolves after the WS push and
    // re-applies an equivalent plan object (new reference, same updated_at),
    // re-running the effect. By now the panel we added is registered, so
    // getPanel returns it while lastSeen is still undefined. This must NOT
    // mark the plan seen — that would suppress the indicator the user expects.
    mockGetPanel.mockReturnValue({ id: "plan" });
    mockPlan = agentPlan();
    client.setQueryData(qk.taskPlan.detail("task-1"), mockPlan);
    rerender();

    expect(mockMarkTaskPlanSeen).not.toHaveBeenCalled();
  });

  it("does not retry the eager fetch after a failure", async () => {
    mockIsLoaded = false;
    mockPlan = null;
    let rejectFn: (err: Error) => void = () => {};
    mockGetTaskPlan.mockImplementation(
      () =>
        new Promise((_, reject) => {
          rejectFn = reject;
        }),
    );
    const { rerender } = renderPlanPanelAutoOpen();
    expect(mockGetTaskPlan).toHaveBeenCalledTimes(1);
    rejectFn(new Error("boom"));
    await new Promise((r) => setTimeout(r, 0));
    rerender();
    rerender();
    expect(mockGetTaskPlan).toHaveBeenCalledTimes(1);
  });
});

describe("usePlanPanelAutoOpen — race guards", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockActiveTaskId = "task-1";
    mockPlan = agentPlan();
    mockLastSeen = undefined;
    mockIsLoaded = true;
    mockConnectionStatus = "connected";
    mockIsRestoringLayout = false;
    mockActivePanelId = "chat";
    mockApi = makeApi();
    mockGetPanel.mockReturnValue(null);
    mockGetTaskPlan.mockResolvedValue(null);
  });

  it("does not overwrite a newer WS-delivered plan with an older HTTP result", async () => {
    mockIsLoaded = false;
    mockPlan = null;
    let resolveFn: (v: TaskPlan | null) => void = () => {};
    mockGetTaskPlan.mockImplementation(
      () =>
        new Promise((resolve) => {
          resolveFn = resolve;
        }),
    );
    const { client } = renderPlanPanelAutoOpen();
    // WS delivers the latest plan while the HTTP fetch is in flight.
    client.setQueryData(qk.taskPlan.detail("task-1"), agentPlan(TS_LATER));
    // HTTP resolves with an older snapshot of the same plan.
    resolveFn(agentPlan(TS));
    await new Promise((r) => setTimeout(r, 0));
    expect(client.getQueryData(qk.taskPlan.detail("task-1"))).toMatchObject({
      updated_at: TS_LATER,
    });
  });

  it("does not overwrite a WS-delivered plan when the fetch resolves with null", async () => {
    mockIsLoaded = false;
    mockPlan = null;
    let resolveFn: (v: TaskPlan | null) => void = () => {};
    mockGetTaskPlan.mockImplementation(
      () =>
        new Promise((resolve) => {
          resolveFn = resolve;
        }),
    );
    const { client } = renderPlanPanelAutoOpen();
    // Simulate a WS event populating the query cache while the HTTP fetch is in flight.
    client.setQueryData(qk.taskPlan.detail("task-1"), agentPlan());
    resolveFn(null);
    await new Promise((r) => setTimeout(r, 0));
    expect(client.getQueryData(qk.taskPlan.detail("task-1"))).toMatchObject({
      updated_at: TS,
    });
  });

  it("retries the eager fetch after WS reconnects following a failure", async () => {
    mockIsLoaded = false;
    mockPlan = null;
    mockGetTaskPlan.mockRejectedValueOnce(new Error("boom"));
    mockGetTaskPlan.mockResolvedValueOnce(null);

    const { rerender } = renderPlanPanelAutoOpen();
    expect(mockGetTaskPlan).toHaveBeenCalledTimes(1);
    await new Promise((r) => setTimeout(r, 0));

    // WS disconnect — clears the attempted set
    mockConnectionStatus = "connecting";
    rerender();

    // WS reconnect — fetches again
    mockConnectionStatus = "connected";
    rerender();
    await new Promise((r) => setTimeout(r, 0));
    expect(mockGetTaskPlan).toHaveBeenCalledTimes(2);
  });
});
