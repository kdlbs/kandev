import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { handleHttp, handleSocketRequest } from "./worker";
import type { DemoWorkerResponse } from "./protocol";
import { createDemoState, DEMO_IDS } from "./scenario";

const CHECKOUT_SESSION = "demo-session-checkout";
const README_PATH = "README.md";
let posted: DemoWorkerResponse[];
function initialize(persistedState?: string) {
  self.onmessage?.(
    new MessageEvent("message", { data: { kind: "init", id: "init", persistedState } }),
  );
}
function socket(action: string, payload: Record<string, unknown>) {
  handleSocketRequest("review", JSON.stringify({ id: "request", action, payload }));
  const response = posted
    .filter((m) => m.kind === "ws-event" && m.data && JSON.parse(m.data).id === "request")
    .at(-1);
  return JSON.parse((response as { data: string }).data);
}
function http(path: string, body: Record<string, unknown>, method = "POST") {
  return handleHttp({ path, method, headers: {}, body: JSON.stringify(body) });
}
beforeEach(() => {
  posted = [];
  vi.spyOn(self, "postMessage").mockImplementation((m) => posted.push(m as DemoWorkerResponse));
  initialize();
  self.onmessage?.(
    new MessageEvent("message", {
      data: { kind: "ws-open", socketId: "review", url: "ws://demo/ws" },
    }),
  );
});
afterEach(() => {
  vi.restoreAllMocks();
  vi.useRealTimers();
});

it("answers the seeded clarification and publishes the resolved message", async () => {
  const answers = [{ question_id: "empty-state-action", selected_option_id: "connect" }];
  const response = await http("/api/v1/clarification/demo-clarification-empty-state/respond", {
    answers,
  });
  expect(response.status).toBe(200);
  const messages = await http("/api/v1/task-sessions/demo-session-empty/messages", {}, "GET");
  expect(messages.body).toMatchObject({
    messages: expect.arrayContaining([
      expect.objectContaining({
        id: "empty-question",
        requests_input: false,
        metadata: expect.objectContaining({ status: "answered", answers }),
      }),
    ]),
  });
  expect(
    posted.some((m) => m.kind === "ws-event" && m.data?.includes("session.message.updated")),
  ).toBe(true);
});

it("preserves file and plan edits in the tab snapshot", () => {
  socket("workspace.file.update", {
    session_id: CHECKOUT_SESSION,
    path: README_PATH,
    desired_content: "saved file",
  });
  socket("task.plan.create", { task_id: "demo-task-checkout", content: "saved plan" });
  const saved = posted.filter((m) => m.kind === "persist").at(-1) as { state: string };
  expect(saved).toBeDefined();
  initialize(saved?.state);
  expect(
    socket("workspace.file.get", { session_id: CHECKOUT_SESSION, path: README_PATH }).payload,
  ).toMatchObject({ content: "saved file" });
  expect(socket("task.plan.get", { task_id: "demo-task-checkout" }).payload).toMatchObject({
    content: "saved plan",
  });
});

it("isolates task workspaces and selects the API repository for a single-repository task", () => {
  socket("workspace.file.update", {
    session_id: CHECKOUT_SESSION,
    path: README_PATH,
    desired_content: "checkout only",
  });
  expect(
    socket("workspace.file.get", { session_id: "demo-session-audit", path: README_PATH }).payload
      .content,
  ).not.toBe("checkout only");
  const state = createDemoState();
  const task = state.tasks.find((t) => t.id === "demo-task-checkout")!;
  task.repositories = [
    {
      ...task.repositories![0],
      repository_id: DEMO_IDS.apiRepository as NonNullable<
        typeof task.repositories
      >[number]["repository_id"],
    },
  ];
  initialize(JSON.stringify(state));
  expect(
    socket("workspace.file.get", { session_id: CHECKOUT_SESSION, path: README_PATH }).payload
      .content,
  ).toContain("Acme API");
});

it.each([
  { name: "Release" },
  { name: "Release", steps: [null] },
  { name: "Release", steps: [{ name: "Run", events: { on_turn_complete: "broken" } }] },
  { name: "Release", steps: [{ name: "Run", events: { on_turn_complete: [null] } }] },
])("rejects malformed workflow imports before mutation: %j", async (workflow) => {
  const before = await http("/api/v1/workflows", {}, "GET");
  await expect(
    http(`/api/v1/workspaces/${DEMO_IDS.workspace}/workflows/import`, {
      version: 1,
      type: "kandev_workflow",
      workflows: [workflow],
    }),
  ).resolves.toMatchObject({ status: 400 });
  expect((await http("/api/v1/workflows", {}, "GET")).body).toEqual(before.body);
});

it("validates bulk-move destinations and emits an update for each moved task", async () => {
  const body = {
    source_workflow_id: DEMO_IDS.workflow,
    target_workflow_id: DEMO_IDS.supportWorkflow,
    target_step_id: "missing",
  };
  expect((await http("/api/v1/tasks/bulk-move", body)).status).toBe(400);
  const steps = await http(
    `/api/v1/workflows/${DEMO_IDS.supportWorkflow}/workflow/steps`,
    {},
    "GET",
  );
  body.target_step_id = (steps.body as { steps: { id: string }[] }).steps[0].id;
  posted.length = 0;
  const moved = await http("/api/v1/tasks/bulk-move", body);
  const events = posted.filter(
    (m) => m.kind === "ws-event" && m.data?.includes('"action":"task.updated"'),
  );
  expect(events).toHaveLength((moved.body as { moved_count: number }).moved_count);
  expect(events.length).toBeGreaterThan(0);
});

it("returns live workflow steps for a task move", async () => {
  const steps = await http(
    `/api/v1/workflows/${DEMO_IDS.supportWorkflow}/workflow/steps`,
    {},
    "GET",
  );
  const step = (steps.body as { steps: { id: string }[] }).steps[0];
  const moved = await http("/api/v1/tasks/demo-task-empty/move", { workflow_step_id: step.id });
  expect(moved.body).toMatchObject({ workflow_step: step });
});

it("follows the template turn-complete destination", async () => {
  vi.useFakeTimers();
  const created = await http("/api/v1/workflows", {
    workspace_id: DEMO_IDS.workspace,
    name: "Template",
  });
  const id = (created.body as { id: string }).id;
  await http(`/api/v1/workflows/${id}/workflow/steps`, { template_id: "simple" });
  const snapshot = await http(`/api/v1/workflows/${id}/workflow/steps`, {}, "GET");
  const steps = (snapshot.body as { steps: { id: string; name: string }[] }).steps;
  const task = await http("/api/v1/tasks", {
    title: "Template task",
    start_agent: true,
    workflow_id: id,
    workflow_step_id: steps.find((s) => s.name === "In Progress")!.id,
  });
  await vi.runAllTimersAsync();
  const result = await http(`/api/v1/tasks/${(task.body as { id: string }).id}`, {}, "GET");
  expect(result.body).toMatchObject({
    workflow_step_id: steps.find((s) => s.name === "Review")!.id,
  });
});

it("returns a bounded HTTP error when a handler throws", async () => {
  await expect(http("http://[", {})).resolves.toMatchObject({
    status: 500,
    body: { demo_mode: true, error: expect.any(String) },
  });
});

it("returns a correlated WebSocket error when a request handler throws", () => {
  const response = socket(42 as unknown as string, {});
  expect(response).toMatchObject({
    id: "request",
    type: "error",
    payload: { demo_mode: true, message: expect.any(String) },
  });
});
