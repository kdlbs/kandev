import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { DemoWorkerRequest, DemoWorkerResponse } from "./protocol";
import type { Message } from "@/lib/types/http";
import { createDemoState, DEMO_IDS } from "./scenario";
import { handleHttp, handleSocketRequest } from "./worker";

const SOCKET = "follow-up-test";
const SESSION_ID = "demo-session-auth";
const ADD_MESSAGE = "message.add";
function dispatch(message: DemoWorkerRequest) {
  (self.onmessage as ((event: MessageEvent<DemoWorkerRequest>) => void) | null)?.(
    new MessageEvent("message", { data: message }),
  );
}
function request(action: string, payload: Record<string, unknown>) {
  const spy = vi.mocked(self.postMessage);
  spy.mockClear();
  handleSocketRequest(SOCKET, JSON.stringify({ id: action, type: "request", action, payload }));
  const response = spy.mock.calls
    .map(([item]) => item as DemoWorkerResponse)
    .flatMap((item) =>
      item.kind === "ws-event" && item.event === "message" ? [JSON.parse(item.data ?? "{}")] : [],
    )
    .find((item) => item.id === action);
  return response;
}
async function history(sessionId: string) {
  const result = await handleHttp({
    method: "GET",
    path: `/api/v1/task-sessions/${sessionId}/messages`,
    headers: {},
  });
  return structuredClone((result.body as { messages: Message[] }).messages);
}
function identity(taskId = "demo-task-auth", sessionId = SESSION_ID) {
  return { task_id: taskId, session_id: sessionId, session_incarnation_id: `${sessionId}-queue` };
}
beforeEach(() => {
  vi.useFakeTimers();
  vi.spyOn(self, "postMessage").mockImplementation(() => undefined);
  vi.spyOn(Math, "random").mockReturnValue(0);
  dispatch({ kind: "init", id: "init" });
  dispatch({ kind: "ws-open", socketId: SOCKET, url: "ws://demo.test/ws" });
});
afterEach(() => {
  dispatch({ kind: "ws-close", socketId: SOCKET });
  vi.useRealTimers();
  vi.restoreAllMocks();
});

describe("demo follow-up chat", () => {
  it("records a follow-up once, thinks briefly, adds tools and a varied answer, then returns idle", async () => {
    const original = await history(SESSION_ID);
    const payload = {
      ...identity(),
      content: "Check the logout regression too",
      client_message_id: "follow-up-1",
    };
    expect(request(ADD_MESSAGE, payload)).toMatchObject({
      type: "response",
      payload: { id: "follow-up-1", content: payload.content, author_type: "user" },
    });
    request(ADD_MESSAGE, payload);
    expect(await history(SESSION_ID)).toHaveLength(original.length + 1);
    await vi.advanceTimersByTimeAsync(300);
    expect((await history(SESSION_ID)).at(-1)?.type).toBe("thinking");
    await vi.runAllTimersAsync();
    const messages = await history(SESSION_ID);
    expect(messages.slice(0, original.length)).toEqual(original);
    expect(messages.slice(original.length).map((item) => item.type)).toEqual([
      "message",
      "thinking",
      "tool_read",
      "message",
    ]);
    expect(messages.at(-1)?.metadata?.demo_reply_variant).toBe(0);
    expect(messages.at(-2)?.metadata?.normalized).toMatchObject({
      read_file: { file_path: "/demo/worktrees/demo-task-auth/README.md" },
    });
    expect(messages.map((item) => item.created_at)).toEqual(
      [...messages]
        .sort((a, b) => a.created_at.localeCompare(b.created_at))
        .map((item) => item.created_at),
    );
    request(ADD_MESSAGE, {
      ...payload,
      content: "One more check",
      client_message_id: "follow-up-2",
    });
    await vi.runAllTimersAsync();
    expect((await history(SESSION_ID)).at(-1)?.metadata?.demo_reply_variant).toBe(1);
    expect(
      await handleHttp({ method: "GET", path: "/api/v1/tasks/demo-task-auth", headers: {} }),
    ).toMatchObject({
      body: {
        primary_session_state: "IDLE",
        state: "REVIEW",
        workflow_step_id: DEMO_IDS.steps.review,
      },
    });
  });

  it("drains a queued message on a running task and prevents duplicate retries", async () => {
    const payload = {
      ...identity("demo-task-checkout", "demo-session-checkout"),
      content: "Check the retry path",
      client_queue_id: "queued-check",
    };
    expect(request("message.queue.add", payload)).toMatchObject({
      payload: { id: "queued-check", content: payload.content },
    });
    expect(request("message.queue.get", payload)).toMatchObject({ payload: { count: 1 } });
    request("message.queue.add", payload);
    await vi.runAllTimersAsync();
    expect(request("message.queue.get", payload)).toMatchObject({
      payload: { count: 0, entries: [] },
    });
    const messages = await history("demo-session-checkout");
    expect(
      messages.filter((item) => item.metadata?.client_queue_id === "queued-check"),
    ).toHaveLength(1);
    expect(messages.at(-1)?.author_type).toBe("agent");
  });

  it("serializes rapid follow-ups and rejects mismatched sessions or empty content", async () => {
    expect(
      request(ADD_MESSAGE, { ...identity(), session_id: "demo-session-audit", content: "x" }).type,
    ).toBe("error");
    expect(request(ADD_MESSAGE, { ...identity(), content: "  " }).type).toBe("error");
    for (const id of ["first", "second"])
      request(ADD_MESSAGE, { ...identity(), client_message_id: id, content: id });
    await vi.runAllTimersAsync();
    const answers = (await history(SESSION_ID)).filter(
      (item) => item.metadata?.demo_reply_variant !== undefined,
    );
    expect(answers.map((item) => item.turn_id)).toEqual(["first-reply", "second-reply"]);
  });

  it("cancels pending replies on reset and hydrates fresh queue identities", async () => {
    request(ADD_MESSAGE, { ...identity(), content: "discard on reset" });
    dispatch({ kind: "init", id: "reset" });
    await vi.runAllTimersAsync();
    expect(await history(SESSION_ID)).toEqual(createDemoState().messagesBySession[SESSION_ID]);
  });
});

describe("demo task workflow completion", () => {
  it("keeps a new incident-response task inside its own workflow", async () => {
    const created = await handleHttp({
      method: "POST",
      path: "/api/v1/tasks",
      headers: {},
      body: JSON.stringify({
        title: "Investigate a timeout",
        workflow_id: DEMO_IDS.supportWorkflow,
        workflow_step_id: "demo-support-step-triage",
        start_agent: true,
      }),
    });
    const task = created.body as { id: string };
    await vi.runAllTimersAsync();
    expect(
      await handleHttp({ method: "GET", path: `/api/v1/tasks/${task.id}`, headers: {} }),
    ).toMatchObject({
      body: {
        workflow_id: DEMO_IDS.supportWorkflow,
        workflow_step_id: "demo-support-step-verify",
        primary_session_state: "IDLE",
      },
    });
  });
});
