/* eslint-disable max-lines-per-function */

import { afterEach, describe, expect, it, vi } from "vitest";
import type { DemoWorkerRequest, DemoWorkerResponse } from "./protocol";
import { DEMO_IDS } from "./scenario";
import { handleHttp, handleSocketRequest } from "./worker";

function get(path: string) {
  return handleHttp({ method: "GET", path, headers: {} });
}

let socketRequestSequence = 0;
const CHECKOUT_SESSION_ID = "demo-session-checkout";
const AUDIT_SESSION_ID = "demo-session-audit";
const REACT_TASK_ID = "demo-task-react";

function requestSocket(action: string, payload: Record<string, unknown> = {}) {
  const postMessage = vi.spyOn(self, "postMessage").mockImplementation(() => undefined);
  postMessage.mockClear();
  const requestId = `request-${++socketRequestSequence}`;
  handleSocketRequest(
    "control-socket",
    JSON.stringify({ id: requestId, type: "request", action, payload }),
  );
  const response = postMessage.mock.calls
    .map(([message]) => message as DemoWorkerResponse)
    .find(
      (message) =>
        message.kind === "ws-event" &&
        message.event === "message" &&
        JSON.parse(message.data ?? "{}").id === requestId,
    );
  expect(response).toBeDefined();
  return JSON.parse((response as Extract<DemoWorkerResponse, { kind: "ws-event" }>).data ?? "{}");
}

afterEach(() => {
  vi.restoreAllMocks();
});

describe("browser demo worker HTTP runtime", () => {
  it("serves the workspace and repository scripts used by settings and task panels", async () => {
    const workspace = await get(`/api/v1/workspaces/${DEMO_IDS.workspace}`);
    const scripts = await get(`/api/v1/repositories/${DEMO_IDS.repository}/scripts`);

    expect(workspace).toMatchObject({ status: 200, body: { id: DEMO_IDS.workspace } });
    expect(scripts.status).toBe(200);
    expect(scripts.body).toMatchObject({ total: 2 });
    expect((scripts.body as { scripts: unknown[] }).scripts).toEqual(
      expect.arrayContaining([
        expect.objectContaining({ name: "Run tests", command: "pnpm test" }),
      ]),
    );
  });

  it("adds the demo environment to task sessions and exposes an ordinary terminal", async () => {
    const sessions = await get("/api/v1/tasks/demo-task-audit/sessions");
    const terminals = await get("/api/v1/tasks/demo-task-audit/terminals");

    expect(sessions).toMatchObject({
      status: 200,
      body: { sessions: [{ task_environment_id: "demo-environment-demo-task-audit" }] },
    });
    expect(terminals).toMatchObject({
      status: 200,
      body: { terminals: [{ kind: "ordinary", pty_status: "running" }] },
    });
  });

  it("serves the resources required to navigate to tasks and configure new ones", async () => {
    const [agents, executors, workflows, supportSnapshot] = await Promise.all([
      get("/api/v1/agents"),
      get("/api/v1/executors"),
      get(`/api/v1/workspaces/${DEMO_IDS.workspace}/workflows`),
      get(`/api/v1/workflows/${DEMO_IDS.supportWorkflow}/snapshot`),
    ]);

    expect(agents).toMatchObject({ status: 200, body: { total: 1 } });
    expect(executors).toMatchObject({ status: 200, body: { total: 1 } });
    expect(workflows).toMatchObject({ status: 200, body: { total: 2 } });
    expect(supportSnapshot).toMatchObject({
      status: 200,
      body: { workflow: { id: DEMO_IDS.supportWorkflow }, steps: { length: 3 } },
    });
  });

  it("serves PR reviews and comments for the demo pull request", async () => {
    const response = await get("/api/v1/github/prs/kandev-demo/acme-web/142");

    expect(response.status).toBe(200);
    const feedback = response.body as { comments: unknown[]; checks: unknown[] };
    expect(feedback.comments).toEqual(
      expect.arrayContaining([expect.objectContaining({ author: "mira" })]),
    );
    expect(feedback.checks).toEqual(
      expect.arrayContaining([expect.objectContaining({ conclusion: "success" })]),
    );
  });

  it("serves every statistics section used by the statistics page", async () => {
    const sections = [
      "global",
      "tasks",
      "daily-activity",
      "completed-activity",
      "agent-usage",
      "repositories",
      "git",
    ];
    const responses = await Promise.all(
      sections.map((section) =>
        get(`/api/v1/workspaces/${DEMO_IDS.workspace}/stats/${section}?range=week`),
      ),
    );

    expect(responses.every((response) => response.status === 200)).toBe(true);
    expect(responses[0].body).toMatchObject({ total_tasks: expect.any(Number) });
  });
});

describe("browser demo worker WebSocket runtime", () => {
  it("announces the selected task's environment when a session subscribes", () => {
    const postMessage = vi.spyOn(self, "postMessage").mockImplementation(() => undefined);
    const openSocket = self.onmessage as ((event: MessageEvent<DemoWorkerRequest>) => void) | null;
    openSocket?.(
      new MessageEvent("message", {
        data: { kind: "ws-open", socketId: "control-socket", url: "ws://demo.test/ws" },
      }),
    );
    postMessage.mockClear();
    handleSocketRequest(
      "control-socket",
      JSON.stringify({
        id: "subscribe-audit",
        type: "request",
        action: "session.subscribe",
        payload: { session_id: AUDIT_SESSION_ID },
      }),
    );

    const ready = postMessage.mock.calls
      .map(([message]) => message as DemoWorkerResponse)
      .flatMap((message) =>
        message.kind === "ws-event" && message.event === "message"
          ? [JSON.parse(message.data ?? "{}")]
          : [],
      )
      .find((message) => message.action === "session.agentctl_ready");

    expect(ready?.payload).toMatchObject({
      session_id: AUDIT_SESSION_ID,
      task_environment_id: "demo-environment-demo-task-audit",
    });
  });

  it("serves the seeded task plan through the plan editor protocol", () => {
    const plan = requestSocket("task.plan.get", { task_id: REACT_TASK_ID });
    const revisions = requestSocket("task.plan.revisions.list", {
      task_id: REACT_TASK_ID,
    });

    expect(plan.payload).toMatchObject({
      task_id: REACT_TASK_ID,
      title: "React 19 multi-repository upgrade",
      created_by: "agent",
    });
    expect(plan.payload.content).toContain("acme-api");
    expect(revisions.payload.revisions).toEqual([
      expect.objectContaining({ task_id: REACT_TASK_ID, revision_number: 1 }),
    ]);
  });

  it("serves changed files, commits, and cumulative diffs for seeded work", () => {
    const commits = requestSocket("session.git.commits", {
      session_id: CHECKOUT_SESSION_ID,
    });
    const diff = requestSocket("session.cumulative_diff", {
      session_id: CHECKOUT_SESSION_ID,
    });

    expect(commits.payload).toMatchObject({
      ready: true,
      commits: [expect.objectContaining({ commit_message: expect.stringContaining("checkout") })],
    });
    expect(diff.payload.cumulative_diff).toMatchObject({
      session_id: CHECKOUT_SESSION_ID,
      total_commits: 1,
      files: {
        "src/checkout/complete-order.ts": {
          status: "modified",
          diff: expect.stringContaining("completePayment"),
        },
        "tests/checkout/concurrent-inventory.test.ts": { status: "added" },
      },
    });
  });

  it("returns an empty git history for tasks that have not changed files", () => {
    const commits = requestSocket("session.git.commits", {
      session_id: "demo-session-react",
    });
    const diff = requestSocket("session.cumulative_diff", {
      session_id: "demo-session-react",
    });

    expect(commits.payload).toEqual({ commits: [], ready: true });
    expect(diff.payload).toEqual({ cumulative_diff: null, ready: true });
  });

  it("serves a browsable workspace tree and file contents", () => {
    const tree = requestSocket("workspace.tree.get", {
      session_id: AUDIT_SESSION_ID,
      path: "",
      depth: 1,
    });
    const file = requestSocket("workspace.file.get", {
      session_id: AUDIT_SESSION_ID,
      path: "README.md",
    });

    expect(tree.payload.root.children).toEqual(
      expect.arrayContaining([expect.objectContaining({ name: "src", is_dir: true })]),
    );
    expect(file.payload).toMatchObject({ path: "README.md", is_binary: false });
    expect(file.payload.content).toContain("Acme Web");
  });

  it("lists the demo shell with the terminal union shape", () => {
    const response = requestSocket("user_shell.list", {
      task_id: "demo-task-audit",
      task_environment_id: "demo-environment",
      include_parked: true,
    });

    expect(response.payload.shells).toEqual([
      expect.objectContaining({
        id: "demo-terminal-1",
        kind: "ordinary",
        display_name: "Terminal 1",
        state: "open",
      }),
    ]);
  });
});
