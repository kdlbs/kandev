import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { Message, Task } from "@/lib/types/http";
import { DEMO_IDS } from "./scenario";
import type { DemoWorkerRequest, DemoWorkerResponse } from "./protocol";
import { handleHttp, handleSocketRequest } from "./worker";

function dispatch(message: DemoWorkerRequest) {
  (self.onmessage as ((event: MessageEvent<DemoWorkerRequest>) => void) | null)?.(
    new MessageEvent("message", { data: message }),
  );
}
const http = (method: string, path: string, body?: unknown) =>
  handleHttp({
    method,
    path,
    headers: {},
    body: body === undefined ? undefined : JSON.stringify(body),
  });
function request(action: string, payload: Record<string, unknown>) {
  handleSocketRequest("lifecycle", JSON.stringify({ id: action, action, payload }));
  return vi
    .mocked(self.postMessage)
    .mock.calls.map(([item]) => item as DemoWorkerResponse)
    .flatMap((item) =>
      item.kind === "ws-event" && item.event === "message" ? [JSON.parse(item.data ?? "{}")] : [],
    )
    .findLast((item) => item.id === action);
}
function savedState() {
  const saved = vi
    .mocked(self.postMessage)
    .mock.calls.map(([item]) => item as DemoWorkerResponse)
    .findLast((item) => item.kind === "persist");
  if (saved?.kind !== "persist") throw new Error("No persisted state");
  return saved.state;
}
async function startTask() {
  const result = await http("POST", "/api/v1/tasks", {
    title: "Lifecycle regression",
    workflow_id: DEMO_IDS.workflow,
    start_agent: true,
  });
  return result.body as Task & { session_id: string };
}
async function messages(sessionId: string) {
  return structuredClone(
    (await http("GET", `/api/v1/task-sessions/${sessionId}/messages`)).body,
  ) as { messages: Message[] };
}
beforeEach(() => {
  vi.useFakeTimers();
  vi.spyOn(self, "postMessage").mockImplementation(() => undefined);
  dispatch({ kind: "init", id: "init" });
});
afterEach(() => {
  vi.clearAllTimers();
  vi.useRealTimers();
  vi.restoreAllMocks();
});

describe("demo task detail reads", () => {
  it("serves task detail reads with seeded data instead of generic success", () => {
    const identity = { task_id: "demo-task-audit", session_id: "demo-session-audit" };
    expect(request("task.session.status", identity)).toMatchObject({
      type: "response",
      payload: { ...identity, state: "WAITING_FOR_INPUT", needs_resume: false },
    });
    expect(request("github.task_pr.sync", identity)).toMatchObject({
      type: "response",
      payload: { prs: [expect.objectContaining({ pr_number: 142 })] },
    });
    expect(request("task.walkthrough.get", identity)).toMatchObject({
      type: "response",
      payload: null,
    });
    expect(request("task.preview_feedback.list", identity)).toMatchObject({
      type: "response",
      payload: { task_id: identity.task_id, revision: 0, items: [] },
    });
    expect(
      request("github.pr_files.get", { owner: "kandev-demo", repo: "acme-web", number: 142 }),
    ).toMatchObject({
      type: "response",
      payload: {
        files: expect.arrayContaining([
          expect.objectContaining({ filename: "src/audit/record-event.ts" }),
        ]),
      },
    });
  });
});

describe("demo lifecycle recovery", () => {
  it("settles a persisted run on reload and accepts a fresh follow-up", async () => {
    const task = await startTask();
    await vi.advanceTimersByTimeAsync(500);
    const before = await messages(task.session_id);
    const persistedState = savedState();
    vi.clearAllTimers();
    dispatch({ kind: "init", id: "reload", persistedState });
    await vi.runAllTimersAsync();
    expect(await http("GET", `/api/v1/tasks/${task.id}`)).toMatchObject({
      body: { primary_session_state: "IDLE" },
    });
    expect(await messages(task.session_id)).toEqual(before);
    request("message.add", {
      task_id: task.id,
      session_id: task.session_id,
      content: "Continue",
      client_message_id: "after-reload",
    });
    await vi.runAllTimersAsync();
    expect((await messages(task.session_id)).messages.at(-1)?.turn_id).toBe("after-reload-reply");
  });

  it.each(["session.stop", "agent.cancel", "orchestrator.stop"])(
    "%s stops timers and queued turns without affecting a replacement run",
    async (action) => {
      const task = await startTask();
      await vi.advanceTimersByTimeAsync(500);
      request("message.add", {
        task_id: task.id,
        session_id: task.session_id,
        content: "Old follow-up",
        client_message_id: "old",
      });
      request("message.queue.add", {
        task_id: task.id,
        session_id: task.session_id,
        session_incarnation_id: `${task.session_id}-queue`,
        content: "Old queue",
        client_queue_id: "old-queue",
      });
      const before = await messages(task.session_id);
      expect(
        request(
          action,
          action === "orchestrator.stop" ? { task_id: task.id } : { session_id: task.session_id },
        ),
      ).toMatchObject({
        type: "response",
        payload: { success: true },
      });
      expect(await http("GET", `/api/v1/tasks/${task.id}`)).toMatchObject({
        body: { primary_session_state: "IDLE" },
      });
      expect(
        request("message.queue.get", { task_id: task.id, session_id: task.session_id }),
      ).toMatchObject({ payload: { count: 0 } });
      request("message.add", {
        task_id: task.id,
        session_id: task.session_id,
        content: "New follow-up",
        client_message_id: "new",
      });
      await vi.runAllTimersAsync();
      const after = (await messages(task.session_id)).messages;
      expect(after).toHaveLength(before.messages.length + 4);
      expect(after.at(-1)?.turn_id).toBe("new-reply");
    },
  );

  it("rejects unsupported actions and missing stop targets", () => {
    expect(request("unsupported.action", {})).toMatchObject({ type: "error" });
    expect(request("session.stop", { session_id: "missing" })).toMatchObject({ type: "error" });
  });

  it.each(["workflow", "step"])("deletes %s tasks, sessions, and history durably", async (kind) => {
    const task = await startTask();
    const path =
      kind === "workflow"
        ? `/api/v1/workflows/${task.workflow_id}`
        : `/api/v1/workflow/steps/${task.workflow_step_id}`;
    expect((await http("DELETE", path)).status).toBe(200);
    await vi.runAllTimersAsync();
    dispatch({ kind: "init", id: "reload", persistedState: savedState() });
    expect((await http("GET", `/api/v1/tasks/${task.id}`)).status).toBe(404);
    expect((await http("GET", `/api/v1/task-sessions/${task.session_id}`)).status).toBe(404);
    expect(await messages(task.session_id)).toEqual({ messages: [], has_more: false });
    const workspace = (await http("GET", `/api/v1/workspaces/${DEMO_IDS.workspace}/tasks`))
      .body as { tasks: Task[] };
    expect(workspace.tasks.some((item) => item.id === task.id)).toBe(false);
  });
});
