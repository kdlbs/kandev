import { afterEach, describe, expect, it, vi } from "vitest";
import type { DemoWorkerRequest, DemoWorkerResponse } from "./protocol";
import { DEMO_IDS } from "./scenario";
import { handleHttp, handleSocketRequest } from "./worker";

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
    const [agents, executors, workflows, workflowTemplates, supportSnapshot] = await Promise.all([
      get("/api/v1/agents"),
      get("/api/v1/executors"),
      get(`/api/v1/workspaces/${DEMO_IDS.workspace}/workflows`),
      get("/api/v1/workflow/templates"),
      get(`/api/v1/workflows/${DEMO_IDS.supportWorkflow}/snapshot`),
    ]);

    expect(agents).toMatchObject({ status: 200, body: { total: 1 } });
    expect(executors).toMatchObject({ status: 200, body: { total: 1 } });
    expect(workflows).toMatchObject({ status: 200, body: { total: 2 } });
    expect(workflowTemplates).toMatchObject({
      status: 200,
      body: { templates: [{ name: "Kanban" }, { name: "Plan and execute" }], total: 2 },
    });
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
});

describe("browser demo worker HTTP reporting and remote repositories", () => {
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

  it("routes agent command preview and system settings surfaces without fallbacks", async () => {
    const [preview, disk, database, storage, runs, quarantine] = await Promise.all([
      requestHttp("POST", "/api/v1/agent-command-preview/mock", { model: "demo-fast" }),
      get("/api/v1/system/disk-usage"),
      get("/api/v1/system/database"),
      get("/api/v1/system/storage"),
      get("/api/v1/system/storage/runs?limit=20"),
      get("/api/v1/system/storage/quarantine"),
    ]);

    expect(preview).toMatchObject({ status: 200, body: { supported: true } });
    expect(disk).toMatchObject({ status: 200, body: { computing: false } });
    expect(database).toMatchObject({ status: 200, body: { driver: "sqlite" } });
    expect(storage).toMatchObject({ status: 200, body: { settings: { enabled: true } } });
    expect(runs).toMatchObject({ status: 200, body: { runs: expect.any(Array) } });
    expect(quarantine).toMatchObject({ status: 200, body: { entries: expect.any(Array) } });
  });

  it("returns explicit errors for unknown routed resources", async () => {
    const missingWorkflow = await get("/api/v1/workflows/missing/snapshot");
    const missingTask = await get("/api/v1/tasks/missing");
    const unsupported = await get("/api/v1/not-implemented");

    expect(missingWorkflow).toMatchObject({ status: 404, body: { error: "Workflow not found" } });
    expect(missingTask).toMatchObject({ status: 404, body: { error: "Task not found" } });
    expect(unsupported).toMatchObject({
      status: 501,
      body: { demo_mode: true, unsupported: "/api/v1/not-implemented" },
    });
  });
});

describe("browser demo worker remote repository picker", () => {
  it("serves remote repositories and their branches for the new task dialog", async () => {
    const repositories = await get("/api/v1/github/repos?limit=100");
    const webBranches = await get("/api/v1/github/repos/kandev-demo/acme-web/branches");
    const apiBranches = await get("/api/v1/github/repos/kandev-demo/acme-api/branches");

    expect(repositories).toMatchObject({
      status: 200,
      body: {
        repos: [
          {
            full_name: "kandev-demo/acme-web",
            default_branch: "main",
            private: false,
          },
          {
            full_name: "kandev-demo/acme-api",
            default_branch: "main",
            private: true,
          },
        ],
      },
    });
    expect(webBranches.status).toBe(200);
    expect((webBranches.body as { branches: { name: string }[] }).branches).toEqual(
      expect.arrayContaining([{ name: "main" }, { name: "kandev/audit-logging" }]),
    );
    expect(apiBranches.status).toBe(200);
    expect((apiBranches.body as { branches: { name: string }[] }).branches).toEqual(
      expect.arrayContaining([{ name: "main" }, { name: "develop" }]),
    );
  });

  it("returns a not-found response for branches of an unknown remote repository", async () => {
    const response = await get("/api/v1/github/repos/kandev-demo/missing/branches");

    expect(response).toMatchObject({ status: 404, body: { error: "Repository not found" } });
  });
});

describe("browser demo worker HTTP task mutations", () => {
  it("creates, updates, moves, archives, and deletes tasks with notifications", async () => {
    const postMessage = vi.spyOn(self, "postMessage").mockImplementation(() => undefined);
    const openSocket = self.onmessage as ((event: MessageEvent<DemoWorkerRequest>) => void) | null;
    openSocket?.(
      new MessageEvent("message", {
        data: { kind: "ws-open", socketId: "task-events", url: "ws://demo.test/ws" },
      }),
    );
    postMessage.mockClear();

    const created = await requestHttp("POST", "/api/v1/tasks", {
      title: "Review worker mutations",
      workflow_id: DEMO_IDS.workflow,
      workflow_step_id: DEMO_IDS.steps.backlog,
    });
    const taskId = (created.body as { id: string }).id;
    const updated = await requestHttp("PATCH", `/api/v1/tasks/${taskId}`, {
      title: "Review worker routing",
    });
    const moved = await requestHttp("POST", `/api/v1/tasks/${taskId}/move`, {
      workflow_step_id: DEMO_IDS.steps.review,
      position: 0,
    });

    expect(created.status).toBe(201);
    expect(updated.body).toMatchObject({ id: taskId, title: "Review worker routing" });
    expect(moved.body).toMatchObject({
      task: { id: taskId, workflow_step_id: DEMO_IDS.steps.review, position: 0 },
    });
    expect(await requestHttp("POST", `/api/v1/tasks/${taskId}/archive`)).toMatchObject({
      status: 204,
    });
    expect(await requestHttp("DELETE", `/api/v1/tasks/${taskId}`)).toMatchObject({ status: 204 });

    const actions = postMessage.mock.calls.flatMap(([message]) => {
      const event = message as DemoWorkerResponse;
      if (event.kind !== "ws-event" || event.event !== "message") return [];
      return [JSON.parse(event.data ?? "{}").action as string | undefined];
    });
    expect(actions).toEqual(
      expect.arrayContaining(["task.created", "task.updated", "task.deleted"]),
    );
  });
});

describe("browser demo worker integration settings", () => {
  it("serves a healthy Jira configuration and its watcher list", async () => {
    const [config, watches] = await Promise.all([
      get(`/api/v1/jira/config?workspace_id=${DEMO_IDS.workspace}`),
      get(`/api/v1/jira/watches/issue?workspace_id=${DEMO_IDS.workspace}`),
    ]);

    expect(config).toMatchObject({
      status: 200,
      body: {
        workspaceId: DEMO_IDS.workspace,
        siteUrl: "https://acme-platform.atlassian.net",
        defaultProjectKey: "PLAT",
        hasSecret: true,
        lastOk: true,
      },
    });
    expect(watches).toMatchObject({ status: 200, body: { watches: [] } });
  });

  it("serves Linear configuration, teams, and watchers", async () => {
    const [config, teams, watches] = await Promise.all([
      get(`/api/v1/linear/config?workspace_id=${DEMO_IDS.workspace}`),
      get(`/api/v1/linear/teams?workspace_id=${DEMO_IDS.workspace}`),
      get(`/api/v1/linear/watches/issue?workspace_id=${DEMO_IDS.workspace}`),
    ]);

    expect(config).toMatchObject({
      status: 200,
      body: { defaultTeamKey: "ENG", orgSlug: "acme-platform", hasSecret: true, lastOk: true },
    });
    expect(teams).toMatchObject({
      status: 200,
      body: { teams: [{ key: "ENG", name: "Engineering" }, { key: "PLAT" }] },
    });
    expect(watches).toMatchObject({ status: 200, body: { watches: [] } });
  });

  it("serves configured Sentry instances and their watcher list", async () => {
    const [instances, watches] = await Promise.all([
      get(`/api/v1/sentry/instances?workspace_id=${DEMO_IDS.workspace}`),
      get(`/api/v1/sentry/watches/issue?workspace_id=${DEMO_IDS.workspace}`),
    ]);

    expect(instances).toMatchObject({
      status: 200,
      body: {
        instances: [
          {
            workspaceId: DEMO_IDS.workspace,
            name: "Production",
            url: "https://sentry.io",
            hasSecret: true,
            lastOk: true,
          },
        ],
      },
    });
    expect(watches).toMatchObject({ status: 200, body: { watches: [] } });
  });

  it("serves Slack configuration and its selected utility agent", async () => {
    const [config, agents] = await Promise.all([
      get(`/api/v1/slack/config?workspace_id=${DEMO_IDS.workspace}`),
      get("/api/v1/utility/agents"),
    ]);

    expect(config).toMatchObject({
      status: 200,
      body: {
        commandPrefix: "!kandev",
        utilityAgentId: "demo-utility-triage",
        hasToken: true,
        hasCookie: true,
        lastOk: true,
      },
    });
    expect(agents).toMatchObject({
      status: 200,
      body: {
        agents: [{ id: "demo-utility-triage", name: "Slack task triage", enabled: true }],
      },
    });
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
});

describe("browser demo worker session lifecycle", () => {
  it("reuses an existing session and creates one when the task has none", async () => {
    expect(requestSocket("session.ensure", { task_id: "demo-task-audit" }).payload).toMatchObject({
      source: "existing_primary",
      newly_created: false,
      session_id: AUDIT_SESSION_ID,
    });

    const created = await requestHttp("POST", "/api/v1/tasks", {
      title: "Create a demo session",
      workflow_id: DEMO_IDS.workflow,
      workflow_step_id: DEMO_IDS.steps.backlog,
    });
    const taskId = (created.body as { id: string }).id;
    const first = requestSocket("session.ensure", { task_id: taskId, prompt: "Inspect the task" });
    const second = requestSocket("session.ensure", { task_id: taskId });

    expect(first.payload).toMatchObject({ source: "created_start", newly_created: true });
    expect(second.payload).toMatchObject({
      source: "existing_primary",
      newly_created: false,
      session_id: first.payload.session_id,
    });
    expect(await requestHttp("DELETE", `/api/v1/tasks/${taskId}`)).toMatchObject({ status: 204 });
  });

  it("resolves the seeded audit permission and clears the pending session action", async () => {
    const before = requestSocket("message.list", { session_id: AUDIT_SESSION_ID });
    const pending = before.payload.messages.find(
      (message: { id: string }) => message.id === "audit-migration-permission",
    );

    expect(pending).toMatchObject({
      requests_input: true,
      metadata: { status: "pending", tool_call_id: "audit-migration-check" },
    });
    expect(
      requestSocket("permission.respond", {
        session_id: AUDIT_SESSION_ID,
        pending_id: "audit-migration-permission",
        option_id: "audit-allow-once",
      }).payload,
    ).toEqual({ success: true, status: "approved" });

    const after = requestSocket("message.list", { session_id: AUDIT_SESSION_ID });
    const resolved = after.payload.messages.find(
      (message: { id: string }) => message.id === "audit-migration-permission",
    );
    expect(resolved).toMatchObject({ requests_input: false, metadata: { status: "approved" } });
    expect(await get("/api/v1/tasks/demo-task-audit")).toMatchObject({
      body: {
        primary_session_state: "IDLE",
        primary_session_pending_action: null,
      },
    });
  });
});

describe("browser demo worker workspace WebSocket runtime", () => {
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

  it("supports the workspace file mutation lifecycle", () => {
    const path = "src/demo-review-fixture.ts";

    expect(requestSocket("workspace.file.create", { path }).payload).toMatchObject({
      path,
      success: true,
    });
    expect(
      requestSocket("workspace.file.update", { path, desired_content: "export const demo = true;" })
        .payload,
    ).toMatchObject({ path, success: true, resolution: "applied" });
    expect(requestSocket("workspace.file.get", { path }).payload.content).toBe(
      "export const demo = true;",
    );

    const renamedPath = "src/demo-review-renamed.ts";
    expect(
      requestSocket("workspace.file.rename", { old_path: path, new_path: renamedPath }).payload,
    ).toMatchObject({ old_path: path, new_path: renamedPath, success: true });
    expect(requestSocket("workspace.file.delete", { path: renamedPath }).payload).toMatchObject({
      path: renamedPath,
      success: true,
    });
    expect(requestSocket("workspace.file.get", { path: renamedPath })).toMatchObject({
      type: "error",
      payload: { message: expect.stringContaining("File not found") },
    });
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
