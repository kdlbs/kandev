import { act, cleanup, render } from "@testing-library/react";
import type { StoreApi } from "zustand";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { AppState } from "@/lib/state/store";

const renderCounts = vi.hoisted(() => ({
  lanes: new Map<string, number>(),
  cards: new Map<string, number>(),
  stepLists: new Map<string, Array<{ steps: unknown; moveTargetSteps: unknown }>>(),
}));
const stableCallbacks = vi.hoisted(() => ({
  isCollapsed: () => false,
  toggleCollapse: vi.fn(),
  onToggleStepVisibility: vi.fn(),
  onToggleAutoHideEmpty: vi.fn(),
  onWorkflowChange: vi.fn(),
}));
const responsive = vi.hoisted(() => ({ isMobile: false, isTablet: false }));

vi.mock("@/hooks/use-responsive-breakpoint", () => ({
  useResponsiveBreakpoint: () => responsive,
}));

vi.mock("@/hooks/use-kanban-display-settings", () => ({
  useKanbanDisplaySettings: () => ({
    onToggleStepVisibility: stableCallbacks.onToggleStepVisibility,
    onToggleAutoHideEmpty: stableCallbacks.onToggleAutoHideEmpty,
  }),
}));

vi.mock("@/hooks/domains/kanban/use-swimlane-collapse", () => ({
  useSwimlaneCollapse: () => ({
    isCollapsed: stableCallbacks.isCollapsed,
    toggleCollapse: stableCallbacks.toggleCollapse,
  }),
}));

vi.mock("@/lib/api", () => ({ reorderWorkflows: vi.fn() }));

vi.mock("@/lib/kanban/view-registry", () => {
  const CountedView = ({
    workflowId,
    steps,
    moveTargetSteps,
    tasks,
  }: {
    workflowId: string;
    steps: unknown;
    moveTargetSteps: unknown;
    tasks: Array<{ id: string }>;
  }) => {
    renderCounts.lanes.set(workflowId, (renderCounts.lanes.get(workflowId) ?? 0) + 1);
    const stepLists = renderCounts.stepLists.get(workflowId) ?? [];
    stepLists.push({ steps, moveTargetSteps });
    renderCounts.stepLists.set(workflowId, stepLists);
    for (const task of tasks) {
      renderCounts.cards.set(task.id, (renderCounts.cards.get(task.id) ?? 0) + 1);
    }
    return <div data-testid={`workflow-${workflowId}`} />;
  };
  const view = {
    id: "kanban",
    component: CountedView,
  };
  return { getEffectiveView: () => view };
});

import { StateProvider, useAppStoreApi } from "@/components/state-provider";
import { defaultState } from "@/lib/state/default-state";
import { SwimlaneContainer } from "./swimlane-container";

const WORKSPACE_ID = "workspace-1";
const WORKFLOW_A = "workflow-a";
const WORKFLOW_B = "workflow-b";
const STORE_CAPTURE_ERROR = "Store was not captured";
const UPDATED_TASK_TITLE = "Updated task";

function task(id: string, workflowId: string, workflowStepId: string) {
  return {
    id,
    workflowId,
    workflowStepId,
    title: id,
    position: 0,
  };
}

function snapshot(workflowId: string, taskId: string) {
  return {
    workflowId,
    workflowName: workflowId,
    steps: [{ id: `${workflowId}-step`, title: "Todo", color: "bg-blue-500", position: 0 }],
    tasks: [task(taskId, workflowId, `${workflowId}-step`)],
  };
}

function StoreCapture({ holder }: { holder: { current: StoreApi<AppState> | null } }) {
  holder.current = useAppStoreApi();
  return null;
}

function renderSwimlanes(
  holder: { current: StoreApi<AppState> | null },
  matchesPluginTaskFilters?: (taskId: string) => boolean,
) {
  return render(
    <StateProvider
      initialState={{
        workspaces: { ...defaultState.workspaces, activeId: WORKSPACE_ID },
        workflows: {
          activeId: null,
          items: [
            { id: WORKFLOW_A, workspaceId: WORKSPACE_ID, name: "Workflow A" },
            { id: WORKFLOW_B, workspaceId: WORKSPACE_ID, name: "Workflow B" },
          ],
        },
        kanbanMulti: {
          isLoading: false,
          snapshots: {
            [WORKFLOW_A]: snapshot(WORKFLOW_A, "task-a"),
            [WORKFLOW_B]: snapshot(WORKFLOW_B, "task-b"),
          },
        },
        repositories: { ...defaultState.repositories, itemsByWorkspaceId: {} },
        userSettings: {
          ...defaultState.userSettings,
          hiddenWorkflowStepIds: {},
          workflowIdsWithAutoHideEmptySteps: [],
        },
      }}
    >
      <StoreCapture holder={holder} />
      <SwimlaneContainer
        viewMode="graph2"
        workflowFilter={null}
        onPreviewTask={vi.fn()}
        onOpenTask={vi.fn()}
        onEditTask={vi.fn()}
        onDeleteTask={vi.fn()}
        onWorkflowChange={stableCallbacks.onWorkflowChange}
        matchesPluginTaskFilters={matchesPluginTaskFilters}
      />
    </StateProvider>,
  );
}

beforeEach(() => {
  renderCounts.lanes.clear();
  renderCounts.cards.clear();
  renderCounts.stepLists.clear();
  responsive.isMobile = false;
  responsive.isTablet = false;
});

afterEach(cleanup);

describe("SwimlaneContainer render isolation", () => {
  it("does not rerender an unaffected workflow lane or card after another workflow task updates", () => {
    const holder: { current: StoreApi<AppState> | null } = { current: null };
    renderSwimlanes(holder);

    const store = holder.current;
    if (!store) throw new Error(STORE_CAPTURE_ERROR);
    const currentTask = store.getState().kanbanMulti.snapshots[WORKFLOW_A].tasks[0];

    act(() => {
      store.getState().updateMultiTask(WORKFLOW_A, { ...currentTask, title: UPDATED_TASK_TITLE });
    });

    expect({
      affectedLane: renderCounts.lanes.get(WORKFLOW_A),
      affectedCard: renderCounts.cards.get("task-a"),
      unaffectedLane: renderCounts.lanes.get(WORKFLOW_B),
      unaffectedCard: renderCounts.cards.get("task-b"),
    }).toEqual({ affectedLane: 2, affectedCard: 2, unaffectedLane: 1, unaffectedCard: 1 });
  });

  it("does not rerender the focused mobile workflow when another workflow task updates", () => {
    responsive.isMobile = true;
    const holder: { current: StoreApi<AppState> | null } = { current: null };
    renderSwimlanes(holder);

    const store = holder.current;
    if (!store) throw new Error(STORE_CAPTURE_ERROR);
    const currentTask = store.getState().kanbanMulti.snapshots[WORKFLOW_B].tasks[0];

    act(() => {
      store.getState().updateMultiTask(WORKFLOW_B, { ...currentTask, title: UPDATED_TASK_TITLE });
    });

    expect({
      focusedLane: renderCounts.lanes.get(WORKFLOW_A),
      focusedCard: renderCounts.cards.get("task-a"),
    }).toEqual({ focusedLane: 1, focusedCard: 1 });
  });

  it("does not recompute another workflow's task projection", () => {
    const matchesPluginTaskFilters = vi.fn((taskId: string) => taskId.length > 0);
    const holder: { current: StoreApi<AppState> | null } = { current: null };
    renderSwimlanes(holder, matchesPluginTaskFilters);

    const unaffectedCallsBefore = matchesPluginTaskFilters.mock.calls.filter(
      ([taskId]) => taskId === "task-b",
    ).length;
    const store = holder.current;
    if (!store) throw new Error(STORE_CAPTURE_ERROR);
    const currentTask = store.getState().kanbanMulti.snapshots[WORKFLOW_A].tasks[0];

    act(() => {
      store.getState().updateMultiTask(WORKFLOW_A, { ...currentTask, title: UPDATED_TASK_TITLE });
    });

    expect(
      matchesPluginTaskFilters.mock.calls.filter(([taskId]) => taskId === "task-b").length,
    ).toBe(unaffectedCallsBefore);
  });

  it("preserves step-list identities for a task-only update", () => {
    const holder: { current: StoreApi<AppState> | null } = { current: null };
    renderSwimlanes(holder);

    const store = holder.current;
    if (!store) throw new Error(STORE_CAPTURE_ERROR);
    const currentTask = store.getState().kanbanMulti.snapshots[WORKFLOW_A].tasks[0];

    act(() => {
      store.getState().updateMultiTask(WORKFLOW_A, { ...currentTask, title: UPDATED_TASK_TITLE });
    });

    const [initial, updated] = renderCounts.stepLists.get(WORKFLOW_A) ?? [];
    expect(updated?.steps).toBe(initial?.steps);
    expect(updated?.moveTargetSteps).toBe(initial?.moveTargetSteps);
  });

  it("evicts a cached projection when its workflow leaves the snapshot map", () => {
    const matchesPluginTaskFilters = vi.fn((taskId: string) => taskId.length > 0);
    const holder: { current: StoreApi<AppState> | null } = { current: null };
    renderSwimlanes(holder, matchesPluginTaskFilters);

    const store = holder.current;
    if (!store) throw new Error(STORE_CAPTURE_ERROR);
    const snapshotB = store.getState().kanbanMulti.snapshots[WORKFLOW_B];
    const callsBeforeRemoval = matchesPluginTaskFilters.mock.calls.filter(
      ([taskId]) => taskId === "task-b",
    ).length;

    act(() => {
      store.setState((state) => ({
        kanbanMulti: {
          ...state.kanbanMulti,
          snapshots: { [WORKFLOW_A]: state.kanbanMulti.snapshots[WORKFLOW_A] },
        },
      }));
    });
    act(() => {
      store.setState((state) => ({
        kanbanMulti: {
          ...state.kanbanMulti,
          snapshots: { ...state.kanbanMulti.snapshots, [WORKFLOW_B]: snapshotB },
        },
      }));
    });

    const callsAfterRestore = matchesPluginTaskFilters.mock.calls.filter(
      ([taskId]) => taskId === "task-b",
    ).length;
    expect(callsAfterRestore - callsBeforeRemoval).toBe(4);
  });
});
