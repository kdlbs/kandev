import type { Task, ListTasksResponse } from "@/lib/types/http";
import type { TaskStatusSummary } from "@/lib/types/task-status-summary";
import type { TaskEventPayload, TaskStatusSummaryUpdatedPayload } from "@/lib/types/backend";
import { ApiError } from "@/lib/api/client";
import { listTasksByWorkspace } from "@/lib/api/domains/kanban-api";
import { isNewerStatusSummary, pickFreshestStatusSummary } from "@/lib/task-status-summary";

export type CoordinatorFilters = {
  query?: string;
  workflowId?: string | null;
  repositoryId?: string | null;
};
type Snapshot = {
  tasks: Task[];
  total: number;
  complete: boolean;
  stale: boolean;
  loading: boolean;
  error: unknown;
};
const initialSnapshot = (): Snapshot => ({
  tasks: [],
  total: 0,
  complete: false,
  stale: false,
  loading: false,
  error: null,
});

/** Page-local observation of canonical tasks. It owns no execution or task mutations. */
export class CoordinatorTaskObservation {
  private snapshot = initialSnapshot();
  private listeners = new Set<() => void>();
  private summaries = new Map<string, TaskStatusSummary>();
  private deleted = new Set<string>();
  private pages = 1;
  private generation = 0;
  private lifecycleRevision = 0;
  private disposed = false;
  private request?: AbortController;
  private timer?: ReturnType<typeof setTimeout>;
  constructor(
    private workspace: string,
    private filters: CoordinatorFilters,
    private loader = listTasksByWorkspace,
  ) {}
  getSnapshot = () => this.snapshot;
  subscribe = (listener: () => void) => {
    this.listeners.add(listener);
    return () => {
      this.listeners.delete(listener);
    };
  };
  private update(patch: Partial<Snapshot>) {
    this.snapshot = { ...this.snapshot, ...patch };
    for (const listener of this.listeners) listener();
  }
  activate = () => {
    this.disposed = false;
  };
  dispose = () => {
    this.disposed = true;
    ++this.generation;
    this.request?.abort();
    clearTimeout(this.timer);
    this.timer = undefined;
    this.summaries.clear();
    this.deleted.clear();
    this.snapshot = initialSnapshot();
  };
  private async readWindow(signal: AbortSignal): Promise<ListTasksResponse> {
    const tasks: Task[] = [];
    let total = 0;
    for (let page = 1; page <= this.pages; page++) {
      const response = await this.loader(
        this.workspace,
        { ...this.filters, page, pageSize: 100, excludeConfig: true, view: "kanban" },
        { init: { signal } },
      );
      tasks.push(...response.tasks);
      total = response.total;
      if (!response.tasks.length || page * 100 >= total) break;
    }
    return { tasks, total };
  }
  private mergeRows(rows: Task[]): Task[] {
    const current = new Map(this.snapshot.tasks.map((task) => [task.id, task]));
    const unique = new Map<string, Task>();
    for (const task of rows) {
      if (task.workspace_id !== this.workspace || this.deleted.has(task.id)) continue;
      const cached = pickFreshestStatusSummary(
        this.summaries.get(task.id),
        current.get(task.id)?.status_summary,
      );
      const summary = pickFreshestStatusSummary(task.status_summary, cached);
      if (summary) this.summaries.set(task.id, summary);
      unique.set(task.id, { ...task, status_summary: summary });
    }
    return [...unique.values()];
  }
  refresh = async () => {
    if (this.disposed) return;
    this.request?.abort();
    const request = new AbortController();
    this.request = request;
    const generation = ++this.generation;
    const revision = this.lifecycleRevision;
    this.update({ loading: true });
    try {
      const response = await this.readWindow(request.signal);
      if (this.disposed || generation !== this.generation) return;
      if (revision !== this.lifecycleRevision) {
        this.scheduleRefresh();
        return;
      }
      const tasks = this.mergeRows(response.tasks);
      this.update({
        tasks,
        total: response.total,
        complete: tasks.length >= response.total,
        stale: false,
        error: null,
      });
    } catch (error) {
      if (this.disposed || generation !== this.generation) return;
      const denied = error instanceof ApiError && [401, 403, 404].includes(error.status);
      if (denied) {
        this.summaries.clear();
        this.deleted.clear();
        this.update(initialSnapshot());
      }
      this.update({ stale: true, complete: false, error });
    } finally {
      if (!this.disposed && generation === this.generation) this.update({ loading: false });
    }
  };
  loadMore = async () => {
    if (this.snapshot.loading || this.snapshot.complete) return;
    ++this.pages;
    await this.refresh();
  };
  scheduleRefresh = () => {
    if (this.disposed || this.timer) return;
    this.timer = setTimeout(() => {
      this.timer = undefined;
      void this.refresh();
    }, 250);
  };
  applySummary = (payload: TaskStatusSummaryUpdatedPayload) => {
    if (
      this.disposed ||
      payload.workspace_id !== this.workspace ||
      this.deleted.has(payload.task_id)
    )
      return;
    if (!isNewerStatusSummary(payload.status_summary, this.summaries.get(payload.task_id))) return;
    const summary = payload.status_summary;
    if (!summary) return;
    this.summaries.set(payload.task_id, summary);
    this.update({
      tasks: this.snapshot.tasks.map((task) =>
        task.id === payload.task_id
          ? { ...task, status_summary: pickFreshestStatusSummary(summary, task.status_summary) }
          : task,
      ),
    });
  };
  lifecycle = (payload: Pick<TaskEventPayload, "task_id" | "workspace_id">, deleted = false) => {
    if (this.disposed || (payload.workspace_id && payload.workspace_id !== this.workspace)) return;
    if (!payload.workspace_id && !this.snapshot.tasks.some((task) => task.id === payload.task_id))
      return;
    ++this.lifecycleRevision;
    if (deleted) {
      this.deleted.add(payload.task_id);
      this.summaries.delete(payload.task_id);
      this.update({
        tasks: this.snapshot.tasks.filter((task) => task.id !== payload.task_id),
        complete: false,
      });
    }
    this.scheduleRefresh();
  };
}
