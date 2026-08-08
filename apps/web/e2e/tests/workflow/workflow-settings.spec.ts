import type { Locator } from "@playwright/test";
import { test, expect } from "../../fixtures/test-base";
import { WorkflowSettingsPage } from "../../pages/workflow-settings-page";

async function maxRingSpread(locator: Locator): Promise<number> {
  const boxShadow = await locator.evaluate((element) => getComputedStyle(element).boxShadow);
  const spreads = Array.from(boxShadow.matchAll(/0px 0px 0px ([\d.]+)px/g), (match) =>
    Number(match[1]),
  );
  return Math.max(0, ...spreads);
}

test.describe("Workflow settings", () => {
  test("hides system-only templates from the add workflow dialog", async ({
    testPage,
    seedData,
  }) => {
    const page = new WorkflowSettingsPage(testPage);
    await page.goto(seedData.workspaceId);

    await page.addWorkflowButton.click();
    await expect(page.createDialog).toBeVisible();
    await expect(page.createDialog.getByText("Office Default", { exact: true })).toHaveCount(0);
    await expect(page.createDialog.getByText("Routine", { exact: true })).toHaveCount(0);
  });

  test("displays existing workflows on the settings page", async ({ testPage, seedData }) => {
    const page = new WorkflowSettingsPage(testPage);
    await page.goto(seedData.workspaceId);

    // The seeded "E2E Workflow" should be visible
    const card = await page.findWorkflowCard("E2E Workflow");
    await expect(card).toBeVisible();

    // Should display workflow steps from the "simple" template
    for (const step of seedData.steps) {
      await expect(card.getByText(step.name)).toBeVisible();
    }
  });

  test("keeps turn-complete policy controls aligned", async ({ testPage, apiClient, seedData }) => {
    const workflow = await apiClient.createWorkflow(seedData.workspaceId, "Turn Complete Layout");
    await apiClient.createWorkflowStep(workflow.id, "Inbox", 0, { is_start_step: true });
    const working = await apiClient.createWorkflowStep(workflow.id, "Working", 1);
    const done = await apiClient.createWorkflowStep(workflow.id, "Done", 2);
    await apiClient.updateWorkflowStep(working.id, {
      events: {
        on_turn_complete: [{ type: "move_to_step", config: { step_id: done.id } }],
      },
    });

    const page = new WorkflowSettingsPage(testPage);
    await testPage.setViewportSize({ width: 1920, height: 1080 });
    await page.goto(seedData.workspaceId);
    const card = await page.findWorkflowCard("Turn Complete Layout");
    const panel = await page.selectStep(card, "Working");
    const signalRow = panel.getByTestId(`${working.id}-require-signal-row`);
    const cancelRow = panel.getByTestId(`${working.id}-cancel-completion-row`);
    const label = panel.getByTestId(`${working.id}-cancel-completion-label`);
    const helpTip = panel.getByTestId(`${working.id}-cancel-completion-help`);
    const [signalRowBox, cancelRowBox, labelBox, helpBox] = await Promise.all([
      signalRow.boundingBox(),
      cancelRow.boundingBox(),
      label.boundingBox(),
      helpTip.boundingBox(),
    ]);

    expect(signalRowBox).not.toBeNull();
    expect(cancelRowBox).not.toBeNull();
    expect(labelBox).not.toBeNull();
    expect(helpBox).not.toBeNull();
    expect(cancelRowBox!.height).toBeCloseTo(signalRowBox!.height, 1);
    expect(helpBox!.x - (labelBox!.x + labelBox!.width)).toBeGreaterThanOrEqual(0);
    expect(helpBox!.x - (labelBox!.x + labelBox!.width)).toBeLessThanOrEqual(12);
  });

  test("configures the original session with the shared model settings picker", async ({
    testPage,
    backend,
    apiClient,
    seedData,
    prCapture,
  }) => {
    // The host-utility probe runs asynchronously during backend startup. Wait
    // for the deterministic mock provider used by this spec instead of
    // selecting whichever provider happens to be first in the response.
    await expect
      .poll(
        async () => {
          const available = await testPage.request.get(
            `${backend.baseUrl}/api/v1/agents/available`,
          );
          if (!available.ok()) return false;
          const payload = (await available.json()) as {
            agents?: Array<{
              name: string;
              model_config?: { config_options?: Array<{ id: string }> };
            }>;
          };
          const mock = payload.agents?.find((item) => item.name === "mock-agent");
          return Boolean(
            mock?.model_config?.config_options?.some((option) => option.id === "effort"),
          );
        },
        { timeout: 20_000, intervals: [250, 500, 1_000] },
      )
      .toBe(true);

    const available = await testPage.request.get(`${backend.baseUrl}/api/v1/agents/available`);
    expect(available.ok()).toBe(true);
    const availablePayload = (await available.json()) as {
      agents?: Array<{ name: string }>;
    };
    const agent = availablePayload.agents?.find((item) => item.name === "mock-agent");
    expect(agent).toBeDefined();

    const workflow = await apiClient.createWorkflow(
      seedData.workspaceId,
      "Conditional Session Settings",
    );
    const workStep = await apiClient.createWorkflowStep(workflow.id, "Work", 0, {
      is_start_step: true,
    });

    const page = new WorkflowSettingsPage(testPage);
    await page.goto(seedData.workspaceId);
    const card = await page.findWorkflowCard("Conditional Session Settings");
    await expect(card).toBeVisible();
    await page.selectStep(card, "Work");

    const agentProfileHelpId = `${workStep.id}-agent-profile-help`;
    const originalSessionHelpId = `${workStep.id}-override-original-session-help`;
    const helpOrder = await card
      .locator(`[data-testid="${agentProfileHelpId}"], [data-testid="${originalSessionHelpId}"]`)
      .evaluateAll((elements) => elements.map((element) => element.getAttribute("data-testid")));
    expect(helpOrder).toEqual([agentProfileHelpId, originalSessionHelpId]);

    await card.getByLabel("Override original session options").click();
    const editor = card.getByTestId(`${workStep.id}-session-config-editor`);
    await expect(editor.getByTestId("session-config-rule-0")).toBeVisible();
    await expect(page.stepAgentProfileSelect(card)).toBeDisabled();

    const settings = editor.getByRole("button", { name: `Settings for ${agent!.name}` });
    await settings.click();
    await testPage.getByText("Mock Smart", { exact: true }).click();
    await testPage.getByTestId("config-option-trigger-effort").click();
    await testPage.getByRole("button", { name: "Max", exact: true }).click();
    await testPage.keyboard.press("Escape");
    await prCapture.screenshot("desktop-original-session-editor", {
      caption: "Workflow step editor with a conditional original-session model and effort rule.",
    });

    await page.saveChanges();
    const { steps } = await apiClient.listWorkflowSteps(workflow.id);
    expect(steps.find((step) => step.id === workStep.id)?.events?.on_enter).toEqual([
      {
        type: "configure_session",
        config: {
          rules: [
            {
              agent_name: agent!.name,
              operation: "set",
              model: "mock-smart",
              config_options: { effort: "max" },
            },
          ],
        },
      },
    ]);
  });

  test("creates a workflow from template only after Save", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const page = new WorkflowSettingsPage(testPage);
    await page.goto(seedData.workspaceId);

    // Select a known template so the step-level assertions do not depend on template ordering.
    await page.createWorkflow("Template Test Workflow", "Kanban");

    // Verify the new card appears
    const card = await page.findWorkflowCard("Template Test Workflow");
    await expect(card).toBeVisible();
    await expect(card).toHaveAttribute("data-settings-dirty", "true");
    await expect(card.locator("input").first()).toHaveAttribute("data-settings-dirty", "true");
    await expect(page.stepNodeByName(card, "Backlog")).toHaveAttribute(
      "data-settings-dirty",
      "true",
    );

    const beforeSave = await apiClient.listWorkflows(seedData.workspaceId);
    expect(
      beforeSave.workflows.some((workflow) => workflow.name === "Template Test Workflow"),
    ).toBe(false);
    await expect(page.floatingSave).toBeVisible();
    await page.saveChanges();

    const savedCard = await page.findWorkflowCard("Template Test Workflow");
    await expect(savedCard).toHaveAttribute("data-settings-dirty", "false");
    await expect(page.stepNodeByName(savedCard, "Backlog")).toHaveAttribute(
      "data-settings-dirty",
      "false",
    );

    await page.goto(seedData.workspaceId);
    const reloadedCard = await page.findWorkflowCard("Template Test Workflow");
    await expect(reloadedCard).toBeVisible();
  });

  test("creates a custom workflow without template", async ({ testPage, seedData }) => {
    const page = new WorkflowSettingsPage(testPage);
    await page.goto(seedData.workspaceId);

    await page.createWorkflow("Custom Test Workflow", "Custom");

    const card = await page.findWorkflowCard("Custom Test Workflow");
    await expect(card).toBeVisible();

    // Custom workflows get default steps (Todo, In Progress, Review, Done)
    await expect(card.getByText("Todo")).toBeVisible();
    await expect(card.getByText("In Progress")).toBeVisible();
    await expect(card.getByText("Review")).toBeVisible();
    await expect(card.getByText("Done")).toBeVisible();

    await page.saveChanges();
    await page.goto(seedData.workspaceId);
    const reloadedCard = await page.findWorkflowCard("Custom Test Workflow");
    await expect(reloadedCard).toBeVisible();
  });

  test("adds a step locally and persists it with Save", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const page = new WorkflowSettingsPage(testPage);
    await page.goto(seedData.workspaceId);

    // Create workflow from template
    await page.createWorkflow("Step Add Test");

    const card = await page.findWorkflowCard("Step Add Test");
    await expect(card).toBeVisible();
    await page.saveChanges();

    const persistedCard = await page.findWorkflowCard("Step Add Test");
    const workflows = await apiClient.listWorkflows(seedData.workspaceId);
    const workflow = workflows.workflows.find((item) => item.name === "Step Add Test");
    expect(workflow).toBeDefined();
    const stepsBefore = await apiClient.listWorkflowSteps(workflow!.id);

    await page.addStepButton(persistedCard).click();

    await expect(persistedCard.getByText("New Step")).toBeVisible();
    expect((await apiClient.listWorkflowSteps(workflow!.id)).steps).toHaveLength(
      stepsBefore.steps.length,
    );

    await page.saveChanges();

    await page.goto(seedData.workspaceId);
    const reloadedCard = await page.findWorkflowCard("Step Add Test");
    await expect(reloadedCard).toBeVisible();
    await expect(reloadedCard.getByText("New Step")).toBeVisible();
  });

  test("configures an all child tasks complete transition", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const workflow = await apiClient.createWorkflow(
      seedData.workspaceId,
      "Child Completion Settings",
    );
    const waitStep = await apiClient.createWorkflowStep(workflow.id, "Waiting for Children", 0);
    const doneStep = await apiClient.createWorkflowStep(workflow.id, "All Children Done", 1);

    const page = new WorkflowSettingsPage(testPage);
    await page.goto(seedData.workspaceId);

    const card = await page.findWorkflowCard("Child Completion Settings");
    await expect(card).toBeVisible();
    await page.stepNodeByName(card, "Waiting for Children").click();

    await card.getByTestId(`${waitStep.id}-children-completed-help`).hover();
    await expect(
      testPage.getByText("When every active direct child task is COMPLETED, FAILED, or CANCELLED"),
    ).toBeVisible();

    await card.getByTestId(`${waitStep.id}-children-completed-transition-select`).click();
    await testPage.getByRole("option", { name: "Move to specific step" }).click();
    await expect(card.getByTestId(`${waitStep.id}-children-completed-step-select`)).toContainText(
      "All Children Done",
    );

    const beforeSave = await apiClient.listWorkflowSteps(workflow.id);
    expect(
      beforeSave.steps.find((step) => step.id === waitStep.id)?.events?.on_children_completed,
    ).toBeUndefined();

    await page.saveChanges();
    const afterSave = await apiClient.listWorkflowSteps(workflow.id);
    expect(
      afterSave.steps.find((step) => step.id === waitStep.id)?.events?.on_children_completed,
    ).toEqual([{ type: "move_to_step", config: { step_id: doneStep.id } }]);
  });

  test("configures WIP limit and feeder step", async ({ testPage, apiClient, seedData }) => {
    const workflow = await apiClient.createWorkflow(seedData.workspaceId, "WIP Settings");
    const backlogStep = await apiClient.createWorkflowStep(workflow.id, "Backlog", 0);
    const reviewStep = await apiClient.createWorkflowStep(workflow.id, "Review", 1);

    const page = new WorkflowSettingsPage(testPage);
    await page.goto(seedData.workspaceId);

    const card = await page.findWorkflowCard("WIP Settings");
    await expect(card).toBeVisible();
    await page.stepNodeByName(card, "Review").click();

    await card.getByTestId(`${reviewStep.id}-wip-limit-input`).fill("2");
    await card.getByTestId(`${reviewStep.id}-pull-from-step-select`).click();
    await testPage.getByRole("option", { name: "Backlog" }).click();

    expect(
      (await apiClient.listWorkflowSteps(workflow.id)).steps.find(
        (step) => step.id === reviewStep.id,
      ),
    ).not.toMatchObject({ wip_limit: 2, pull_from_step_id: backlogStep.id });

    await page.saveChanges();
    expect(
      (await apiClient.listWorkflowSteps(workflow.id)).steps.find(
        (step) => step.id === reviewStep.id,
      ),
    ).toMatchObject({
      wip_limit: 2,
      pull_from_step_id: backlogStep.id,
    });
  });

  test("modifies a step name only after Save", async ({ testPage, apiClient, seedData }) => {
    const workflow = await apiClient.createWorkflow(seedData.workspaceId, "Step Rename Workflow");
    const step = await apiClient.createWorkflowStep(workflow.id, "Original Step", 0);
    const page = new WorkflowSettingsPage(testPage);
    await page.goto(seedData.workspaceId);

    const card = await page.findWorkflowCard(workflow.name);
    await expect(card).toBeVisible();

    // Click on the first step to open config panel
    const stepNode = page.stepNodeByName(card, step.name);
    await stepNode.click();

    // Find the step name input in the config panel and rename it
    const nameInput = card.getByPlaceholder("Step name");
    await nameInput.fill("Renamed Step");

    await expect(nameInput).toHaveAttribute("data-settings-dirty", "true");
    await expect(card).toHaveAttribute("data-settings-dirty", "true");
    const stepPanel = card.getByTestId(`workflow-step-panel-${step.id}`);
    const dirtyStepNode = card.getByTestId(`workflow-step-node-${step.id}`);

    await expect(stepPanel).toHaveAttribute("data-settings-dirty", "true");
    await expect(card).toHaveAttribute("data-settings-dirty-level", "card");
    await expect(stepPanel).toHaveAttribute("data-settings-dirty-level", "container");
    await expect(dirtyStepNode).toHaveAttribute("data-settings-dirty", "true");
    await expect(dirtyStepNode).toHaveAttribute("data-settings-dirty-level", "container");
    await nameInput.blur();
    expect(await maxRingSpread(nameInput)).toBeGreaterThan(0);
    expect(await maxRingSpread(card)).toBe(0);
    expect(await maxRingSpread(stepPanel)).toBe(0);
    expect(await maxRingSpread(dirtyStepNode)).toBe(0);

    expect((await apiClient.listWorkflowSteps(workflow.id)).steps[0]?.name).toBe(step.name);
    await page.saveChanges();

    await expect(nameInput).toHaveAttribute("data-settings-dirty", "false");
    await expect(card).toHaveAttribute("data-settings-dirty", "false");

    await page.goto(seedData.workspaceId);
    const reloadedCard = await page.findWorkflowCard(workflow.name);
    await expect(reloadedCard).toBeVisible();
    await expect(reloadedCard.getByText("Renamed Step")).toBeVisible();
  });

  test("shows delete confirmation dialog when removing a persisted step", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const workflow = await apiClient.createWorkflow(seedData.workspaceId, "Delete Step Test");
    await apiClient.createWorkflowStep(workflow.id, "Keep", 0);
    await apiClient.createWorkflowStep(workflow.id, "Review", 1);
    const page = new WorkflowSettingsPage(testPage);
    await page.goto(seedData.workspaceId);

    const card = await page.findWorkflowCard("Delete Step Test");
    await expect(card).toBeVisible();

    // Try to delete the "Review" step via hovering and clicking trash
    await page.clickDeleteStepButton(card, "Review");

    // Confirmation dialog should appear
    await expect(page.stepDeleteDialog).toBeVisible();
    await expect(page.stepDeleteDialog.getByText("Review")).toBeVisible();

    // Cancel — step should still exist
    await page.stepDeleteDialog.getByRole("button", { name: "Cancel" }).click();
    await expect(page.stepDeleteDialog).not.toBeVisible();
    await expect(card.getByText("Review")).toBeVisible();

    // Delete again and confirm
    await page.clickDeleteStepButton(card, "Review");
    await expect(page.stepDeleteDialog).toBeVisible();
    await page.stepDeleteDialog.getByRole("button", { name: "Delete Step", exact: true }).click();
    await expect(page.stepDeleteDialog).not.toBeVisible();

    // Step should be removed
    await expect(page.stepNodeByName(card, "Review")).not.toBeVisible();
  });

  test("deletes a workflow", async ({ testPage, seedData }) => {
    const page = new WorkflowSettingsPage(testPage);
    await page.goto(seedData.workspaceId);

    await page.createWorkflow("To Delete Workflow", "Custom");
    await page.saveChanges();

    const card = await page.findWorkflowCard("To Delete Workflow");
    await expect(card).toBeVisible();

    // Click delete workflow
    await page.deleteWorkflowButton(card).click();

    // The delete dialog should appear — confirm deletion
    const deleteDialog = testPage.getByRole("dialog").filter({ hasText: "Delete" });
    await expect(deleteDialog).toBeVisible();
    // Click the delete button (it will say "Delete" or "Delete Workflow")
    await deleteDialog
      .getByRole("button", { name: /delete/i })
      .last()
      .click();

    // Workflow card should be removed
    const deletedCard = await page.findWorkflowCard("To Delete Workflow");
    await expect(deletedCard).not.toBeVisible();
  });

  test("keeps workflow details local until the route-level Save", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const workflow = await apiClient.createWorkflow(seedData.workspaceId, "Workflow Detail Save");
    const page = new WorkflowSettingsPage(testPage);
    await page.goto(seedData.workspaceId);

    const card = await page.findWorkflowCard(workflow.name);
    const nameInput = card.locator("input").first();
    await nameInput.fill("Manually Saved Workflow Name");

    await expect(nameInput).toHaveAttribute("data-settings-dirty", "true");
    await expect(card).toHaveAttribute("data-settings-dirty", "true");

    expect(
      (await apiClient.listWorkflows(seedData.workspaceId)).workflows.find(
        (candidate) => candidate.id === workflow.id,
      )?.name,
    ).toBe(workflow.name);
    await expect(page.floatingSave).toBeVisible();
    await page.saveChanges();

    await expect(nameInput).toHaveAttribute("data-settings-dirty", "false");
    await expect(card).toHaveAttribute("data-settings-dirty", "false");

    await page.goto(seedData.workspaceId);
    await expect(await page.findWorkflowCard("Manually Saved Workflow Name")).toBeVisible();
  });

  test("edits and persists a workflow description", async ({ testPage, apiClient, seedData }) => {
    const workflow = await apiClient.createWorkflow(
      seedData.workspaceId,
      "Workflow Description Save",
    );
    const page = new WorkflowSettingsPage(testPage);
    await page.goto(seedData.workspaceId);

    const card = await page.findWorkflowCard(workflow.name);
    const descriptionInput = card.getByLabel("Description", { exact: true });
    await expect(descriptionInput).toBeVisible();
    await expect(descriptionInput).toBeEnabled();
    await descriptionInput.fill("rev 1 (2026-08-08) — test description");

    await expect(descriptionInput).toHaveAttribute("data-settings-dirty", "true");

    expect(
      (await apiClient.listWorkflows(seedData.workspaceId)).workflows.find(
        (candidate) => candidate.id === workflow.id,
      )?.description,
    ).toBeFalsy();
    await page.saveChanges();

    await expect(descriptionInput).toHaveAttribute("data-settings-dirty", "false");

    await page.goto(seedData.workspaceId);
    const reloadedCard = await page.findWorkflowCard(workflow.name);
    await expect(reloadedCard.getByTestId("workflow-description-input")).toHaveValue(
      "rev 1 (2026-08-08) — test description",
    );
  });

  test("syncs a cross-tab description/prompt update live without clobbering an unsaved edit", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    // Regression coverage for the WS store-sync effect in use-workflow-settings.ts:
    // it used to only propagate `name` from workflow.updated events, leaving
    // description/prompt/agent_profile_id stale on an open settings page until a
    // full reload. Simulates "another tab" by mutating via the API (which fires the
    // real workflow.updated WS event) while this page stays open — no page.goto.
    const workflow = await apiClient.createWorkflow(seedData.workspaceId, "Cross-Tab Sync Test");
    await apiClient.updateWorkflow(workflow.id, { description: "Old description" });

    const page = new WorkflowSettingsPage(testPage);
    await page.goto(seedData.workspaceId);
    const card = await page.findWorkflowCard(workflow.name);
    const descriptionInput = card.getByLabel("Description", { exact: true });
    await expect(descriptionInput).toHaveValue("Old description");

    // Local, unsaved edit to a different field (name) — must survive the cross-tab sync below.
    const nameInput = card.locator("input").first();
    await nameInput.fill("Unsaved local name");
    await expect(nameInput).toHaveAttribute("data-settings-dirty", "true");

    // "Another tab" edits description and prompt via the API.
    await apiClient.updateWorkflow(workflow.id, {
      description: "New description from another tab",
      prompt: "New prompt from another tab",
    });

    // Description and prompt update live via WS, with no local draft on either field.
    await expect(descriptionInput).toHaveValue("New description from another tab");
    await expect(descriptionInput).toHaveAttribute("data-settings-dirty", "false");
    const promptInput = card.getByTestId("workflow-prompt-input");
    await expect(promptInput).toBeVisible();
    await expect(promptInput).toHaveValue("New prompt from another tab");
    await expect(promptInput).toHaveAttribute("data-settings-dirty", "false");

    // The unsaved local name edit was not clobbered by the cross-tab sync.
    await expect(nameInput).toHaveValue("Unsaved local name");
    await expect(nameInput).toHaveAttribute("data-settings-dirty", "true");

    // The saved-baseline is what future dirty checks compare against, so saving now
    // must persist the local name edit alongside the already-synced description/prompt.
    await page.saveChanges();
    const persisted = (await apiClient.listWorkflows(seedData.workspaceId)).workflows.find(
      (candidate) => candidate.id === workflow.id,
    );
    expect(persisted?.name).toBe("Unsaved local name");
    expect(persisted?.description).toBe("New description from another tab");
    expect(persisted?.prompt).toBe("New prompt from another tab");
  });
});

test.describe("Seed protection", () => {
  // Backend restart can be flaky
  test.describe.configure({ retries: 1 });

  test("backend restart preserves user-customized workflows visible in UI", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    // 1. Create workflows from templates via API
    const kanbanWf = await apiClient.createWorkflow(seedData.workspaceId, "My Kanban", "simple");
    const prReviewWf = await apiClient.createWorkflow(
      seedData.workspaceId,
      "My PR Review",
      "pr-review",
    );

    // 2. Customize via API — set custom prompts
    const { steps: kanbanSteps } = await apiClient.listWorkflowSteps(kanbanWf.id);
    const reviewStep = kanbanSteps.find((s) => s.name === "Review");
    expect(reviewStep).toBeDefined();
    await apiClient.updateWorkflowStep(reviewStep!.id, {
      prompt: "Custom QA review prompt",
    });

    const { steps: prSteps } = await apiClient.listWorkflowSteps(prReviewWf.id);
    const prReviewStep = prSteps.find((s) => s.name === "Review");
    expect(prReviewStep).toBeDefined();
    await apiClient.updateWorkflowStep(prReviewStep!.id, {
      prompt: "My custom PR review instructions",
    });

    // 3. Verify workflows are visible in UI before restart
    const page = new WorkflowSettingsPage(testPage);
    await page.goto(seedData.workspaceId);
    await expect(await page.findWorkflowCard("My Kanban")).toBeVisible();
    await expect(await page.findWorkflowCard("My PR Review")).toBeVisible();

    // 4. Restart the backend — triggers seed/init again
    await backend.restart();

    // 5. Reload the page and verify workflows still visible with correct steps
    await page.goto(seedData.workspaceId);
    const kanbanCard = await page.findWorkflowCard("My Kanban");
    await expect(kanbanCard).toBeVisible();
    await expect(kanbanCard.getByText("Backlog")).toBeVisible();
    await expect(kanbanCard.getByText("Review")).toBeVisible();

    const prCard = await page.findWorkflowCard("My PR Review");
    await expect(prCard).toBeVisible();

    // 6. Verify customizations survived via API
    const { steps: postKanban } = await apiClient.listWorkflowSteps(kanbanWf.id);
    const postReview = postKanban.find((s) => s.id === reviewStep!.id);
    expect(postReview).toBeDefined();
    expect(postReview!.prompt).toBe("Custom QA review prompt");

    const { steps: postPR } = await apiClient.listWorkflowSteps(prReviewWf.id);
    const postPRReview = postPR.find((s) => s.id === prReviewStep!.id);
    expect(postPRReview).toBeDefined();
    expect(postPRReview!.prompt).toBe("My custom PR review instructions");

    // 7. Same number of steps (no duplication or loss)
    expect(postKanban).toHaveLength(kanbanSteps.length);
    expect(postPR).toHaveLength(prSteps.length);
  });

  test("hidden system workflows do not appear in the settings list", async ({
    testPage,
    seedData,
    apiClient,
  }) => {
    // Reproduces the original "Improve Kandev" leak: while the user is on
    // the workspace workflow settings page, a hidden system workflow gets
    // created (e.g. via the Improve Kandev dialog). The backend fires a
    // `workflow.created` WS event with hidden=true; the frontend receives
    // it and previously surfaced the entry as a manageable card in the
    // settings list. Verify the new hidden entry never appears as a card.
    const hiddenName = "Improve Kandev";
    const visibleName = `Hidden filter sentinel ${Date.now()}`;
    const visibleWorkflow = await apiClient.createWorkflow(
      seedData.workspaceId,
      visibleName,
      "simple",
    );

    const page = new WorkflowSettingsPage(testPage);
    await page.goto(seedData.workspaceId);

    // Establish a visible control in this test instead of depending on a seed
    // that an earlier restart test may still be restoring.
    const visibleCard = page.workflowCard(visibleWorkflow.id);
    await expect(visibleCard).toBeVisible();
    const baselineCount = await testPage.locator('[data-testid^="workflow-card-"]').count();

    // Trigger the leak path: a hidden workflow is created and the
    // `workflow.created` WS event arrives at the open settings page.
    await apiClient.e2eCreateHiddenWorkflow(seedData.workspaceId, hiddenName);

    // Allow the WS event to propagate and the React effect in
    // useWorkflowSettings a chance to (incorrectly) add a card.
    await testPage.waitForTimeout(500);

    // No new card appeared and the hidden entry is not in the list.
    const allCards = testPage.locator('[data-testid^="workflow-card-"]');
    const newCount = await allCards.count();
    const cardNames: string[] = [];
    for (let i = 0; i < newCount; i++) {
      const value = await allCards
        .nth(i)
        .locator("input")
        .first()
        .inputValue({ timeout: 500 })
        .catch(() => "");
      cardNames.push(value);
    }
    expect(cardNames).not.toContain(hiddenName);
    expect(newCount).toBe(baselineCount);
  });
});
