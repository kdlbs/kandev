import { describe, expect, it } from "vitest";
import { createAppStore } from "../store";
import { toKanbanTask } from "@/lib/kanban/map-task";
import { coveredTaskOverviews } from "./task-overview-coverage";
import { DEFAULT_VIEW } from "./ui/sidebar-view-builtins";
import type { FilterClause } from "./ui/sidebar-view-types";
import type { TaskCoverage } from "@/lib/types/http";
import { defaultKanbanState } from "./kanban/kanban-slice";

function fixture(total = 1) {
  const task = toKanbanTask({
    id: "task",
    workspace_id: "workspace",
    workflow_id: "workflow",
    workflow_step_id: "step",
    title: "Task",
    created_at: "2026-09-29T00:00:00Z",
    updated_at: "2026-09-29T00:00:00Z",
  });
  const store = createAppStore({
    workspaces: { activeId: "workspace", items: [] },
    workflows: {
      items: [{ id: "workflow", workspaceId: "workspace", name: "Workflow" }],
      activeId: "workflow",
      taskWorkflowCoverage: {
        workspace_id: "workspace",
        workflow_ids: ["workflow"],
        complete: true,
      },
    },
    kanbanMulti: {
      ...defaultKanbanState.kanbanMulti,
      snapshots: {
        workflow: {
          workflowId: "workflow",
          workflowName: "Workflow",
          steps: [],
          tasks: total ? [task] : [],
          taskCoverage: {
            workspace_id: "workspace",
            workflow_id: "workflow",
            membership: "active",
            total,
            complete: true,
            ordering_profile: "sqlite_nocase_v1",
          },
        },
      },
    },
  });
  store.setState((state) => {
    state.repositories.itemsByWorkspaceId.workspace = [];
  });
  return store;
}

describe("authoritative overview coverage", () => {
  it.each([0, 1])("accepts complete boot coverage with %i tasks", (total) => {
    expect(coveredTaskOverviews(fixture(total).getState(), "workspace", DEFAULT_VIEW)).toHaveLength(
      total,
    );
  });
  it.each<Partial<TaskCoverage>>([
    { complete: false },
    { total: 2 },
    { ordering_profile: "server_only" },
    { ordering_profile: "future" },
    { workspace_id: "other" },
    { workflow_id: "other" },
  ])("rejects incomplete or unsupported coverage %j", (patch) => {
    const store = fixture();
    store.setState((state) => {
      Object.assign(state.kanbanMulti.snapshots.workflow.taskCoverage!, patch);
    });
    expect(coveredTaskOverviews(store.getState(), "workspace", DEFAULT_VIEW)).toBeNull();
  });
  it("does not infer completeness from older-server data or missing projection fields", () => {
    const store = fixture();
    store.setState((state) => {
      delete state.kanbanMulti.snapshots.workflow.taskCoverage;
    });
    expect(coveredTaskOverviews(store.getState(), "workspace", DEFAULT_VIEW)).toBeNull();
    const missing = fixture();
    missing.setState((state) => {
      delete state.taskOverview.byId.task.statusSummary;
    });
    expect(
      coveredTaskOverviews(missing.getState(), "workspace", {
        ...DEFAULT_VIEW,
        sort: { key: "lastActivityAt", direction: "desc" },
      }),
    ).toBeNull();
  });
  it("requires the task-wide running field before locally evaluating Running sort", () => {
    const store = fixture();
    const view = { ...DEFAULT_VIEW, sort: { key: "running", direction: "desc" } as const };
    expect(coveredTaskOverviews(store.getState(), "workspace", view)).toBeNull();

    store.setState((state) => {
      state.kanbanMulti.snapshots.workflow.tasks[0].statusSummary = {
        revision: 1,
        updated_at: "2026-09-29T00:00:00Z",
        has_running_session: false,
      };
    });
    expect(coveredTaskOverviews(store.getState(), "workspace", view)).toHaveLength(1);
  });
  it("requires hidden active workflows but allows an explicit covered workflow restriction", () => {
    const store = fixture();
    store.setState((state) => {
      state.workflows.taskWorkflowCoverage!.workflow_ids.push("hidden");
    });
    expect(coveredTaskOverviews(store.getState(), "workspace", DEFAULT_VIEW)).toBeNull();
    expect(
      coveredTaskOverviews(store.getState(), "workspace", {
        ...DEFAULT_VIEW,
        filters: [{ id: "workflow", dimension: "workflow", op: "in", value: ["workflow"] }],
      }),
    ).toHaveLength(1);
  });
  it("keeps archives on the server path for every boolean filter spelling", () => {
    const state = fixture().getState();
    const filters: Array<Pick<FilterClause, "op" | "value">> = [
      { op: "is", value: true },
      { op: "is_not", value: false },
      { op: "in", value: ["true", "false"] },
      { op: "not_in", value: ["false"] },
    ];
    for (const filter of filters) {
      expect(
        coveredTaskOverviews(state, "workspace", {
          ...DEFAULT_VIEW,
          filters: [
            {
              id: "archive",
              dimension: "archived",
              op: filter.op,
              value: filter.value,
            },
          ],
        }),
      ).toBeNull();
    }
  });
  it("invalidates complete coverage across reconnect gaps and context disposal", () => {
    const store = fixture();
    store.setState((state) => {
      state.connection.status = "connected";
    });
    store.setState((state) => {
      state.connection.status = "disconnected";
    });
    expect(coveredTaskOverviews(store.getState(), "workspace", DEFAULT_VIEW)).toBeNull();
    store.getState().resetKanbanWorkspaceContext();
    expect(store.getState().taskOverview.byId).toEqual({});
  });
});

it("requires a fresh scope inventory and snapshot after a reconnect gap", () => {
  const store = fixture();
  const before = store.getState();
  store.getState().setConnectionStatus("connected");
  store.getState().setConnectionStatus("reconnecting");
  expect(store.getState().workflows.taskWorkflowCoverage?.complete).toBe(false);
  const read = store.getState().beginTaskOverviewRead();
  store.getState().setWorkflows(before.workflows.items, before.workflows.taskWorkflowCoverage);
  store.getState().setWorkflowSnapshot("workflow", before.kanbanMulti.snapshots.workflow);
  expect(coveredTaskOverviews(store.getState(), "workspace", DEFAULT_VIEW)).toBeNull();
  store.getState().setConnectionStatus("connected");
  expect(store.getState().taskOverview.reads[read]).toBeUndefined();
  store.getState().setWorkflowSnapshot("workflow", before.kanbanMulti.snapshots.workflow);
  expect(coveredTaskOverviews(store.getState(), "workspace", DEFAULT_VIEW)).toBeNull();
  store.getState().setWorkflows(before.workflows.items, before.workflows.taskWorkflowCoverage);
  expect(coveredTaskOverviews(store.getState(), "workspace", DEFAULT_VIEW)).toHaveLength(1);
});
