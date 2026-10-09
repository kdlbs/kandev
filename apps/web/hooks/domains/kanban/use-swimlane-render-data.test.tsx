import { act, renderHook } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { StateProvider, useAppStoreApi } from "@/components/state-provider";
import type { WorkflowSnapshotData } from "@/lib/state/slices/kanban/types";
import { useSwimlaneRenderData, useWorkflowSwimlaneData } from "./use-swimlane-render-data";
import type { Repository } from "@/lib/types/http";

const REPOSITORIES: string[] = [];
const FIRST_TASK_ID = "first-task";

function snapshot(id: string): WorkflowSnapshotData {
  return {
    workflowId: id,
    workflowName: id,
    steps: [{ id: `${id}-step`, title: "Ready", color: "blue", position: 0 }],
    tasks: [
      { id: `${id}-task`, title: id, workflowId: id, workflowStepId: `${id}-step`, position: 0 },
    ],
  };
}

function renderOverview() {
  return renderHook(
    () => ({ store: useAppStoreApi(), data: useSwimlaneRenderData(null, REPOSITORIES, "") }),
    {
      wrapper: ({ children }) => <StateProvider>{children}</StateProvider>,
    },
  );
}

// @covers AC-UI-TASK-NAVIGATION-RESPONSIVENESS-001.1
describe("overview snapshot hydration", () => {
  it("renders a missing snapshot beside a loaded workflow", () => {
    const { result } = renderOverview();
    act(() => {
      result.current.store.setState((state) => ({
        kanbanMulti: { ...state.kanbanMulti, snapshots: { loaded: snapshot("loaded") } },
      }));
    });

    expect(result.current.data.getFilteredTasks("loaded").map((task) => task.id)).toEqual([
      "loaded-task",
    ]);
    expect(result.current.data.getFilteredTasks("unloaded")).toEqual([]);
  });

  it("keeps filters scoped as another snapshot arrives and is removed", () => {
    const { result } = renderOverview();
    act(() => {
      result.current.store.setState((state) => ({
        kanbanMulti: { ...state.kanbanMulti, snapshots: { first: snapshot("first") } },
        userSettings: {
          ...state.userSettings,
          hiddenWorkflowStepIds: { first: ["first-step"] },
        },
      }));
    });
    expect(result.current.data.getFilteredTasks("first")).toEqual([]);
    expect(result.current.data.getFilteredTasks("second")).toEqual([]);

    act(() => {
      result.current.store.setState((state) => ({
        kanbanMulti: {
          ...state.kanbanMulti,
          snapshots: { ...state.kanbanMulti.snapshots, second: snapshot("second") },
        },
      }));
    });
    expect(result.current.data.getFilteredTasks("second").map((task) => task.id)).toEqual([
      "second-task",
    ]);
    expect(result.current.data.getFilteredTasks("first")).toEqual([]);

    act(() => {
      result.current.store.setState((state) => ({
        kanbanMulti: { ...state.kanbanMulti, snapshots: { first: snapshot("first") } },
        userSettings: { ...state.userSettings, hiddenWorkflowStepIds: {} },
      }));
    });
    expect(result.current.data.getFilteredTasks("second")).toEqual([]);
    expect(result.current.data.getFilteredTasks("first").map((task) => task.id)).toEqual([
      FIRST_TASK_ID,
    ]);
  });

  it("reuses the projection while its inputs are unchanged", () => {
    const { result, rerender } = renderOverview();
    act(() => {
      result.current.store.setState((state) => ({
        kanbanMulti: { ...state.kanbanMulti, snapshots: { first: snapshot("first") } },
      }));
    });
    const tasks = result.current.data.getFilteredTasks("first");
    rerender();
    expect(result.current.data.getFilteredTasks("first")).toBe(tasks);
  });
});

// @covers AC-UI-BOARD-REPOSITORY-MATCHING-001.5, AC-UI-BOARD-REPOSITORY-MATCHING-001.6, AC-UI-BOARD-REPOSITORY-MATCHING-001.7
describe("swimlane repository projections", () => {
  it("retains secondary membership across workflows and focused occupancy while search changes", () => {
    const selected = ["web"];
    const repoFilter = new Set(selected);
    const { result, rerender } = renderHook(
      ({ searchQuery }) => ({
        store: useAppStoreApi(),
        overview: useSwimlaneRenderData(null, selected, searchQuery),
        focused: useWorkflowSwimlaneData("first", repoFilter, searchQuery),
      }),
      {
        initialProps: { searchQuery: "" },
        wrapper: ({ children }) => <StateProvider>{children}</StateProvider>,
      },
    );
    const first = snapshot("first");
    first.tasks[0].repositoryId = "backend";
    first.tasks[0].repositories = [
      { id: "backend-link", repository_id: "backend", base_branch: "main", position: 0 },
      { id: "web-link", repository_id: "web", base_branch: "main", position: 1 },
    ];
    const second = snapshot("second");
    second.tasks[0].repositoryId = "web";
    act(() => {
      result.current.store.setState((state) => ({
        kanbanMulti: { ...state.kanbanMulti, snapshots: { first, second } },
        workflows: {
          ...state.workflows,
          items: [
            { id: "first", name: "First", workspaceId: "workspace" },
            { id: "second", name: "Second", workspaceId: "workspace" },
          ],
        },
        repositories: {
          ...state.repositories,
          itemsByWorkspaceId: {
            workspace: [
              { id: "backend", name: "Server" },
              { id: "web", name: "Client" },
            ] as Repository[],
          },
        },
        userSettings: { ...state.userSettings, workflowIdsWithAutoHideEmptySteps: ["first"] },
      }));
    });
    expect(result.current.overview.getFilteredTasks("first").map((task) => task.id)).toEqual([
      FIRST_TASK_ID,
    ]);
    expect(result.current.overview.getFilteredTasks("second").map((task) => task.id)).toEqual([
      "second-task",
    ]);
    expect(
      result.current.overview.workflowOptions.map(({ id, taskCount }) => ({ id, taskCount })),
    ).toEqual([
      { id: "first", taskCount: 1 },
      { id: "second", taskCount: 1 },
    ]);
    expect(result.current.focused.tasks.map((task) => task.id)).toEqual([FIRST_TASK_ID]);
    expect(result.current.focused.autoHideEmpty).toBe(true);
    expect(result.current.focused.occupancyTasks.map((task) => task.workflowStepId)).toEqual([
      "first-step",
    ]);
    rerender({ searchQuery: "absent" });
    expect(result.current.overview.getFilteredTasks("first")).toEqual([]);
    expect(result.current.focused.tasks).toEqual([]);
    expect(result.current.focused.occupancyTasks.map((task) => task.id)).toEqual([FIRST_TASK_ID]);
  });
});

// @covers AC-UI-BOARD-REPOSITORY-MATCHING-001.4, AC-UI-BOARD-REPOSITORY-MATCHING-001.5
it.each(["CLIENT-UI", "/projects/client"])(
  "searches secondary repository metadata in live swimlanes: %s",
  (searchQuery) => {
    const selected = ["web"];
    const repoFilter = new Set(selected);
    const { result } = renderHook(
      () => ({
        store: useAppStoreApi(),
        overview: useSwimlaneRenderData(null, selected, searchQuery),
        focused: useWorkflowSwimlaneData("first", repoFilter, searchQuery),
      }),
      { wrapper: ({ children }) => <StateProvider>{children}</StateProvider> },
    );
    const first = snapshot("first");
    first.tasks[0].repositoryId = "backend";
    first.tasks[0].repositories = [
      { id: "backend-link", repository_id: "backend", base_branch: "main", position: 0 },
      { id: "web-link", repository_id: "web", base_branch: "main", position: 1 },
    ];
    const second = snapshot("second");
    second.tasks[0].repositoryId = "web";
    const metadata = [
      { id: "web", name: "client-ui", local_path: "/projects/client" },
    ] as Repository[];
    act(() => {
      result.current.store.setState((state) => ({
        kanbanMulti: { ...state.kanbanMulti, snapshots: { first, second } },
        workflows: {
          ...state.workflows,
          items: [
            { id: "first", name: "First", workspaceId: "active" },
            { id: "second", name: "Second", workspaceId: "other" },
          ],
        },
        repositories: { ...state.repositories, itemsByWorkspaceId: { other: metadata } },
      }));
    });
    expect(result.current.overview.getFilteredTasks("first")).toEqual([]);
    expect(result.current.focused.tasks).toEqual([]);
    expect(result.current.overview.getFilteredTasks("second").map((t) => t.id)).toEqual([
      "second-task",
    ]);
    expect(result.current.focused.occupancyTasks.map((t) => t.id)).toEqual([FIRST_TASK_ID]);
    act(() => {
      result.current.store.setState((state) => ({
        repositories: {
          ...state.repositories,
          itemsByWorkspaceId: { active: metadata, other: metadata },
        },
      }));
    });
    const matching = result.current.overview.getFilteredTasks("first");
    expect(matching.map((t) => t.id)).toEqual([FIRST_TASK_ID]);
    expect(result.current.focused.tasks.map((t) => t.id)).toEqual([FIRST_TASK_ID]);
    act(() => {
      result.current.store.setState((state) => ({
        repositories: {
          ...state.repositories,
          itemsByWorkspaceId: {
            active: [{ id: "web", name: "renamed", local_path: "/moved" }] as Repository[],
            other: metadata,
          },
        },
      }));
    });
    expect(result.current.overview.getFilteredTasks("first")).toEqual([]);
    expect(result.current.focused.tasks).toEqual([]);
    act(() => {
      result.current.store.setState((state) => ({
        repositories: {
          ...state.repositories,
          itemsByWorkspaceId: { active: [], other: metadata },
        },
      }));
    });
    expect(result.current.overview.getFilteredTasks("first")).toEqual([]);
    expect(result.current.focused.tasks).toEqual([]);
    expect(result.current.focused.occupancyTasks.map((t) => t.id)).toEqual([FIRST_TASK_ID]);
    act(() => {
      result.current.store.setState((state) => ({
        workflows: {
          ...state.workflows,
          items: state.workflows.items.map((workflow) => ({ ...workflow, workspaceId: "other" })),
        },
      }));
    });
    expect(result.current.overview.getFilteredTasks("first").map((t) => t.id)).toEqual([
      FIRST_TASK_ID,
    ]);
    expect(result.current.focused.tasks.map((t) => t.id)).toEqual([FIRST_TASK_ID]);
    act(() => {
      result.current.store.setState((state) => ({ workflows: { ...state.workflows, items: [] } }));
    });
    expect(result.current.overview.getFilteredTasks("first")).toEqual([]);
    expect(result.current.focused.tasks).toEqual([]);
  },
);
