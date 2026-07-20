import { afterEach, describe, expect, it, vi } from "vitest";
import type { DemoWorkerResponse } from "./protocol";
import { DEMO_IDS } from "./scenario";
import { handleHttp, handleSocketRequest } from "./worker";

function get(path: string) {
  return handleHttp({ method: "GET", path, headers: {} });
}

let socketRequestSequence = 0;

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
      body: { sessions: [{ task_environment_id: "demo-environment" }] },
    });
    expect(terminals).toMatchObject({
      status: 200,
      body: { terminals: [{ kind: "ordinary", pty_status: "running" }] },
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
  it("serves a browsable workspace tree and file contents", () => {
    const tree = requestSocket("workspace.tree.get", {
      session_id: "demo-session-audit",
      path: "",
      depth: 1,
    });
    const file = requestSocket("workspace.file.get", {
      session_id: "demo-session-audit",
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
