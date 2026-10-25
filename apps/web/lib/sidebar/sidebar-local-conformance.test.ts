import { expect, it } from "vitest";
import fixtures from "../../../backend/internal/task/repository/testdata/sidebar-local-conformance.json";
import { createAppStore } from "@/lib/state/store";
import { toKanbanTask, type TaskLike } from "@/lib/kanban/map-task";
import type { SidebarTaskQuery } from "@/lib/types/http";
import { localSidebarPage } from "./sidebar-local-view";
import { projectLocalSidebarTasks } from "./sidebar-local-projection";

type FixtureCase = {
  name: string;
  query: SidebarTaskQuery;
  preferences?: {
    pinned_task_ids?: string[];
    ordered_task_ids?: string[];
    subtask_order_by_parent_id?: Record<string, string[]>;
  };
  expected_task_ids: string[];
  expected_group_keys?: string[];
  expected_queues?: Record<string, [number, number]>;
  expected_depths?: Record<string, number>;
  expected_subtask_counts?: Record<string, number>;
  expected_continuation?: string;
  expected_total_visible?: number;
};
type FixtureTask = TaskLike & {
  summary?: TaskLike["status_summary"];
  repository_ids?: string[];
  executor_type?: string;
};

function projectScenario(tasks: FixtureTask[]) {
  const store = createAppStore();
  store.setState((state) => {
    state.repositories.itemsByWorkspaceId.conformance = fixtures.repositories.map((repo) => ({
      ...repo,
      workspace_id: "conformance",
    })) as (typeof state.repositories.itemsByWorkspaceId)[string];
    state.workflows.items = fixtures.workflows.map((wf) => ({
      id: wf.id,
      name: wf.name,
      workspaceId: "conformance",
    }));
    for (const wf of fixtures.workflows)
      state.kanbanMulti.snapshots[wf.id] = {
        workflowId: wf.id,
        workflowName: wf.name,
        tasks: [],
        steps: [{ id: wf.step_id, title: wf.step_id, color: "", position: 0 }],
      };
  });
  return projectLocalSidebarTasks(
    tasks.map((task) =>
      toKanbanTask({
        ...task,
        status_summary: task.summary,
        primary_executor_type: task.executor_type,
        repositories: (task.repository_ids ?? []).map((id, position) => ({
          id: `${task.id}-${id}`,
          repository_id: id,
          position,
        })),
      }),
    ),
    store.getState(),
    "conformance",
  );
}

function assertRowMetadata(page: ReturnType<typeof localSidebarPage>, fixture: FixtureCase) {
  for (const [id, queue] of Object.entries(fixture.expected_queues ?? {})) {
    const row = page.entries.find((row) => row.task_id === id)!;
    expect([row.wip_queue_position, row.wip_queue_total]).toEqual(queue);
  }
  for (const [id, depth] of Object.entries(fixture.expected_depths ?? {}))
    expect(page.entries.find((row) => row.task_id === id)?.depth ?? 0).toBe(depth);
  for (const [id, count] of Object.entries(fixture.expected_subtask_counts ?? {}))
    expect(page.entries.find((row) => row.task_id === id)?.subtask_count ?? 0).toBe(count);
}

for (const scenario of fixtures.scenarios) {
  it.each(scenario.cases as FixtureCase[])(
    `${scenario.name}: $name agrees with actual SQLite`,
    (fixture) => {
      const tasks = projectScenario(scenario.tasks as FixtureTask[]);
      const preferences = fixture.preferences ?? {};
      const page = localSidebarPage(tasks, fixture.query, {
        pinnedTaskIds: preferences.pinned_task_ids ?? [],
        orderedTaskIds: preferences.ordered_task_ids ?? [],
        subtaskOrderByParentId: preferences.subtask_order_by_parent_id ?? {},
      });
      expect(page.entries.flatMap((row) => (row.task_id ? [row.task_id] : []))).toEqual(
        fixture.expected_task_ids,
      );
      if (fixture.expected_group_keys)
        expect(
          page.entries.filter((row) => row.kind === "group").map((row) => row.group_key),
        ).toEqual(fixture.expected_group_keys);
      assertRowMetadata(page, fixture);
      if (fixture.expected_continuation)
        expect(
          page.entries.filter((row) => row.kind === "continuation").map((row) => row.parent_id),
        ).toContain(fixture.expected_continuation);
      if (fixture.expected_total_visible !== undefined)
        expect(page.total_visible_tasks).toBe(fixture.expected_total_visible);
    },
  );
}

it("does not infer running when the summary omits primary_session", () => {
  const baselineTime = "2026-10-01T00:00:00Z";
  const tasks = projectScenario([
    {
      id: "parent",
      title: "parent",
      workspace_id: "conformance",
      workflow_id: "wf-a",
      workflow_step_id: "step-a",
      state: "TODO",
      updated_at: baselineTime,
      created_at: baselineTime,
      summary: {
        revision: 1,
        updated_at: baselineTime,
        last_activity_at: baselineTime,
      },
    },
    {
      id: "running-child",
      title: "running child",
      parent_id: "parent",
      workspace_id: "conformance",
      workflow_id: "wf-a",
      workflow_step_id: "step-a",
      state: "TODO",
      primary_session_state: "RUNNING",
      updated_at: "2026-10-04T00:00:00Z",
      created_at: baselineTime,
      summary: {
        revision: 1,
        updated_at: "2026-10-04T00:00:00Z",
        last_activity_at: "2026-10-04T00:00:00Z",
      },
    },
    {
      id: "running-root",
      title: "running root",
      workspace_id: "conformance",
      workflow_id: "wf-a",
      workflow_step_id: "step-a",
      state: "TODO",
      primary_session_state: "RUNNING",
      updated_at: "2026-10-03T00:00:00Z",
      created_at: baselineTime,
      summary: {
        revision: 1,
        updated_at: "2026-10-03T00:00:00Z",
        last_activity_at: "2026-10-03T00:00:00Z",
        primary_session: { id: "running-root-session", state: "RUNNING" },
      },
    },
  ]);

  const page = localSidebarPage(
    tasks,
    {
      filters: [],
      sort: {
        key: "running",
        direction: "desc",
        then_by: [{ key: "lastActivityAt", direction: "desc" }],
      },
      group: "none",
      collapsed_group_keys: [],
      collapsed_task_ids: [],
      page: 1,
      page_size: 100,
      locale: "en",
    },
    { pinnedTaskIds: [], orderedTaskIds: [], subtaskOrderByParentId: {} },
  );

  expect(page.entries.flatMap((entry) => (entry.task_id ? [entry.task_id] : []))).toEqual([
    "running-root",
    "parent",
    "running-child",
  ]);
});
