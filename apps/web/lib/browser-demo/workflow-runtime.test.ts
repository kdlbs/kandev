import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { DemoWorkerRequest, DemoWorkerResponse } from "./protocol";
import { DEMO_IDS } from "./scenario";
import { handleHttp } from "./worker";

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

function resetWorkerState() {
  const postMessage = vi.spyOn(self, "postMessage").mockImplementation(() => undefined);
  const onMessage = self.onmessage as ((event: MessageEvent<DemoWorkerRequest>) => void) | null;
  onMessage?.(new MessageEvent("message", { data: { kind: "init", id: "reset" } }));
  postMessage.mockClear();
}

beforeEach(resetWorkerState);
afterEach(() => vi.restoreAllMocks());

describe("workflow editing", () => {
  it("supports templates and persists workflow and step edits", async () => {
    const templates = await get("/api/v1/workflow/templates");
    expect(templates).toMatchObject({
      status: 200,
      body: { templates: [{ name: "Kanban" }, { name: "Plan and execute" }], total: 2 },
    });

    const created = await requestHttp("POST", "/api/v1/workflows", {
      workspace_id: DEMO_IDS.workspace,
      name: "Release train",
      workflow_template_id: "plan-execute",
    });
    const workflowId = (created.body as { id: string }).id;
    const updated = await requestHttp("PATCH", `/api/v1/workflows/${workflowId}`, {
      description: "Coordinate a safe production release.",
      agent_profile_id: DEMO_IDS.reviewProfile,
    });
    const initialSteps = await get(`/api/v1/workflows/${workflowId}/workflow/steps`);
    const stepIds = (initialSteps.body as { steps: { id: string }[] }).steps.map((step) => step.id);
    const firstUpdated = await requestHttp("PUT", `/api/v1/workflow/steps/${stepIds[0]}`, {
      prompt: "Write and submit an implementation plan.",
      wip_limit: 2,
    });
    const added = await requestHttp("POST", "/api/v1/workflow/steps", {
      workflow_id: workflowId,
      name: "Deploy",
      position: 4,
      color: "bg-violet-500",
    });
    const addedId = (added.body as { id: string }).id;
    const reordered = await requestHttp(
      "PUT",
      `/api/v1/workflows/${workflowId}/workflow/steps/reorder`,
      { step_ids: [addedId, ...stepIds] },
    );

    expect(created.status).toBe(201);
    expect(updated.body).toMatchObject({
      id: workflowId,
      description: expect.stringContaining("release"),
    });
    expect(initialSteps).toMatchObject({ status: 200, body: { total: 4 } });
    expect(firstUpdated.body).toMatchObject({
      prompt: expect.stringContaining("plan"),
      wip_limit: 2,
    });
    expect((reordered.body as { steps: { id: string }[] }).steps[0].id).toBe(addedId);

    const persisted = vi
      .mocked(self.postMessage)
      .mock.calls.map(([message]) => message as DemoWorkerResponse)
      .findLast((message) => message.kind === "persist") as Extract<
      DemoWorkerResponse,
      { kind: "persist" }
    >;
    const onMessage = self.onmessage as ((event: MessageEvent<DemoWorkerRequest>) => void) | null;
    onMessage?.(
      new MessageEvent("message", {
        data: { kind: "init", id: "restore", persistedState: persisted.state },
      }),
    );
    expect(await get(`/api/v1/workflows/${workflowId}/workflow/steps`)).toMatchObject({
      status: 200,
      body: { total: 5 },
    });
    expect(await get("/api/v1/app-state")).toMatchObject({
      body: {
        initialState: {
          workflows: {
            items: expect.arrayContaining([expect.objectContaining({ id: workflowId })]),
          },
        },
      },
    });
  });

  it("counts and migrates tasks used by destructive edit guards", async () => {
    expect(await get(`/api/v1/workflows/${DEMO_IDS.workflow}/task-count`)).toMatchObject({
      body: { task_count: 5 },
    });
    expect(await get(`/api/v1/workflow/steps/${DEMO_IDS.steps.progress}/task-count`)).toMatchObject(
      {
        body: { task_count: 2 },
      },
    );
    expect(
      await requestHttp("POST", "/api/v1/tasks/bulk-move", {
        source_workflow_id: DEMO_IDS.workflow,
        source_step_id: DEMO_IDS.steps.progress,
        target_workflow_id: DEMO_IDS.supportWorkflow,
        target_step_id: "demo-support-step-triage",
      }),
    ).toMatchObject({ status: 200, body: { moved_count: 2 } });
  });
});

describe("workflow transfer and sync", () => {
  it("round-trips a raw YAML export through import", async () => {
    const created = await requestHttp("POST", "/api/v1/workflows", {
      workspace_id: DEMO_IDS.workspace,
      name: "Customer release",
      description: "Ship a customer-facing release.",
      workflow_template_id: "simple",
    });
    const workflowId = (created.body as { id: string }).id;
    const exported = await get(`/api/v1/workflows/${workflowId}/export`);

    expect(exported).toMatchObject({
      status: 200,
      headers: { "Content-Type": "application/x-yaml" },
      bodyFormat: "text",
    });
    expect(String(exported.body)).toMatch(/^version: 1\ntype: kandev_workflow\n/);
    expect(exported.body).toContain('  - name: "Customer release"');

    await requestHttp("DELETE", `/api/v1/workflows/${workflowId}`);
    const imported = await handleHttp({
      method: "POST",
      path: `/api/v1/workspaces/${DEMO_IDS.workspace}/workflows/import`,
      headers: { "Content-Type": "application/x-yaml" },
      body: String(exported.body),
    });
    expect(imported).toMatchObject({
      status: 200,
      body: { created: ["Customer release"], skipped: [] },
    });
    const workflows = await get(`/api/v1/workspaces/${DEMO_IDS.workspace}/workflows`);
    const importedWorkflow = (
      workflows.body as { workflows: Array<{ id: string; name: string }> }
    ).workflows.find((workflow) => workflow.name === "Customer release");
    const importedSteps = await get(
      `/api/v1/workflows/${importedWorkflow?.id ?? "missing"}/workflow/steps`,
    );
    const steps = (importedSteps.body as { steps: Array<Record<string, unknown>> }).steps;
    const review = steps.find((step) => step.name === "Review");
    const inProgress = steps.find((step) => step.name === "In Progress");
    expect(inProgress?.events).toMatchObject({
      on_turn_complete: [{ config: { step_id: review?.id } }],
    });
  });

  it("configures, runs, and removes GitHub workflow sync", async () => {
    expect(await get(`/api/v1/workflow-sync/config?workspace_id=${DEMO_IDS.workspace}`)).toEqual({
      status: 204,
    });
    const configured = await requestHttp(
      "POST",
      `/api/v1/workflow-sync/config?workspace_id=${DEMO_IDS.workspace}`,
      { repo_owner: "kandev-demo", repo_name: "workflow-library" },
    );
    const synced = await requestHttp(
      "POST",
      `/api/v1/workflow-sync/sync?workspace_id=${DEMO_IDS.workspace}`,
    );
    const removed = await requestHttp(
      "DELETE",
      `/api/v1/workflow-sync/config?workspace_id=${DEMO_IDS.workspace}`,
    );

    expect(configured).toMatchObject({ status: 200, body: { repo_owner: "kandev-demo" } });
    expect(synced).toMatchObject({ status: 200, body: { result: { unchanged: true } } });
    expect(removed).toMatchObject({ status: 200, body: { deleted: true } });
  });
});
