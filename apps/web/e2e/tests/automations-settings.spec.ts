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
    await expect(automations.nameInput).toHaveAttribute("data-settings-dirty", "true");

    // Select a schedule preset
    await automations.schedulePreset("@daily").click();

    // Select workflow and step
    await automations.selectWorkflow("E2E Workflow");
    await automations.selectWorkflowStep(seedData.steps[0].name);

    // Save — button should be enabled now
    await expect(automations.saveButton).toBeEnabled({ timeout: 5_000 });
    await automations.saveButton.click();

    // Should land on the listings page with the new automation visible
    await expect(testPage).toHaveURL(/automations$/, { timeout: 15_000 });
    await expect(automations.table).toBeVisible({ timeout: 10_000 });
    await expect(testPage.getByText("Daily Check")).toBeVisible();
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

    await expect(testPage).toHaveURL(/automations$/, { timeout: 15_000 });
    await expect(testPage.getByText("Custom Schedule")).toBeVisible({ timeout: 10_000 });
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

    // After create we land on the listings page — open the new automation
    // by clicking its row. Wait for the table to render before locating
    // the row so the click doesn't race the listings hydration.
    await expect(testPage).toHaveURL(/automations$/, { timeout: 15_000 });
    await expect(automations.table).toBeVisible({ timeout: 10_000 });
    await automations.table.locator("tr", { hasText: "Original Name" }).click();
    await expect(testPage).toHaveURL(/automations\/[a-f0-9-]+$/, { timeout: 10_000 });
    await expect(automations.editor).toBeVisible();

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

    // Land on listings, click into the new row to reach the editor.
    await expect(testPage).toHaveURL(/automations$/, { timeout: 15_000 });
    await expect(automations.table).toBeVisible({ timeout: 10_000 });
    await automations.table.locator("tr", { hasText: "To Be Deleted" }).click();
    await expect(testPage).toHaveURL(/automations\/[a-f0-9-]+$/, { timeout: 10_000 });

    // Delete it
    await automations.deleteButton.click();

    // Should redirect to list page
    await expect(testPage).toHaveURL(/automations$/, { timeout: 10_000 });

    // The deleted automation should not appear in the list
    await expect(testPage.getByText("To Be Deleted")).not.toBeVisible({ timeout: 10_000 });
  });

  test("create webhook automation shows reveal dialog with URL and secret", async ({
    testPage,
    seedData,
  }) => {
    const automations = new AutomationsPage(testPage, seedData.workspaceId);
    await automations.gotoNew();

    // Fill in name
    await automations.nameInput.fill("My Webhook");

    // Switch to webhook mode
    await testPage.getByText("Or use a webhook instead").click();

    // Select workflow and step
    await automations.selectWorkflow("E2E Workflow");
    await automations.selectWorkflowStep(seedData.steps[0].name);

    // Save
    await expect(automations.saveButton).toBeEnabled({ timeout: 5_000 });
    await automations.saveButton.click();

    // Dialog should appear with URL and secret
    await expect(testPage.getByTestId("webhook-created-dialog")).toBeVisible({ timeout: 10_000 });

    // Verify the webhook URL is shown in the dialog
    await expect(testPage.locator('input[value*="/api/v1/automations/webhook/"]')).toBeVisible();

    // Verify a non-empty secret input is shown
    const secretInputs = testPage.locator("input[readonly]");
    const count = await secretInputs.count();
    let hasNonEmptySecret = false;
    for (let i = 0; i < count; i++) {
      const val = await secretInputs.nth(i).inputValue();
      if (val && !val.includes("/api/v1/automations/webhook/")) {
        hasNonEmptySecret = true;
        break;
      }
    }
    expect(hasNonEmptySecret).toBe(true);

    // Close the dialog
    await testPage.getByTestId("webhook-created-dialog-close").click();

    // Should redirect to listings and show the new automation
    await expect(testPage).toHaveURL(/automations$/, { timeout: 15_000 });
    await expect(automations.table).toBeVisible({ timeout: 10_000 });
    await expect(testPage.getByText("My Webhook")).toBeVisible();
  });

  test("webhook secret is masked by default on the edit page and revealable", async ({
    testPage,
    seedData,
  }) => {
    const automations = new AutomationsPage(testPage, seedData.workspaceId);
    await automations.gotoNew();

    // Create a webhook automation
    await automations.nameInput.fill("Reveal Me");
    await testPage.getByText("Or use a webhook instead").click();
    await automations.selectWorkflow("E2E Workflow");
    await automations.selectWorkflowStep(seedData.steps[0].name);
    await expect(automations.saveButton).toBeEnabled({ timeout: 5_000 });
    await automations.saveButton.click();

    // Close the dialog and wait for listings
    await expect(testPage.getByTestId("webhook-created-dialog")).toBeVisible({ timeout: 10_000 });
    await testPage.getByTestId("webhook-created-dialog-close").click();
    await expect(testPage).toHaveURL(/automations$/, { timeout: 15_000 });
    await expect(automations.table).toBeVisible({ timeout: 10_000 });

    // Click into the automation row to open the editor
    await automations.table.locator("tr", { hasText: "Reveal Me" }).click();
    await expect(testPage).toHaveURL(/automations\/[a-f0-9-]+$/, { timeout: 10_000 });
    await expect(automations.editor).toBeVisible();

    // Expand the webhook trigger card by clicking the summary button
    await testPage.locator("button", { hasText: "Webhook" }).click();

    // Secret should be masked by default
    const secretInput = testPage.getByTestId("automation-webhook-secret-input");
    await expect(secretInput).toBeVisible({ timeout: 10_000 });
    await expect(secretInput).toHaveValue(/^•+$/);

    // Click reveal toggle — secret should be unmasked
    await testPage.getByTestId("automation-webhook-secret-toggle").click();
    await expect(secretInput).not.toHaveValue(/^•+$/);
  });

  test("repository defaults to none on a fresh automation", async ({ testPage, seedData }) => {
    const automations = new AutomationsPage(testPage, seedData.workspaceId);
    await automations.gotoNew();

    // The repository selector should show "None" by default
    await expect(testPage.getByTestId("repository-selector")).toContainText(/None|no repository/i, {
      timeout: 10_000,
    });
  });

  test("create page keeps scrolling inside the settings pane", async ({ testPage, seedData }) => {
    const automations = new AutomationsPage(testPage, seedData.workspaceId);
    await automations.gotoNew();

    const rootScrollStyles = await testPage.evaluate(() => ({
      htmlOverflowY: getComputedStyle(document.documentElement).overflowY,
      htmlOverscrollY: getComputedStyle(document.documentElement).overscrollBehaviorY,
      bodyOverflowY: getComputedStyle(document.body).overflowY,
      bodyOverscrollY: getComputedStyle(document.body).overscrollBehaviorY,
    }));
    expect(rootScrollStyles).toEqual({
      htmlOverflowY: "hidden",
      htmlOverscrollY: "none",
      bodyOverflowY: "hidden",
      bodyOverscrollY: "none",
    });

    const settingsScroller = testPage.getByTestId("settings-scroll-container");
    await expect(settingsScroller).toBeVisible();
    await expect(settingsScroller).toHaveCSS("overflow-y", "auto");
    await expect(settingsScroller).toHaveCSS("overscroll-behavior-y", "contain");

    await testPage.mouse.wheel(0, 3000);
    await expect.poll(() => testPage.evaluate(() => window.scrollY), { timeout: 5_000 }).toBe(0);
  });

  test("enable/disable toggle on list page", async ({ testPage, seedData }) => {
    const automations = new AutomationsPage(testPage, seedData.workspaceId);

    // Create an automation — the new flow lands directly on the listings page.
    await automations.gotoNew();
    await automations.nameInput.fill("Toggle Test");
    await automations.schedulePreset("@daily").click();
    await automations.selectWorkflow("E2E Workflow");
    await automations.selectWorkflowStep(seedData.steps[0].name);
    await expect(automations.saveButton).toBeEnabled({ timeout: 5_000 });
    await automations.saveButton.click();
    await expect(testPage).toHaveURL(/automations$/, { timeout: 15_000 });
    await expect(automations.table).toBeVisible({ timeout: 10_000 });

    // Find the toggle — automations are enabled by default.
    // The table row containing "Toggle Test" has a switch inside it.
    const row = automations.table.locator("tr", { hasText: "Toggle Test" });
    const toggle = row.locator('[role="switch"]');
    await expect(toggle).toBeChecked();

    // Disable it
    await toggle.click();
    await expect(toggle).not.toBeChecked();
    await expect(toggle).toHaveAttribute("data-settings-dirty", "true");
    await expect(automations.table).toHaveAttribute("data-settings-dirty", "true");

    const floatingSave = testPage.getByTestId("settings-floating-save");
    await expect(floatingSave).toBeVisible();
    await floatingSave.getByRole("button", { name: "Save changes" }).click();
    await expect(floatingSave).not.toBeVisible({ timeout: 15_000 });

    // Reload only after the explicit settings save and verify it persisted.
    await testPage.reload();
    await expect(automations.table).toBeVisible({ timeout: 10_000 });
    const rowAfterReload = automations.table.locator("tr", { hasText: "Toggle Test" });
    const toggleAfterReload = rowAfterReload.locator('[role="switch"]');
    await expect(toggleAfterReload).not.toBeChecked();
  });

  test("delete individual and all runs from Recent Runs", async ({
    testPage,
    seedData,
    apiClient,
  }) => {
    // Seed an automation and two run rows via HTTP (avoids Node-24 WS requirement).
    const automation = await apiClient.seedAutomation({
      workspaceId: seedData.workspaceId,
      name: "Run Delete Test",
      workflowId: seedData.workflowId,
      workflowStepId: seedData.startStepId,
    });
    await apiClient.seedAutomationRun(automation.id, "skipped");
    await apiClient.seedAutomationRun(automation.id, "skipped");

    // Navigate to the editor page for this automation.
    await testPage.goto(`/settings/workspace/${seedData.workspaceId}/automations/${automation.id}`);
    await testPage.getByTestId("automation-editor").waitFor({ state: "visible", timeout: 15_000 });

    // Scroll to the bottom to ensure Recent Runs is visible.
    const scrollContainer = testPage.getByTestId("settings-scroll-container");
    await scrollContainer.evaluate((el) => (el.scrollTop = el.scrollHeight));

    // Expand the Recent Runs section.
    const recentRunsButton = testPage.locator("button", { hasText: /Recent Runs/ });
    await recentRunsButton.waitFor({ state: "visible", timeout: 10_000 });
    await recentRunsButton.click();

    // Wait for the table to appear with at least one row.
    const tbody = testPage.locator("table tbody");
    await tbody.waitFor({ state: "visible", timeout: 5_000 });
    await expect(tbody.locator("tr")).toHaveCount(2, { timeout: 10_000 });

    // Delete-all button should be visible in the header.
    const deleteAllBtn = testPage.getByTestId("delete-all-runs");
    await expect(deleteAllBtn).toBeVisible();

    // Before hover, the per-row delete button is transparent and
    // non-interactive — Playwright's toBeVisible() would pass even with
    // opacity:0, so assert the actual CSS values gating visibility/clicks.
    const firstRow = tbody.locator("tr").first();
    const deleteRowBtn = firstRow.getByTestId("delete-run");
    await expect(deleteRowBtn).toHaveCSS("opacity", "0");
    await expect(deleteRowBtn).toHaveCSS("pointer-events", "none");

    // Hover over the first row to reveal its delete button and click it.
    await firstRow.hover();
    await expect(deleteRowBtn).toHaveCSS("opacity", "1");
    await expect(deleteRowBtn).toHaveCSS("pointer-events", "auto");
    await deleteRowBtn.click();

    // One run removed — table should now have 1 row.
    await expect(tbody.locator("tr")).toHaveCount(1, { timeout: 5_000 });

    // Delete all remaining runs — click trigger, and confirm the dialog uses
    // unqualified wording rather than a count that could understate runs
    // beyond what's currently loaded in the table.
    await deleteAllBtn.click();
    await expect(testPage.getByText(/permanently remove all run records/)).toBeVisible();
    await testPage.getByTestId("delete-all-runs-confirm").click();

    // Table should show the empty state.
    await expect(testPage.getByText("No runs yet")).toBeVisible({ timeout: 5_000 });
  });

  test("archived task's run shows Archived, cancelled task's run shows Cancelled", async ({
    testPage,
    seedData,
    apiClient,
  }) => {
    // Regression test: an automation-generated task that gets archived
    // (manually, via auto-archive, via cascade, or by the agent itself)
    // before its run is otherwise finalized used to leave the run stuck at
    // "task_created" forever, displayed as "Running" and permanently
    // pinned against max_concurrent_runs. It also used to be
    // indistinguishable from a genuine user cancellation (the task's
    // primary session state going CANCELLED — the signal a real Stop
    // leaves behind; the task's own state is untouched by a stop), both
    // showing as "Cancelled". See internal/automation.Store.CountActiveRuns
    // / ListRuns.
    const automation = await apiClient.seedAutomation({
      workspaceId: seedData.workspaceId,
      name: "Archived Task Run Test",
      workflowId: seedData.workflowId,
      workflowStepId: seedData.startStepId,
    });

    const archivedTask = await apiClient.createTask(
      seedData.workspaceId,
      "Archived Automation Task",
      { workflow_id: seedData.workflowId, workflow_step_id: seedData.startStepId },
    );
    const cancelledTask = await apiClient.createTask(
      seedData.workspaceId,
      "Cancelled Automation Task",
      { workflow_id: seedData.workflowId, workflow_step_id: seedData.startStepId },
    );
    const openTask = await apiClient.createTask(seedData.workspaceId, "Open Automation Task", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    });
    await apiClient.seedAutomationRun(automation.id, "task_created", archivedTask.id);
    await apiClient.seedAutomationRun(automation.id, "task_created", cancelledTask.id);
    await apiClient.seedAutomationRun(automation.id, "task_created", openTask.id);
    await apiClient.archiveTask(archivedTask.id);
    await apiClient.seedAutomationTaskSession(cancelledTask.id, "CANCELLED");

    await testPage.goto(`/settings/workspace/${seedData.workspaceId}/automations/${automation.id}`);
    await testPage.getByTestId("automation-editor").waitFor({ state: "visible", timeout: 15_000 });

    const scrollContainer = testPage.getByTestId("settings-scroll-container");
    await scrollContainer.evaluate((el) => (el.scrollTop = el.scrollHeight));

    const recentRunsButton = testPage.locator("button", { hasText: /Recent Runs/ });
    await recentRunsButton.waitFor({ state: "visible", timeout: 10_000 });
    await recentRunsButton.click();

    const tbody = testPage.locator("table tbody");
    await tbody.waitFor({ state: "visible", timeout: 5_000 });
    await expect(tbody.locator("tr")).toHaveCount(3, { timeout: 10_000 });

    // The archived task's run is no longer outstanding work, and reads
    // "Archived" rather than being conflated with a real cancellation.
    const archivedRow = testPage.locator("table tbody tr", {
      hasText: archivedTask.id.slice(0, 8),
    });
    await expect(archivedRow.getByText("Archived", { exact: true })).toBeVisible();

    // A task whose current (primary) session was genuinely cancelled —
    // the signal a real Stop leaves behind — reads "Cancelled", distinct
    // from "Archived".
    const cancelledRow = testPage.locator("table tbody tr", {
      hasText: cancelledTask.id.slice(0, 8),
    });
    await expect(cancelledRow.getByText("Cancelled", { exact: true })).toBeVisible();

    // The still-open task's run is unaffected.
    const openRow = testPage.locator("table tbody tr", { hasText: openTask.id.slice(0, 8) });
    await expect(openRow.getByText("Running", { exact: true })).toBeVisible();
  });
});
