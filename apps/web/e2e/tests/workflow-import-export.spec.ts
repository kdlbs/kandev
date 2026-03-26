import { test, expect } from "../fixtures/test-base";

test.describe("Workflow import/export", () => {
  test("export all workflows produces valid YAML", async ({ apiClient, seedData }) => {
    const yaml = await apiClient.exportAllWorkflows(seedData.workspaceId);

    expect(yaml).toContain("version: 1");
    expect(yaml).toContain("type: kandev_workflow");
    expect(yaml).toContain("E2E Workflow");
  });

  test("export single workflow produces valid YAML with steps", async ({ apiClient, seedData }) => {
    const yaml = await apiClient.exportWorkflow(seedData.workflowId);

    expect(yaml).toContain("version: 1");
    expect(yaml).toContain("E2E Workflow");
    // Kanban template steps
    expect(yaml).toContain("Backlog");
    expect(yaml).toContain("In Progress");
    expect(yaml).toContain("Review");
    expect(yaml).toContain("Done");
  });

  test("import YAML creates new workflows", async ({ apiClient, seedData }) => {
    const yamlContent = `
version: 1
type: kandev_workflow
workflows:
  - name: Imported Workflow
    steps:
      - name: Todo
        position: 0
        color: bg-neutral-400
        is_start_step: true
        allow_manual_move: true
      - name: Done
        position: 1
        color: bg-green-500
        allow_manual_move: true
`;
    const result = await apiClient.importWorkflows(seedData.workspaceId, yamlContent);

    expect(result.created).toContain("Imported Workflow");
    expect(result.skipped).toHaveLength(0);

    // Verify the workflow exists
    const { workflows } = await apiClient.listWorkflows(seedData.workspaceId);
    const imported = workflows.find((w) => w.name === "Imported Workflow");
    expect(imported).toBeDefined();

    // Verify steps
    const { steps } = await apiClient.listWorkflowSteps(imported!.id);
    expect(steps).toHaveLength(2);
    expect(steps.map((s) => s.name).sort()).toEqual(["Done", "Todo"]);
  });

  test("import skips duplicate workflow names", async ({ apiClient, seedData }) => {
    const yamlContent = `
version: 1
type: kandev_workflow
workflows:
  - name: E2E Workflow
    steps:
      - name: Step1
        position: 0
        color: bg-neutral-400
        is_start_step: true
        allow_manual_move: true
`;
    const result = await apiClient.importWorkflows(seedData.workspaceId, yamlContent);

    expect(result.skipped).toContain("E2E Workflow");
    expect(result.created).toHaveLength(0);
  });

  test("round-trip export then import preserves structure", async ({ apiClient, seedData }) => {
    // Create a workflow with custom prompt
    const wf = await apiClient.createWorkflow(seedData.workspaceId, "Roundtrip Test", "standard");
    const { steps: originalSteps } = await apiClient.listWorkflowSteps(wf.id);

    // Customize a step prompt
    const planStep = originalSteps.find((s) => s.name === "Plan");
    if (planStep) {
      await apiClient.updateWorkflowStep(planStep.id, { prompt: "Custom plan prompt for roundtrip" });
    }

    // Re-fetch after update
    const { steps: updatedSteps } = await apiClient.listWorkflowSteps(wf.id);

    // Export
    const yaml = await apiClient.exportWorkflow(wf.id);
    expect(yaml).toContain("Roundtrip Test");
    expect(yaml).toContain("Custom plan prompt for roundtrip");

    // Delete the workflow
    await apiClient.deleteWorkflow(wf.id);

    // Import it back
    const result = await apiClient.importWorkflows(seedData.workspaceId, yaml);
    expect(result.created).toContain("Roundtrip Test");

    // Verify reimported structure
    const { workflows } = await apiClient.listWorkflows(seedData.workspaceId);
    const reimported = workflows.find((w) => w.name === "Roundtrip Test");
    expect(reimported).toBeDefined();

    const { steps: reimportedSteps } = await apiClient.listWorkflowSteps(reimported!.id);

    // Same number of steps
    expect(reimportedSteps).toHaveLength(updatedSteps.length);

    // Same step names in same order
    const originalNames = updatedSteps.sort((a, b) => a.position - b.position).map((s) => s.name);
    const reimportedNames = reimportedSteps
      .sort((a, b) => a.position - b.position)
      .map((s) => s.name);
    expect(reimportedNames).toEqual(originalNames);

    // Custom prompt preserved
    const reimportedPlan = reimportedSteps.find((s) => s.name === "Plan");
    expect(reimportedPlan?.prompt).toContain("Custom plan prompt for roundtrip");
  });

  test("import rejects invalid YAML", async ({ backend, seedData }) => {
    const res = await fetch(
      `${backend.baseUrl}/api/v1/workspaces/${seedData.workspaceId}/workflows/import`,
      {
        method: "POST",
        headers: { "Content-Type": "application/x-yaml" },
        body: "this: is: not: [valid yaml",
      },
    );
    expect(res.status).toBe(400);
  });
});

test.describe("Seed protection", () => {
  // Backend restart can be flaky
  test.describe.configure({ retries: 1 });

  test("backend restart preserves user-customized workflows", async ({
    apiClient,
    seedData,
    backend,
  }) => {
    // 1. Create workflows from templates
    const kanbanWf = await apiClient.createWorkflow(seedData.workspaceId, "My Kanban", "simple");
    const prReviewWf = await apiClient.createWorkflow(
      seedData.workspaceId,
      "My PR Review",
      "pr-review",
    );

    // 2. Customize the Kanban workflow — rename "Review" step and set a custom prompt
    const { steps: kanbanSteps } = await apiClient.listWorkflowSteps(kanbanWf.id);
    const reviewStep = kanbanSteps.find((s) => s.name === "Review");
    expect(reviewStep).toBeDefined();
    await apiClient.updateWorkflowStep(reviewStep!.id, {
      prompt: "Custom QA review prompt",
    });

    // 3. Customize the PR Review workflow — set a custom review prompt
    const { steps: prSteps } = await apiClient.listWorkflowSteps(prReviewWf.id);
    const prReviewStep = prSteps.find((s) => s.name === "Review");
    expect(prReviewStep).toBeDefined();
    await apiClient.updateWorkflowStep(prReviewStep!.id, {
      prompt: "My custom PR review instructions",
    });

    // 4. Record state before restart
    const { steps: preRestartKanban } = await apiClient.listWorkflowSteps(kanbanWf.id);
    const { steps: preRestartPR } = await apiClient.listWorkflowSteps(prReviewWf.id);
    const { templates: preRestartTemplates } = await apiClient.listWorkflowTemplates();

    // 5. Restart the backend — this triggers seed/init again
    await backend.restart();

    // 6. Verify customizations survived the restart
    const { steps: postRestartKanban } = await apiClient.listWorkflowSteps(kanbanWf.id);
    const postReviewStep = postRestartKanban.find((s) => s.id === reviewStep!.id);
    expect(postReviewStep).toBeDefined();
    expect(postReviewStep!.prompt).toBe("Custom QA review prompt");

    const { steps: postRestartPR } = await apiClient.listWorkflowSteps(prReviewWf.id);
    const postPRReviewStep = postRestartPR.find((s) => s.id === prReviewStep!.id);
    expect(postPRReviewStep).toBeDefined();
    expect(postPRReviewStep!.prompt).toBe("My custom PR review instructions");

    // 7. Same number of steps (no duplication or loss)
    expect(postRestartKanban).toHaveLength(preRestartKanban.length);
    expect(postRestartPR).toHaveLength(preRestartPR.length);

    // 8. System templates still exist
    const { templates: postRestartTemplates } = await apiClient.listWorkflowTemplates();
    expect(postRestartTemplates.length).toBeGreaterThanOrEqual(preRestartTemplates.length);
  });
});
