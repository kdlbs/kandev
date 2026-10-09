import { beforeEach, describe, expect, it, vi } from "vitest";
import { querySidebarTasks } from "@/lib/api/domains/kanban-api";
import { createAppStore } from "@/lib/state/store";
import { defaultKanbanState } from "@/lib/state/slices/kanban/kanban-slice";
import { recordTaskOverviewChange } from "@/lib/state/slices/task-overview-merge";
import { toKanbanTask } from "@/lib/kanban/map-task";
import type { BackendMessageMap } from "@/lib/types/backend";
import type { SidebarTaskPageResponse, SidebarTaskQuery, Task } from "@/lib/types/http";
import { taskId, workflowId, workspaceId } from "@/lib/types/ids";
import type { TaskStatusSummary } from "@/lib/types/task-status-summary";
import { registerTasksHandlers } from "@/lib/ws/handlers/tasks";
import { SidebarTaskPageCache } from "./sidebar-task-page-cache";

vi.mock("@/lib/api/domains/kanban-api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/lib/api/domains/kanban-api")>()),
  querySidebarTasks: vi.fn(),
}));

const WORKSPACE = "archived-freshness-workspace";
const WORKFLOW = "archived-freshness-workflow";
const TASK = "archived-freshness-task";
const VIEW = "archived-freshness-view";
const ARCHIVED_AT = "2026-10-07T12:00:00Z";
const HTTP_TIME = "2026-10-08T12:00:00.123456787Z";
const OLDER_EVENT_TIME = "2026-10-08T12:00:00.123456788Z";
const NEWER_EVENT_TIME = "2026-10-08T12:00:00.123456789Z";
const NEWER_HTTP_TIME = "2026-10-08T12:00:00.123456790Z";
const DISPLAY_OWNER = "sidebar:freshness-display";
const LATEST_TITLE = "Latest archived title";

const query: SidebarTaskQuery = {
  filters: [{ dimension: "archived", op: "is", value: true }],
  sort: { key: "title", direction: "asc" },
  group: "none",
  collapsed_group_keys: [],
  collapsed_task_ids: [],
  page: 1,
  page_size: 100,
  locale: "en",
};

function olderArchivedPage(overrides: Partial<Task> = {}): SidebarTaskPageResponse {
  const task: Task = {
    id: taskId(TASK),
    workspace_id: workspaceId(WORKSPACE),
    workflow_id: workflowId(WORKFLOW),
    workflow_step_id: "step",
    title: "HTTP archived title",
    description: "",
    position: 0,
    state: "TODO",
    priority: "medium",
    created_at: "2026-10-07T00:00:00Z",
    updated_at: HTTP_TIME,
    archived_at: ARCHIVED_AT,
    ...overrides,
  };
  return {
    query_key: VIEW,
    page: 1,
    page_size: 100,
    total_tasks: 1,
    total_visible_tasks: 1,
    total_entries: 1,
    has_next: false,
    has_previous: false,
    entries: [{ kind: "task", task_id: TASK, task }],
  };
}

function archivedRename(title: string, updatedAt: string): BackendMessageMap["task.updated"] {
  return {
    type: "notification",
    action: "task.updated",
    payload: {
      task_id: TASK,
      workspace_id: WORKSPACE,
      workflow_id: WORKFLOW,
      workflow_step_id: "step",
      title,
      updated_at: updatedAt,
      archived_at: ARCHIVED_AT,
      is_ephemeral: false,
    },
  };
}

function startArchivedRead(resident = false) {
  const store = createAppStore({
    workspaces: { activeId: WORKSPACE, items: [] },
    kanban: { workflowId: WORKFLOW, steps: [], tasks: [] },
    kanbanMulti: {
      ...defaultKanbanState.kanbanMulti,
      snapshots: {
        [WORKFLOW]: {
          workflowId: WORKFLOW,
          workflowName: "Workflow",
          steps: [],
          tasks: [],
        },
      },
    },
  });
  if (resident) {
    store
      .getState()
      .retainTaskOverviews(DISPLAY_OWNER, [toKanbanTask(olderArchivedPage().entries[0].task!)]);
  }
  const cache = new SidebarTaskPageCache(store);
  let resolve!: (page: SidebarTaskPageResponse) => void;
  let reject!: (error: Error) => void;
  vi.mocked(querySidebarTasks).mockReturnValueOnce(
    new Promise((done, fail) => {
      resolve = done;
      reject = fail;
    }),
  );
  const request = cache.request(WORKSPACE, query, VIEW);
  const handlers = registerTasksHandlers(store);
  return { store, cache, request, resolve, reject, handlers };
}

type Fixture = ReturnType<typeof startArchivedRead>;

async function withArchivedRead(run: (fixture: Fixture) => Promise<void>, resident = false) {
  const fixture = startArchivedRead(resident);
  try {
    await run(fixture);
  } finally {
    fixture.resolve(olderArchivedPage());
    fixture.request.release();
    fixture.cache.clear();
    await fixture.request.promise;
    fixture.store.getState().releaseTaskOverviews(DISPLAY_OWNER);
  }
}

function summary(revision: number, queuedPromptCount = 0): TaskStatusSummary {
  return { revision, updated_at: "2030-01-01T00:00:00Z", queued_prompt_count: queuedPromptCount };
}

function dispatchSummary(fixture: Fixture, value: TaskStatusSummary) {
  fixture.handlers["task.status_summary.updated"]!({
    type: "notification",
    action: "task.status_summary.updated",
    payload: { task_id: TASK, workspace_id: WORKSPACE, status_summary: value },
  });
}

beforeEach(() => {
  vi.mocked(querySidebarTasks).mockReset();
});

function registerTitleRegressionTests() {
  // @covers AC-UI-SIDEBAR-ARCHIVED-FILTER-002.20
  it("preserves newest nonresident archived title across reversed updates and older HTTP", async () => {
    const { store, cache, request, resolve, handlers } = startArchivedRead();
    try {
      expect(store.getState().taskOverview.byId[TASK]).toBeUndefined();
      handlers["task.updated"]!(archivedRename(LATEST_TITLE, NEWER_EVENT_TIME));
      handlers["task.updated"]!(archivedRename("Superseded title", OLDER_EVENT_TIME));
      expect(store.getState().taskOverview.byId[TASK]).toBeUndefined();

      resolve(olderArchivedPage());
      const result = await request.promise;
      expect(result.entries.map((entry) => entry.task_id)).toEqual([TASK]);
      const row = store.getState().taskOverview.byId[result.entries[0].task_id!];
      expect(row.title).toBe(LATEST_TITLE);
      expect(row.updatedAt).toBe(NEWER_EVENT_TIME);
      expect(row.isArchived).toBe(true);
      expect(store.getState().kanban.tasks).toEqual([]);
      expect(store.getState().kanbanMulti.snapshots[WORKFLOW].tasks).toEqual([]);
      expect(result.provisional).toBe(true);
      expect(result.entries[0].task).toBeUndefined();
      expect(cache.get(VIEW)).toBeNull();
    } finally {
      resolve(olderArchivedPage());
      request.release();
      cache.clear();
      await request.promise;
    }
    expect(store.getState().taskOverview.reads).toEqual({});
  });

  // @covers AC-UI-SIDEBAR-ARCHIVED-FILTER-002.20
  it("accepts chronological archived updates before older HTTP", async () => {
    const { store, cache, request, resolve, handlers } = startArchivedRead();
    try {
      expect(store.getState().taskOverview.byId[TASK]).toBeUndefined();
      handlers["task.updated"]!(archivedRename("Superseded title", OLDER_EVENT_TIME));
      handlers["task.updated"]!(archivedRename(LATEST_TITLE, NEWER_EVENT_TIME));
      expect(store.getState().taskOverview.byId[TASK]).toBeUndefined();

      resolve(olderArchivedPage());
      const result = await request.promise;
      expect(result.entries.map((entry) => entry.task_id)).toEqual([TASK]);
      const row = store.getState().taskOverview.byId[result.entries[0].task_id!];
      expect(row.title).toBe(LATEST_TITLE);
      expect(row.updatedAt).toBe(NEWER_EVENT_TIME);
      expect(row.isArchived).toBe(true);
      expect(store.getState().kanban.tasks).toEqual([]);
      expect(store.getState().kanbanMulti.snapshots[WORKFLOW].tasks).toEqual([]);
      expect(result.provisional).toBe(true);
      expect(result.entries[0].task).toBeUndefined();
      expect(cache.get(VIEW)).toBeNull();
    } finally {
      resolve(olderArchivedPage());
      request.release();
      cache.clear();
      await request.promise;
    }
    expect(store.getState().taskOverview.reads).toEqual({});
  });
}

function registerResidentAndHttpTests() {
  it.each([false, true])(
    "accepts newer HTTP task fields after archived events (resident=%s)",
    async (resident) => {
      for (const reversed of [false, true]) {
        await withArchivedRead(async ({ store, request, resolve, handlers }) => {
          const events = [
            archivedRename("Older", OLDER_EVENT_TIME),
            archivedRename("Live", NEWER_EVENT_TIME),
          ];
          for (const event of reversed ? events.reverse() : events)
            handlers["task.updated"]!(event);
          resolve(olderArchivedPage({ title: "Newest HTTP", updated_at: NEWER_HTTP_TIME }));
          const page = await request.promise;
          expect(page.entries.map((entry) => entry.task_id)).toEqual([TASK]);
          expect(store.getState().taskOverview.byId[TASK]).toMatchObject({
            title: "Newest HTTP",
            updatedAt: NEWER_HTTP_TIME,
          });
        }, resident);
      }
    },
  );

  it("preserves resident archived title across reversed updates and older HTTP", async () => {
    await withArchivedRead(async ({ store, request, resolve, handlers }) => {
      handlers["task.updated"]!(archivedRename(LATEST_TITLE, NEWER_EVENT_TIME));
      const accepted = store.getState().taskOverview.byId[TASK];
      handlers["task.updated"]!(archivedRename("Superseded title", OLDER_EVENT_TIME));
      expect(store.getState().taskOverview.byId[TASK]).toBe(accepted);
      resolve(olderArchivedPage());
      const page = await request.promise;
      expect(page.entries.map((entry) => entry.task_id)).toEqual([TASK]);
      expect(store.getState().taskOverview.byId[TASK]).toBe(accepted);
      request.release();
      expect(store.getState().taskOverview.byId[TASK]).toBe(accepted);
    }, true);
  });
}

function registerSummaryTests() {
  it.each([false, true])(
    "merges task timestamps and summary revisions independently (resident=%s)",
    async (resident) => {
      for (const reversed of [false, true]) {
        for (const newerHttp of [false, true]) {
          await withArchivedRead(async ({ store, request, resolve, handlers }) => {
            const oldTask = archivedRename("Older task", OLDER_EVENT_TIME);
            oldTask.payload.status_summary = summary(8);
            const newTask = archivedRename("Newer task", NEWER_EVENT_TIME);
            newTask.payload.status_summary = summary(3);
            for (const event of reversed ? [newTask, oldTask] : [oldTask, newTask])
              handlers["task.updated"]!(event);
            resolve(
              olderArchivedPage({
                title: "HTTP task",
                updated_at: newerHttp ? NEWER_HTTP_TIME : HTTP_TIME,
                status_summary: summary(newerHttp ? 9 : 1),
              }),
            );
            await request.promise;
            expect(store.getState().taskOverview.byId[TASK]).toMatchObject({
              title: newerHttp ? "HTTP task" : "Newer task",
              updatedAt: newerHttp ? NEWER_HTTP_TIME : NEWER_EVENT_TIME,
              statusSummary: { revision: newerHttp ? 9 : 8 },
            });
          }, resident);
        }
      }
    },
  );

  it("preserves summary-only updates and equal revision queue fields", async () => {
    await withArchivedRead(async (fixture) => {
      dispatchSummary(fixture, summary(7, 1));
      dispatchSummary(fixture, summary(6, 99));
      const event = archivedRename(LATEST_TITLE, NEWER_EVENT_TIME);
      event.payload.status_summary = summary(7, 2);
      fixture.handlers["task.updated"]!(event);
      dispatchSummary(fixture, summary(5, 99));
      fixture.resolve(olderArchivedPage({ status_summary: summary(1, 99) }));
      await fixture.request.promise;
      expect(fixture.store.getState().taskOverview.byId[TASK]).toMatchObject({
        title: LATEST_TITLE,
        updatedAt: NEWER_EVENT_TIME,
        statusSummary: { revision: 7, queued_prompt_count: 2 },
      });
    });
    await withArchivedRead(async ({ store, request, resolve }) => {
      const current = store.getState().taskOverview.byId[TASK];
      store
        .getState()
        .retainTaskOverviews(DISPLAY_OWNER, [{ ...current, statusSummary: summary(7, 2) }]);
      resolve(olderArchivedPage({ status_summary: summary(7, 3) }));
      await request.promise;
      expect(store.getState().taskOverview.byId[TASK].statusSummary?.queued_prompt_count).toBe(3);
    }, true);
  });
}

function registerPartialAndTimestampTests() {
  it.each([
    ["2026-10-08T13:00:00.123456788+01:00", "First"],
    [NEWER_EVENT_TIME, "Incoming"],
    ["2026-10-08T13:00:00.123456789+01:00", "Incoming"],
    [undefined, "Incoming"],
    ["", "Incoming"],
    ["malformed", "Incoming"],
  ])("preserves nanoseconds offsets and timestamp fallbacks (%s)", async (timestamp, expected) => {
    await withArchivedRead(async ({ store, request, resolve, handlers }) => {
      handlers["task.updated"]!(archivedRename("First", NEWER_EVENT_TIME));
      const event = archivedRename("Incoming", timestamp ?? NEWER_EVENT_TIME);
      if (timestamp === undefined) delete event.payload.updated_at;
      handlers["task.updated"]!(event);
      resolve(olderArchivedPage());
      await request.promise;
      expect(store.getState().taskOverview.byId[TASK].title).toBe(expected);
    });
  });

  it.each([false, true])(
    "preserves omitted partial fields and eligible explicit clears (clear=%s)",
    async (clear) => {
      await withArchivedRead(async ({ store, request, resolve, handlers }) => {
        const first = archivedRename("First", OLDER_EVENT_TIME);
        first.payload.description = "Keep description";
        first.payload.parent_id = "parent";
        handlers["task.updated"]!(first);
        const second = archivedRename("Second", NEWER_EVENT_TIME);
        second.payload.parent_id = null;
        if (clear) second.payload.description = "";
        handlers["task.updated"]!(second);
        const patch = Object.values(store.getState().taskOverview.reads)[0].changes[TASK]!;
        expect(Object.hasOwn(patch, "position")).toBe(false);
        expect(Object.hasOwn(patch, "statusSummary")).toBe(false);
        resolve(olderArchivedPage());
        await request.promise;
        expect(store.getState().taskOverview.byId[TASK]).toMatchObject({
          description: clear ? "" : "Keep description",
          parentTaskId: undefined,
          position: 0,
        });
      });
    },
  );
}

function registerTransportAndDeletionTests() {
  it("keeps empty and failed transports distinct and releases reads", async () => {
    await withArchivedRead(async ({ store, request, resolve, handlers }) => {
      handlers["task.updated"]!(archivedRename("Unseen live task", NEWER_EVENT_TIME));
      resolve({
        ...olderArchivedPage(),
        entries: [],
        total_tasks: 0,
        total_visible_tasks: 0,
        total_entries: 0,
      });
      expect((await request.promise).entries).toEqual([]);
      expect(store.getState().taskOverview.byId[TASK]).toBeUndefined();
    });
    const fixture = startArchivedRead();
    try {
      const error = new Error("Transport failed");
      fixture.reject(error);
      await expect(fixture.request.promise).rejects.toBe(error);
      expect(fixture.cache.get(VIEW)).toBeNull();
    } finally {
      fixture.request.release();
      fixture.cache.clear();
    }
    expect(fixture.store.getState().taskOverview.reads).toEqual({});
    expect(fixture.store.getState().taskOverview.byId).toEqual({});
  });

  it("keeps deletion tombstones through later patches and older HTTP", async () => {
    await withArchivedRead(async ({ store, request, resolve, handlers, cache }) => {
      handlers["task.deleted"]!({
        type: "notification",
        action: "task.deleted",
        payload: archivedRename("Deleted", OLDER_EVENT_TIME).payload,
      });
      handlers["task.updated"]!(archivedRename("Late rename", NEWER_EVENT_TIME));
      resolve(olderArchivedPage());
      expect((await request.promise).entries).toEqual([]);
      expect(store.getState().taskOverview.byId[TASK]).toBeUndefined();
      expect(cache.get(VIEW)).toBeNull();
    });
  });
}

function registerBudgetTests() {
  it.each(["ids", "bytes"] as const)(
    "preserves journal ID and byte limits without accumulating stale arrivals (%s)",
    async (budget) => {
      await withArchivedRead(async ({ store, request, resolve, cache }) => {
        const readId = Object.keys(store.getState().taskOverview.reads)[0];
        const record = (id: string, patch: Parameters<typeof recordTaskOverviewChange>[2]) =>
          store.setState((state) => ({
            taskOverview: recordTaskOverviewChange(state.taskOverview, id, patch),
          }));
        if (budget === "ids") {
          for (let i = 0; i < 1000; i++) record(`deleted-${i}`, null);
          expect(Object.keys(store.getState().taskOverview.reads[readId].changes)).toHaveLength(
            1000,
          );
        } else {
          const fields = { id: TASK, description: "", updatedAt: NEWER_EVENT_TIME };
          const overhead = new TextEncoder().encode(JSON.stringify([TASK, fields])).byteLength;
          record(TASK, { ...fields, description: "x".repeat(1024 * 1024 - overhead) });
          expect(store.getState().taskOverview.reads[readId].bytes).toBe(1024 * 1024);
          record(TASK, {
            id: TASK,
            description: "s".repeat(1024 * 1024),
            updatedAt: OLDER_EVENT_TIME,
          });
          expect(store.getState().taskOverview.reads[readId].bytes).toBe(1024 * 1024);
        }
        const generation = store.getState().taskOverview.generation;
        record(
          budget === "ids" ? "overflow" : TASK,
          budget === "ids"
            ? null
            : { id: TASK, description: "x".repeat(1024 * 1024), updatedAt: NEWER_HTTP_TIME },
        );
        expect(store.getState().taskOverview.generation).toBeGreaterThan(generation);
        expect(
          store
            .getState()
            .retainTaskOverviews(
              DISPLAY_OWNER,
              [toKanbanTask(olderArchivedPage().entries[0].task!)],
              readId,
            ),
        ).toBe(false);
        resolve(olderArchivedPage());
        await request.promise;
        expect(store.getState().taskOverview.byId[TASK]).toBeUndefined();
        expect(cache.get(VIEW)).toBeNull();
      });
    },
  );
}

function registerIsolationTests() {
  it.each(["workspace", "account"] as const)(
    "isolates journals between reads workspaces accounts and stores (%s)",
    async (context) => {
      await withArchivedRead(async ({ store, request, resolve, handlers, cache }) => {
        const foreign = archivedRename("Foreign title", NEWER_HTTP_TIME);
        foreign.payload.workspace_id = "foreign-workspace";
        handlers["task.updated"]!(foreign);
        expect(Object.values(store.getState().taskOverview.reads)[0].changes[TASK]).toBeUndefined();
        handlers["task.updated"]!(archivedRename("Local title", NEWER_EVENT_TIME));
        store.setState((state) =>
          context === "workspace"
            ? { workspaces: { ...state.workspaces, activeId: "other-workspace" } }
            : { auth: { ...state.auth, authenticated: !state.auth.authenticated } },
        );
        resolve(olderArchivedPage());
        await request.promise;
        expect(store.getState().taskOverview.byId[TASK]).toBeUndefined();
        expect(cache.get(VIEW)).toBeNull();
      });
    },
  );

  it("isolates staggered reads and independent task IDs", async () => {
    await withArchivedRead(async (first) => {
      first.handlers["task.updated"]!(archivedRename("First read live", NEWER_EVENT_TIME));
      const firstId = Object.keys(first.store.getState().taskOverview.reads)[0];
      let resolveSecond!: (page: SidebarTaskPageResponse) => void;
      vi.mocked(querySidebarTasks).mockReturnValueOnce(
        new Promise((done) => {
          resolveSecond = done;
        }),
      );
      const second = first.cache.request(WORKSPACE, { ...query, page: 2 }, "second-view");
      try {
        const reads = first.store.getState().taskOverview.reads;
        const secondId = Object.keys(reads).find((id) => id !== firstId)!;
        expect(reads[firstId].changes[TASK]).toBeDefined();
        expect(reads[secondId].changes[TASK]).toBeUndefined();
        const other = archivedRename("Independent", NEWER_EVENT_TIME);
        other.payload.task_id = "other-task";
        first.handlers["task.updated"]!(other);
        expect(
          first.store.getState().taskOverview.reads[secondId].changes["other-task"]?.title,
        ).toBe("Independent");
        first.resolve(olderArchivedPage());
        await first.request.promise;
        resolveSecond({ ...olderArchivedPage(), page: 2 });
        await second.promise;
        expect(first.store.getState().taskOverview.byId[TASK].title).toBe("First read live");
        expect(first.store.getState().taskOverview.byId["other-task"]).toBeUndefined();
      } finally {
        resolveSecond({ ...olderArchivedPage(), page: 2 });
        second.release();
        await second.promise;
      }
    });
  });

  it("isolates separate app stores", async () => {
    await withArchivedRead(async (first) => {
      first.handlers["task.updated"]!(archivedRename("First store live", NEWER_EVENT_TIME));
      await withArchivedRead(async (second) => {
        second.resolve(olderArchivedPage());
        await second.request.promise;
        expect(second.store.getState().taskOverview.byId[TASK].title).toBe("HTTP archived title");
        expect(first.store.getState().taskOverview.byId[TASK]).toBeUndefined();
      });
      first.resolve(olderArchivedPage());
      await first.request.promise;
      expect(first.store.getState().taskOverview.byId[TASK].title).toBe("First store live");
    });
  });
}

describe("archived sidebar update freshness", () => {
  registerTitleRegressionTests();
  registerResidentAndHttpTests();
  registerSummaryTests();
  registerPartialAndTimestampTests();
  registerTransportAndDeletionTests();
  registerBudgetTests();
  registerIsolationTests();
});
