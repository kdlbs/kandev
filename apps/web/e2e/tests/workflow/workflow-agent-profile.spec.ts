import { test, expect } from "../../fixtures/test-base";
import { WorkflowSettingsPage } from "../../pages/workflow-settings-page";
import { KanbanPage } from "../../pages/kanban-page";

test.describe("Workflow agent profile", () => {
  test("set workflow-level agent profile in settings and persist after save", async ({
    testPage,
    seedData,
    apiClient,
  }) => {
    const page = new WorkflowSettingsPage(testPage);
    await page.goto(seedData.workspaceId);

    // Find the seeded "E2E Workflow" card
    const card = await page.findWorkflowCard("E2E Workflow");
    await expect(card).toBeVisible();

    // The "Default Agent Profile" select should initially show "None (use task default)"
    const profileSelect = page.workflowAgentProfileSelect(card);
    await expect(profileSelect).toBeVisible();

    // Get the available agent profiles from the API so we know the label to select
    const { agents } = await apiClient.listAgents();
    const agentProfile = agents.flatMap((a) => a.profiles ?? [])[0];
    expect(agentProfile).toBeDefined();
    const profileLabel = `${agentProfile.agent_display_name} \u2022 ${agentProfile.name}`;

    // Open the select and pick the agent profile
    await profileSelect.click();
    await testPage.getByRole("option", { name: profileLabel }).click();

    // Save the workflow
    await page.saveButton(card).click();
    await testPage.waitForTimeout(1000);

    // Reload and verify the selection persists
    await page.goto(seedData.workspaceId);
    const reloadedCard = await page.findWorkflowCard("E2E Workflow");
    await expect(reloadedCard).toBeVisible();
    const reloadedSelect = page.workflowAgentProfileSelect(reloadedCard);
    await expect(reloadedSelect).toContainText(profileLabel);
  });

  test("set per-step agent profile override in settings", async ({
    testPage,
    seedData,
    apiClient,
  }) => {
    const page = new WorkflowSettingsPage(testPage);
    await page.goto(seedData.workspaceId);

    // Find the seeded workflow card
    const card = await page.findWorkflowCard("E2E Workflow");
    await expect(card).toBeVisible();

    // Click on the first step to open the config panel
    const firstStepName = seedData.steps[0]?.name;
    expect(firstStepName).toBeDefined();
    const stepNode = page.stepNodeByName(card, firstStepName!);
    await stepNode.click();

    // The step config panel should be visible with the "Agent Profile Override" select
    const stepProfileSelect = page.stepAgentProfileSelect(card);
    await expect(stepProfileSelect).toBeVisible();

    // Get agent profile info
    const { agents } = await apiClient.listAgents();
    const agentProfile = agents.flatMap((a) => a.profiles ?? [])[0];
    expect(agentProfile).toBeDefined();
    const profileLabel = `${agentProfile.agent_display_name} \u2022 ${agentProfile.name}`;

    // Select an agent profile for this step
    await stepProfileSelect.click();
    await testPage.getByRole("option", { name: profileLabel }).click();

    // Wait for debounced update to propagate
    await testPage.waitForTimeout(600);

    // Save the workflow
    await page.saveButton(card).click();
    await testPage.waitForTimeout(1000);

    // Reload and verify - the step should now show the agent profile icon (IconUserCog)
    await page.goto(seedData.workspaceId);
    const reloadedCard = await page.findWorkflowCard("E2E Workflow");
    await expect(reloadedCard).toBeVisible();

    // The step node should have a UserCog icon indicating a custom agent profile
    const reloadedStepNode = page.stepNodeByName(reloadedCard, firstStepName!);
    await expect(reloadedStepNode.locator(".tabler-icon-user-cog")).toBeVisible();
  });

  test("task creation dialog locks agent selector when workflow has agent profile", async ({
    testPage,
    seedData,
    apiClient,
  }) => {
    // Set an agent profile on the seeded workflow via API
    const { agents } = await apiClient.listAgents();
    const agentProfile = agents.flatMap((a) => a.profiles ?? [])[0];
    expect(agentProfile).toBeDefined();
    await apiClient.updateWorkflow(seedData.workflowId, {
      agent_profile_id: agentProfile.id,
    });

    let noProfileWorkflowId: string | undefined;
    try {
      // Create a second workflow without an agent profile for comparison
      const noProfileWorkflow = await apiClient.createWorkflow(
        seedData.workspaceId,
        "No Profile Workflow",
        "simple",
      );
      noProfileWorkflowId = noProfileWorkflow.id;

      // Open the kanban page — reload to pick up the updated workflow agent_profile_id
      const kanban = new KanbanPage(testPage);
      await kanban.goto();
      await testPage.reload();

      await kanban.createTaskButton.first().click();
      const dialog = testPage.getByTestId("create-task-dialog");
      await expect(dialog).toBeVisible();

      // Fill title so the selectors become visible
      await testPage.getByTestId("task-title-input").fill("Agent Lock Test");
      await testPage.getByTestId("task-description-input").fill("testing agent lock");

      // The seeded workflow should be selected by default (from user settings).
      // The agent selector should be disabled because the workflow has an agent profile.
      const agentSelector = testPage.getByTestId("agent-profile-selector");
      await expect(agentSelector).toBeVisible({ timeout: 15_000 });

      // "Agent set by workflow" text confirms the selector is locked — sufficient verification
      await expect(testPage.getByText("Agent set by workflow")).toBeVisible({ timeout: 10_000 });
    } finally {
      // Always clean up, even if assertions fail
      if (noProfileWorkflowId) {
        await apiClient.deleteWorkflow(noProfileWorkflowId).catch(() => {});
      }
      await apiClient.updateWorkflow(seedData.workflowId, {
        agent_profile_id: "",
      });
    }
  });

  test("single workflow with agent override enables Start task button and shows profile", async ({
    testPage,
    seedData,
    apiClient,
  }) => {
    // Set an agent profile on the only workflow
    const { agents } = await apiClient.listAgents();
    const agentProfile = agents.flatMap((a) => a.profiles ?? [])[0];
    expect(agentProfile).toBeDefined();
    await apiClient.updateWorkflow(seedData.workflowId, {
      agent_profile_id: agentProfile.id,
    });

    try {
      // Verify the API actually has the agent_profile_id set
      const { workflows: wfList } = await apiClient.listWorkflows(seedData.workspaceId);
      const updatedWf = wfList.find((w) => w.id === seedData.workflowId);
      expect(updatedWf?.agent_profile_id).toBe(agentProfile.id);

      const kanban = new KanbanPage(testPage);
      await kanban.goto();
      // Reload to pick up the updated workflow; wait for network to settle
      await testPage.reload({ waitUntil: "networkidle" });

      await kanban.createTaskButton.first().click();
      const dialog = testPage.getByTestId("create-task-dialog");
      await expect(dialog).toBeVisible();

      await testPage.getByTestId("task-title-input").fill("Single Workflow Test");
      await testPage.getByTestId("task-description-input").fill("testing single workflow");

      // The selector should be disabled with "Agent set by workflow" text
      await expect(testPage.getByText("Agent set by workflow")).toBeVisible({ timeout: 10_000 });

      // The agent selector should be disabled (locked by workflow)
      const agentSelector = testPage.getByTestId("agent-profile-selector");
      await expect(agentSelector).toBeDisabled({ timeout: 10_000 });
    } finally {
      await apiClient.updateWorkflow(seedData.workflowId, {
        agent_profile_id: "",
      });
    }
  });

  test("workflow selector shows agent icon for workflow-level override", async ({
    testPage,
    seedData,
    apiClient,
  }) => {
    const { agents } = await apiClient.listAgents();
    const agentProfile = agents.flatMap((a) => a.profiles ?? [])[0];
    expect(agentProfile).toBeDefined();

    // Set agent profile on seeded workflow
    await apiClient.updateWorkflow(seedData.workflowId, {
      agent_profile_id: agentProfile.id,
    });

    let noProfileWorkflowId: string | undefined;
    try {
      // Create second workflow without agent profile so selector is visible
      const noProfileWorkflow = await apiClient.createWorkflow(
        seedData.workspaceId,
        "Plain Workflow",
        "simple",
      );
      noProfileWorkflowId = noProfileWorkflow.id;

      const kanban = new KanbanPage(testPage);
      await kanban.goto();
      await testPage.reload({ waitUntil: "networkidle" });

      await kanban.createTaskButton.first().click();
      const dialog = testPage.getByTestId("create-task-dialog");
      await expect(dialog).toBeVisible();

      await testPage.getByTestId("task-title-input").fill("Icon Test");

      // Open workflow selector
      const workflowButton = dialog.locator("button", { hasText: "E2E Workflow" });
      await expect(workflowButton).toBeVisible({ timeout: 10_000 });
      await workflowButton.click();

      // The workflow with agent override should have an agent logo
      const agentLogo = testPage.getByTestId("workflow-agent-logo");
      await expect(agentLogo.first()).toBeVisible();
    } finally {
      if (noProfileWorkflowId) {
        await apiClient.deleteWorkflow(noProfileWorkflowId).catch(() => {});
      }
      await apiClient.updateWorkflow(seedData.workflowId, {
        agent_profile_id: "",
      });
    }
  });

  test("step-level agent override shown in workflow selector", async ({
    testPage,
    seedData,
    apiClient,
  }) => {
    const { agents } = await apiClient.listAgents();
    const agentProfile = agents.flatMap((a) => a.profiles ?? [])[0];
    expect(agentProfile).toBeDefined();

    // Set agent profile on the first step
    const firstStep = seedData.steps[0];
    expect(firstStep).toBeDefined();
    await apiClient.updateWorkflowStep(firstStep.id, {
      agent_profile_id: agentProfile.id,
    });

    let extraWorkflowId: string | undefined;
    try {
      // Create second workflow so selector is visible
      const extraWorkflow = await apiClient.createWorkflow(
        seedData.workspaceId,
        "Extra Workflow",
        "simple",
      );
      extraWorkflowId = extraWorkflow.id;

      const kanban = new KanbanPage(testPage);
      await kanban.goto();
      await testPage.reload({ waitUntil: "networkidle" });

      await kanban.createTaskButton.first().click();
      const dialog = testPage.getByTestId("create-task-dialog");
      await expect(dialog).toBeVisible();

      await testPage.getByTestId("task-title-input").fill("Step Override Test");

      // Open workflow selector
      const workflowButton = dialog.locator("button", { hasText: "E2E Workflow" });
      await expect(workflowButton).toBeVisible({ timeout: 10_000 });
      await workflowButton.click();

      // The step with agent override should show an agent logo
      const stepAgentLogo = testPage.getByTestId("step-agent-logo");
      await expect(stepAgentLogo.first()).toBeVisible({ timeout: 10_000 });
    } finally {
      if (extraWorkflowId) {
        await apiClient.deleteWorkflow(extraWorkflowId).catch(() => {});
      }
      await apiClient.updateWorkflowStep(firstStep.id, {
        agent_profile_id: "",
      });
    }
  });
});
