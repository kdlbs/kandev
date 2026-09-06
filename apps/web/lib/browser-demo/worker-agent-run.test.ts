import { afterEach, describe, expect, it, vi } from "vitest";
import type { DemoWorkerRequest, DemoWorkerResponse } from "./protocol";
import { DEMO_IDS } from "./scenario";
import { handleHttp, handleSocketRequest } from "./worker";

const SOCKET_ID = "new-task-run";

function get(path: string) {
  return handleHttp({ method: "GET", path, headers: {} });
}

function requestHttp(method: string, path: string, body?: Record<string, unknown>) {
  return handleHttp({
    method,
    path,
    headers: {},
    body: body ? JSON.stringify(body) : undefined,
  });
}

function requestSocket(action: string, payload: Record<string, unknown>) {
  const postMessage = vi.spyOn(self, "postMessage").mockImplementation(() => undefined);
  postMessage.mockClear();
  handleSocketRequest(
    SOCKET_ID,
    JSON.stringify({ id: `request-${action}`, type: "request", action, payload }),
  );
  const response = postMessage.mock.calls
    .map(([message]) => message as DemoWorkerResponse)
    .find((message) => message.kind === "ws-event" && message.event === "message");
  return JSON.parse((response as Extract<DemoWorkerResponse, { kind: "ws-event" }>).data ?? "{}");
}

function dispatchWorker(message: DemoWorkerRequest) {
  const handler = self.onmessage as ((event: MessageEvent<DemoWorkerRequest>) => void) | null;
  handler?.(new MessageEvent("message", { data: message }));
}

function readNotifications(calls: unknown[][]) {
  return calls
    .map((call) => call[0] as DemoWorkerResponse)
    .filter((message) => message.kind !== "ws-event" || message.socketId === SOCKET_ID)
    .flatMap((message) =>
      message.kind === "ws-event" && message.event === "message"
        ? [JSON.parse(message.data ?? "{}")]
        : [],
    )
    .filter((message) => message.type === "notification");
}

function expectNotifications(notifications: Record<string, unknown>[], sessionId: string) {
  expect(
    notifications.filter((message) => message.action === "session.message.added"),
  ).toHaveLength(7);
  expect(notifications).toEqual(
    expect.arrayContaining([
      expect.objectContaining({
        action: "session.state_changed",
        payload: expect.objectContaining({ new_state: "IDLE", session_id: sessionId }),
      }),
      expect.objectContaining({
        action: "task.updated",
        payload: expect.objectContaining({
          state: "REVIEW",
          workflow_step_id: DEMO_IDS.steps.review,
        }),
      }),
      expect.objectContaining({
        action: "session.git.event",
        payload: expect.objectContaining({
          session_id: sessionId,
          status: expect.objectContaining({
            modified: ["src/pages/dashboard-page.tsx"],
            added: ["tests/dashboard.test.tsx"],
          }),
        }),
      }),
    ]),
  );
}

function expectGitData(sessionId: string) {
  const commits = requestSocket("session.git.commits", { session_id: sessionId });
  const diff = requestSocket("session.cumulative_diff", { session_id: sessionId });
  expect(commits.payload).toEqual({ commits: [], ready: true });
  expect(diff.payload.cumulative_diff).toMatchObject({
    session_id: sessionId,
    total_commits: 0,
    files: {
      "src/pages/dashboard-page.tsx": {
        status: "modified",
        diff: expect.stringContaining("ServiceHealthSummary"),
      },
      "tests/dashboard.test.tsx": {
        status: "added",
        diff: expect.stringContaining("shows current service details"),
      },
    },
  });
}

afterEach(() => {
  vi.useRealTimers();
  vi.restoreAllMocks();
});

describe("browser demo worker new task agent run", () => {
  it("streams a rich implementation turn, becomes idle, and moves the task to review", async () => {
    vi.useFakeTimers();
    const postMessage = vi.spyOn(self, "postMessage").mockImplementation(() => undefined);
    dispatchWorker({ kind: "ws-open", socketId: SOCKET_ID, url: "ws://demo.test/ws" });
    postMessage.mockClear();
    let taskId = "";

    try {
      const created = await requestHttp("POST", "/api/v1/tasks", {
        title: "Show service health details",
        description: "Add service health details to the operations dashboard.",
        workflow_id: DEMO_IDS.workflow,
        workflow_step_id: DEMO_IDS.steps.backlog,
        start_agent: true,
      });
      taskId = (created.body as { id: string }).id;
      const sessionId = (created.body as { session_id: string }).session_id;
      expect(created).toMatchObject({
        body: {
          state: "IN_PROGRESS",
          workflow_step_id: DEMO_IDS.steps.progress,
          primary_session_state: "RUNNING",
        },
      });

      await vi.runAllTimersAsync();
      const [task, session, history] = await Promise.all([
        get(`/api/v1/tasks/${taskId}`),
        get(`/api/v1/task-sessions/${sessionId}`),
        get(`/api/v1/task-sessions/${sessionId}/messages`),
      ]);
      expect(task).toMatchObject({
        body: {
          state: "REVIEW",
          workflow_step_id: DEMO_IDS.steps.review,
          primary_session_state: "IDLE",
          review_status: "pending",
        },
      });
      expect(session).toMatchObject({ body: { session: { state: "IDLE" } } });
      const messages = (history.body as { messages: { type: string; content: string }[] }).messages;
      expect(messages.map((message) => message.type)).toEqual([
        "message",
        "thinking",
        "tool_search",
        "tool_read",
        "tool_edit",
        "tool_execute",
        "message",
      ]);
      expect(messages.at(-1)?.content).toContain("PASS tests/dashboard.test.tsx");

      const notifications = readNotifications(postMessage.mock.calls);
      expect(postMessage.mock.calls.some(([message]) => message.kind === "persist")).toBe(true);
      expectNotifications(notifications, sessionId);
      expectGitData(sessionId);
    } finally {
      if (taskId) await requestHttp("DELETE", `/api/v1/tasks/${taskId}`);
      dispatchWorker({ kind: "ws-close", socketId: SOCKET_ID });
    }
  });
});
