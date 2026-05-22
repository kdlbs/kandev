import { test, expect } from "../fixtures/test-base";
import { AutomationsPage } from "../pages/automations-page";

test.describe("Automations settings page", () => {
  test("list page shows empty state", async ({ testPage, seedData }) => {
    const automations = new AutomationsPage(testPage, seedData.workspaceId);
    await automations.goto();

    await expect(automations.emptyState).toBeVisible({ timeout: 10_000 });
    await expect(automations.emptyState).toHaveText(/No automations yet/);
  });

  test("create scheduled automation via UI", async ({ testPage, seedData }) => {
    const automations = new AutomationsPage(testPage, seedData.workspaceId);
    await automations.goto();

    // Navigate to new automation form
    await automations.newAutomationButton.click();
    await expect(testPage).toHaveURL(/automations\/new/, { timeout: 10_000 });
    await expect(automations.editor).toBeVisible();

    // Fill in name
    await automations.nameInput.fill("Daily Check");

    // Select a schedule preset
    await automations.schedulePreset("@daily").click();

    // Select workflow and step
    await automations.selectWorkflow("E2E Workflow");
    await automations.selectWorkflowStep(seedData.steps[0].name);

    // Save — button should be enabled now
    await expect(automations.saveButton).toBeEnabled({ timeout: 5_000 });
    await automations.saveButton.click();

    // Should redirect to the edit page (URL contains automation ID)
    await expect(testPage).toHaveURL(/automations\/[a-f0-9-]+$/, { timeout: 15_000 });

    // Name should persist in the input
    await expect(automations.nameInput).toHaveValue("Daily Check");
  });

  test("create automation with custom schedule expression", async ({ testPage, seedData }) => {
    const automations = new AutomationsPage(testPage, seedData.workspaceId);
    await automations.gotoNew();

    await automations.nameInput.fill("Custom Schedule");
    await automations.customScheduleInput.fill("@every 2h");
    await automations.customScheduleInput.blur();

    // Select workflow and step
    await automations.selectWorkflow("E2E Workflow");
    await automations.selectWorkflowStep(seedData.steps[0].name);

    await expect(automations.saveButton).toBeEnabled({ timeout: 5_000 });
    await automations.saveButton.click();

    await expect(testPage).toHaveURL(/automations\/[a-f0-9-]+$/, { timeout: 15_000 });
  });

  test("schedule validation rejects invalid expression", async ({ testPage, seedData }) => {
    const automations = new AutomationsPage(testPage, seedData.workspaceId);
    await automations.gotoNew();

    await automations.customScheduleInput.fill("invalid-cron");
    await automations.customScheduleInput.blur();

    // Should show error text
    await expect(testPage.getByText("Invalid expression")).toBeVisible({ timeout: 5_000 });
  });

  test("edit automation name", async ({ testPage, seedData }) => {
    const automations = new AutomationsPage(testPage, seedData.workspaceId);

    // Create an automation first
    await automations.gotoNew();
    await automations.nameInput.fill("Original Name");
    await automations.schedulePreset("@hourly").click();
    await automations.selectWorkflow("E2E Workflow");
    await automations.selectWorkflowStep(seedData.steps[0].name);
    await expect(automations.saveButton).toBeEnabled({ timeout: 5_000 });
    await automations.saveButton.click();
    await expect(testPage).toHaveURL(/automations\/[a-f0-9-]+$/, { timeout: 15_000 });

    // Edit the name
    await automations.nameInput.clear();
    await automations.nameInput.fill("Updated Name");
    await automations.saveButton.click();

    // Go back to list and verify
    await automations.goto();
    await expect(automations.table).toBeVisible({ timeout: 10_000 });
    await expect(testPage.getByText("Updated Name")).toBeVisible();
  });

  test("delete automation from editor", async ({ testPage, seedData }) => {
    const automations = new AutomationsPage(testPage, seedData.workspaceId);

    // Create an automation first
    await automations.gotoNew();
    await automations.nameInput.fill("To Be Deleted");
    await automations.schedulePreset("@weekly").click();
    await automations.selectWorkflow("E2E Workflow");
    await automations.selectWorkflowStep(seedData.steps[0].name);
    await expect(automations.saveButton).toBeEnabled({ timeout: 5_000 });
    await automations.saveButton.click();
    await expect(testPage).toHaveURL(/automations\/[a-f0-9-]+$/, { timeout: 15_000 });

    // Delete it
    await automations.deleteButton.click();

    // Should redirect to list page
    await expect(testPage).toHaveURL(/automations$/, { timeout: 10_000 });

    // The deleted automation should not appear in the list
    await expect(testPage.getByText("To Be Deleted")).not.toBeVisible({ timeout: 10_000 });
  });

  test("enable/disable toggle on list page", async ({ testPage, seedData }) => {
    const automations = new AutomationsPage(testPage, seedData.workspaceId);

    // Create an automation
    await automations.gotoNew();
    await automations.nameInput.fill("Toggle Test");
    await automations.schedulePreset("@daily").click();
    await automations.selectWorkflow("E2E Workflow");
    await automations.selectWorkflowStep(seedData.steps[0].name);
    await expect(automations.saveButton).toBeEnabled({ timeout: 5_000 });
    await automations.saveButton.click();
    await expect(testPage).toHaveURL(/automations\/[a-f0-9-]+$/, { timeout: 15_000 });

    // Go back to list
    await automations.goto();
    await expect(automations.table).toBeVisible({ timeout: 10_000 });

    // Find the toggle — automations are enabled by default.
    // The table row containing "Toggle Test" has a switch inside it.
    const row = testPage.locator("tr", { hasText: "Toggle Test" });
    const toggle = row.locator('[role="switch"]');
    await expect(toggle).toBeChecked();

    // Disable it
    await toggle.click();
    await expect(toggle).not.toBeChecked();

    // Reload and verify it persisted
    await testPage.reload();
    await expect(automations.table).toBeVisible({ timeout: 10_000 });
    const rowAfterReload = testPage.locator("tr", { hasText: "Toggle Test" });
    const toggleAfterReload = rowAfterReload.locator('[role="switch"]');
    await expect(toggleAfterReload).not.toBeChecked();
  });
});
