import type { Page } from "@playwright/test";

type PreviewTask = {
  id: string;
  workflowId?: string | null;
  workflowStepId?: string | null;
  state?: string | null;
  primarySessionId?: string | null;
  primarySessionState?: string | null;
  description?: string;
  updatedAt?: string;
  statusSummary?: Record<string, unknown> | null;
};

type PreviewSession = {
  id: string;
  task_id?: string;
  state?: string;
  is_primary?: boolean;
  agent_profile_id?: string;
  started_at?: string;
  metadata?: Record<string, unknown> | null;
  last_read_message_id?: string;
  command_count?: number;
  updated_at?: string;
};

type PreviewStoreState = {
  connection: { status: string };
  workspaceContextGeneration: number;
  kanban: { tasks: PreviewTask[] };
  taskSessions: { items: Record<string, PreviewSession> };
  taskSessionsByTask: {
    itemsByTaskId: Record<string, PreviewSession[]>;
    loadedByTaskId: Record<string, boolean>;
  };
  agentProfiles: { version: number };
};

type PreviewStoreWindow = Window & {
  __KANDEV_E2E_STORE__?: {
    getState: () => PreviewStoreState;
    setState: (updater: (state: PreviewStoreState) => void) => void;
  };
};

/** Apply the bookkeeping updates that previously caused an open preview to reload. */
export async function applyHarmlessPreviewUpdate(
  page: Page,
  taskId: string,
  revision: number,
): Promise<void> {
  await page.waitForFunction(() => Boolean((window as PreviewStoreWindow).__KANDEV_E2E_STORE__));
  await page.evaluate(
    ({ taskId: currentTaskId, revision: currentRevision }) => {
      const store = (window as PreviewStoreWindow).__KANDEV_E2E_STORE__;
      if (!store) throw new Error("E2E store bridge is unavailable");
      store.setState((state) => {
        const task = state.kanban.tasks.find(({ id }) => id === currentTaskId);
        if (!task) throw new Error(`Task ${currentTaskId} is not in the Kanban store`);
        task.description = `Bookkeeping update ${currentRevision}`;
        task.updatedAt = `2026-01-01T00:00:0${currentRevision}.000Z`;
        task.statusSummary = {
          ...(task.statusSummary ?? {}),
          bookkeeping_revision: currentRevision,
        };

        const sessions = state.taskSessionsByTask.itemsByTaskId[currentTaskId] ?? [];
        for (const session of sessions) {
          session.last_read_message_id = `bookkeeping-${currentRevision}`;
          session.command_count = currentRevision;
          session.updated_at = `2026-01-01T00:00:0${currentRevision}.000Z`;
          const indexed = state.taskSessions.items[session.id];
          if (indexed && indexed !== session) {
            indexed.last_read_message_id = session.last_read_message_id;
            indexed.command_count = session.command_count;
            indexed.updated_at = session.updated_at;
          }
        }
        state.agentProfiles.version += 1;
      });
    },
    { taskId, revision },
  );
}

export async function waitForPreviewTaskSessionsLoaded(page: Page, taskId: string): Promise<void> {
  await page.waitForFunction(
    (currentTaskId) =>
      (window as PreviewStoreWindow).__KANDEV_E2E_STORE__?.getState().taskSessionsByTask
        .loadedByTaskId[currentTaskId] === true,
    taskId,
    { timeout: 15_000, message: `Task sessions did not load for preview task ${taskId}` },
  );
}

export function movePreviewRequestPredicate(taskId: string, workflowStepId?: string) {
  return (request: { method(): string; url(): string; postDataJSON(): unknown }) => {
    if (
      request.method() !== "POST" ||
      new URL(request.url()).pathname !== `/api/v1/tasks/${taskId}/move-preview`
    ) {
      return false;
    }
    if (!workflowStepId) return true;
    const payload = request.postDataJSON() as { workflow_step_id?: unknown } | null;
    return payload?.workflow_step_id === workflowStepId;
  };
}
