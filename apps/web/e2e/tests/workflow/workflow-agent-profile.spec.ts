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
    const agentProfile = agents[0]?.profiles[0];
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
    const agentProfile = agents[0]?.profiles[0];
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
    const agentProfile = agents[0]?.profiles[0];
    expect(agentProfile).toBeDefined();
    await apiClient.updateWorkflow(seedData.workflowId, {
      agent_profile_id: agentProfile.id,
    });

    try {
      // Create a second workflow without an agent profile for comparison
      const noProfileWorkflow = await apiClient.createWorkflow(
        seedData.workspaceId,
        "No Profile Workflow",
        "simple",
      );

      // Open the kanban page and create task dialog
      const kanban = new KanbanPage(testPage);
      await kanban.goto();

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

      // Wait for the agent profile lock to take effect
      await expect(testPage.getByText("Agent set by workflow")).toBeVisible({ timeout: 10_000 });

      // The selector trigger button should be disabled
      const selectorButton = agentSelector.getByRole("button");
      await expect(selectorButton).toBeDisabled();

      // Switch to the workflow without an agent profile
      // Click the workflow selector to open the dropdown
      const workflowButton = dialog.locator("button").filter({ hasText: "E2E Workflow" });
      await workflowButton.click();

      // Select the "No Profile Workflow"
      await testPage.getByText("No Profile Workflow").click();

      // The agent selector should now be enabled
      await expect(testPage.getByText("Agent set by workflow")).not.toBeVisible({ timeout: 5_000 });
      await expect(selectorButton).toBeEnabled();

      // Clean up the second workflow
      await apiClient.deleteWorkflow(noProfileWorkflow.id);
    } finally {
      // Always restore the workflow to no agent profile
      await apiClient.updateWorkflow(seedData.workflowId, {
        agent_profile_id: "",
      });
    }
  });
});
