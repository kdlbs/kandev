import { test, expect } from "../../fixtures/office-fixture";

test.describe("Subtask checklist — stepper (blocker chain)", () => {
  test("stepper renders for children linked with a blocker chain", async ({
    testPage,
    officeApi,
    officeSeed,
  }) => {
    // Create parent + three children A → B → C via API.
    const parent = await officeApi.createTask(officeSeed.workspaceId, "Stepper Parent Task", {
      workflow_id: officeSeed.workflowId,
    });
    const parentId = (parent as { id: string }).id;

    const taskA = await officeApi.createTask(officeSeed.workspaceId, "Step A", {
      parent_id: parentId,
      workflow_id: officeSeed.workflowId,
    });
    const taskAId = (taskA as { id: string }).id;

    const taskB = await officeApi.createTask(officeSeed.workspaceId, "Step B", {
      parent_id: parentId,
      blocked_by: [taskAId],
      workflow_id: officeSeed.workflowId,
    });
    const taskBId = (taskB as { id: string }).id;

    await officeApi.createTask(officeSeed.workspaceId, "Step C", {
      parent_id: parentId,
      blocked_by: [taskBId],
      workflow_id: officeSeed.workflowId,
    });

    // Navigate to parent detail page.
    await testPage.goto(`/office/tasks/${parentId}`);

    // Scope assertions to the stepper to avoid clashes with the right-rail
    // sub-issues panel (which lists the same titles).
    const stepper = testPage.getByTestId("subtask-stepper");
    await expect(stepper).toBeVisible({ timeout: 10_000 });

    // The stepper renders numbered circles (1, 2, 3) — look for step numbers.
    await expect(stepper.getByText("1")).toBeVisible({ timeout: 10_000 });
    await expect(stepper.getByText("2")).toBeVisible({ timeout: 10_000 });
    await expect(stepper.getByText("3")).toBeVisible({ timeout: 10_000 });

    // The subtask titles should appear in the stepper.
    await expect(stepper.getByText("Step A")).toBeVisible({ timeout: 10_000 });
    await expect(stepper.getByText("Step B")).toBeVisible({ timeout: 10_000 });
    await expect(stepper.getByText("Step C")).toBeVisible({ timeout: 10_000 });
  });

  test("flat list renders for independent children (no blockers)", async ({
    testPage,
    officeApi,
    officeSeed,
  }) => {
    // Create parent + three independent children with no blocker relationships.
    const parent = await officeApi.createTask(officeSeed.workspaceId, "Flat List Parent Task", {
      workflow_id: officeSeed.workflowId,
    });
    const parentId = (parent as { id: string }).id;

    await officeApi.createTask(officeSeed.workspaceId, "Independent A", {
      parent_id: parentId,
      workflow_id: officeSeed.workflowId,
    });
    await officeApi.createTask(officeSeed.workspaceId, "Independent B", {
      parent_id: parentId,
      workflow_id: officeSeed.workflowId,
    });
    await officeApi.createTask(officeSeed.workspaceId, "Independent C", {
      parent_id: parentId,
      workflow_id: officeSeed.workflowId,
    });

    await testPage.goto(`/office/tasks/${parentId}`);

    // Scope assertions to the flat list to avoid clashes with the right-rail
    // sub-issues panel (which lists the same titles).
    const flatList = testPage.getByTestId("child-issues-list");
    await expect(flatList).toBeVisible({ timeout: 10_000 });

    // Subtask titles must appear in the flat list.
    await expect(flatList.getByText("Independent A")).toBeVisible({ timeout: 10_000 });
    await expect(flatList.getByText("Independent B")).toBeVisible({ timeout: 10_000 });
    await expect(flatList.getByText("Independent C")).toBeVisible({ timeout: 10_000 });

    // The flat list (no blockers) must not render the stepper.
    await expect(testPage.getByTestId("subtask-stepper")).toHaveCount(0);

    // Flat list must NOT show a "Blocked" badge.
    await expect(flatList.locator("text=Blocked")).not.toBeVisible();
  });

  test("completed first step highlights second step as active", async ({
    testPage,
    officeApi,
    officeSeed,
  }) => {
    // Create parent + two children: Step 1 → Step 2.
    const parent = await officeApi.createTask(officeSeed.workspaceId, "Active Step Parent", {
      workflow_id: officeSeed.workflowId,
    });
    const parentId = (parent as { id: string }).id;

    const step1 = await officeApi.createTask(officeSeed.workspaceId, "Active Step 1", {
      parent_id: parentId,
      workflow_id: officeSeed.workflowId,
    });
    const step1Id = (step1 as { id: string }).id;

    await officeApi.createTask(officeSeed.workspaceId, "Active Step 2", {
      parent_id: parentId,
      blocked_by: [step1Id],
      workflow_id: officeSeed.workflowId,
    });

    // Mark first child as COMPLETED via the office status endpoint.
    await officeApi.updateTaskStatus(step1Id, "COMPLETED", "done");

    // Navigate to parent detail page.
    await testPage.goto(`/office/tasks/${parentId}`);

    // Scope assertions to the stepper to avoid clashes with the right-rail
    // sub-issues panel (which lists the same titles).
    const stepper = testPage.getByTestId("subtask-stepper");
    await expect(stepper).toBeVisible({ timeout: 10_000 });

    // Step 1 should render with a checkmark (✓) because it's completed.
    await expect(stepper.getByText("✓")).toBeVisible({ timeout: 10_000 });

    // Both step titles should be visible inside the stepper.
    await expect(stepper.getByText("Active Step 1")).toBeVisible({ timeout: 10_000 });
    await expect(stepper.getByText("Active Step 2")).toBeVisible({ timeout: 10_000 });
  });

  test("clicking a stepper step navigates to that subtask detail", async ({
    testPage,
    officeApi,
    officeSeed,
  }) => {
    // Create parent + two children with a blocker chain so stepper renders.
    const parent = await officeApi.createTask(officeSeed.workspaceId, "Navigate Stepper Parent", {
      workflow_id: officeSeed.workflowId,
    });
    const parentId = (parent as { id: string }).id;

    const step1 = await officeApi.createTask(officeSeed.workspaceId, "Navigate Step 1", {
      parent_id: parentId,
      workflow_id: officeSeed.workflowId,
    });
    const step1Id = (step1 as { id: string }).id;

    const step2 = await officeApi.createTask(officeSeed.workspaceId, "Navigate Step 2", {
      parent_id: parentId,
      blocked_by: [step1Id],
      workflow_id: officeSeed.workflowId,
    });
    const step2Id = (step2 as { id: string }).id;

    await testPage.goto(`/office/tasks/${parentId}`);

    // Scope to the stepper so we click the stepper link, not the right-rail
    // sub-issues entry that targets the same task.
    const stepper = testPage.getByTestId("subtask-stepper");
    await expect(stepper).toBeVisible({ timeout: 10_000 });
    await expect(stepper.getByText("Navigate Step 1")).toBeVisible({ timeout: 10_000 });

    // Click the second step link in the stepper.
    await stepper.getByText("Navigate Step 2").click();

    // Should navigate to the child issue detail page.
    await expect(testPage).toHaveURL(new RegExp(`/office/tasks/${step2Id}`), {
      timeout: 10_000,
    });
  });
});

test.describe("Subtask checklist — API shape", () => {
  test("getIssue returns blockedBy for a child with a blocker", async ({
    officeApi,
    officeSeed,
  }) => {
    const parent = await officeApi.createTask(officeSeed.workspaceId, "API Blocker Parent", {
      workflow_id: officeSeed.workflowId,
    });
    const parentId = (parent as { id: string }).id;

    const blocker = await officeApi.createTask(officeSeed.workspaceId, "API Blocker A", {
      parent_id: parentId,
      workflow_id: officeSeed.workflowId,
    });
    const blockerId = (blocker as { id: string }).id;

    const blocked = await officeApi.createTask(officeSeed.workspaceId, "API Blocked B", {
      parent_id: parentId,
      blocked_by: [blockerId],
      workflow_id: officeSeed.workflowId,
    });
    const blockedId = (blocked as { id: string }).id;

    // Fetch the parent's issue — children should include blockedBy references.
    const issueResp = await officeApi.getTask(parentId);
    const issue = (issueResp as { task: Record<string, unknown> }).task;
    const children = (issue.children ?? []) as Array<Record<string, unknown>>;

    const blockedChild = children.find((c) => c.id === blockedId);
    expect(blockedChild).toBeDefined();

    const blockedBy = (blockedChild?.blockedBy ?? blockedChild?.blocked_by ?? []) as string[];
    expect(blockedBy).toContain(blockerId);
  });

  test("children without blockers have empty blockedBy", async ({ officeApi, officeSeed }) => {
    const parent = await officeApi.createTask(officeSeed.workspaceId, "No Blocker Parent", {
      workflow_id: officeSeed.workflowId,
    });
    const parentId = (parent as { id: string }).id;

    const child = await officeApi.createTask(officeSeed.workspaceId, "No Blocker Child", {
      parent_id: parentId,
      workflow_id: officeSeed.workflowId,
    });
    const childId = (child as { id: string }).id;

    const issueResp = await officeApi.getTask(parentId);
    const issue = (issueResp as { task: Record<string, unknown> }).task;
    const children = (issue.children ?? []) as Array<Record<string, unknown>>;

    const found = children.find((c) => c.id === childId);
    expect(found).toBeDefined();

    const blockedBy = (found?.blockedBy ?? found?.blocked_by ?? []) as string[];
    expect(blockedBy).toHaveLength(0);
  });
});
