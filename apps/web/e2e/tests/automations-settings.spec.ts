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

    // Pick a schedule
    await automations.selectFrequency("every day");

    // Select workflow and step
    await automations.selectWorkflow("E2E Workflow");

    // Save — button should be enabled now
    await expect(automations.saveButton).toBeEnabled({ timeout: 5_000 });
    await automations.saveButton.click();

    // Should land on the listings page with the new automation visible
    await expect(testPage).toHaveURL(/automations$/, { timeout: 15_000 });
    await expect(automations.table).toBeVisible({ timeout: 10_000 });
    await expect(testPage.getByText("Daily Check")).toBeVisible();
  });

  test("shows continuity choices and persists a reusable automation", async ({
    testPage,
    seedData,
  }) => {
    const automations = new AutomationsPage(testPage, seedData.workspaceId);
    await automations.gotoNew();

    await expect(testPage.getByText("Context between runs", { exact: true })).toBeVisible();
    await expect(
      testPage.getByText(
        "Choose whether each run starts fresh or continues the same conversation and files.",
        { exact: true },
      ),
    ).toBeVisible();
    await expect(
      testPage.getByText(
        "Each run starts with a separate conversation and files. These tasks do not appear in Kanban or the sidebar. Use this option for independent jobs and concurrent runs.",
        { exact: true },
      ),
    ).toBeVisible();
    await expect(
      testPage.getByText(
        "Runs continue the same conversation and files, so the agent keeps prior context and changes. Runs execute one at a time.",
        { exact: true },
      ),
    ).toBeVisible();

    const reuse = testPage.getByRole("radio", { name: "Continue the previous session" });
    await expect(reuse).not.toBeChecked();
    await reuse.check();
    await expect(reuse).toBeChecked();
    await expect(testPage.getByRole("spinbutton")).toHaveValue("1");
    await expect(testPage.getByRole("spinbutton")).toBeDisabled();
    await expect(
      testPage.getByText("This option supports one active run at a time."),
    ).toBeVisible();

    await testPage.getByTestId("automation-name-input").fill("Reusable Context");
    await automations.selectFrequency("every day");
    await automations.selectWorkflow("E2E Workflow");
    await expect(automations.saveButton).toBeEnabled({ timeout: 5_000 });
    await automations.saveButton.click();

    await expect(testPage).toHaveURL(/automations$/, { timeout: 15_000 });
    await automations.openByName("Reusable Context");
    await expect(testPage).toHaveURL(/automations\/[a-f0-9-]+$/, { timeout: 10_000 });
    await expect(
      testPage.getByRole("radio", { name: "Continue the previous session" }),
    ).toBeChecked();
    await expect(testPage.getByRole("spinbutton")).toHaveValue("1");
  });

  test("create automation with custom schedule expression", async ({ testPage, seedData }) => {
    const automations = new AutomationsPage(testPage, seedData.workspaceId);
    await automations.gotoNew();

    await automations.nameInput.fill("Custom Schedule");
    await automations.setCustomSchedule("@every 2h");

    // Select workflow and step
    await automations.selectWorkflow("E2E Workflow");

    await expect(automations.saveButton).toBeEnabled({ timeout: 5_000 });
    await automations.saveButton.click();

    await expect(testPage).toHaveURL(/automations$/, { timeout: 15_000 });
    await expect(testPage.getByText("Custom Schedule")).toBeVisible({ timeout: 10_000 });
  });

  test("schedule validation rejects invalid expression", async ({ testPage, seedData }) => {
    const automations = new AutomationsPage(testPage, seedData.workspaceId);
    await automations.gotoNew();

    await automations.setCustomSchedule("invalid-cron");

    // Should show error text
    await expect(testPage.getByText("not a schedule we can read")).toBeVisible({
      timeout: 5_000,
    });
  });

  test("edit automation name", async ({ testPage, seedData }) => {
    const automations = new AutomationsPage(testPage, seedData.workspaceId);

    // Create an automation first
    await automations.gotoNew();
    await automations.nameInput.fill("Original Name");
    await automations.selectFrequency("every hour");
    await automations.selectWorkflow("E2E Workflow");
    await expect(automations.saveButton).toBeEnabled({ timeout: 5_000 });
    await automations.saveButton.click();

    // After create we land on the listings page — open the new automation
    // by clicking its row. Wait for the table to render before locating
    // the row so the click doesn't race the listings hydration.
    await expect(testPage).toHaveURL(/automations$/, { timeout: 15_000 });
    await expect(automations.table).toBeVisible({ timeout: 10_000 });
    await automations.openByName("Original Name");
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

  test("delete automation from editor requires confirmation", async ({ testPage, seedData }) => {
    const automations = new AutomationsPage(testPage, seedData.workspaceId);

    // Create an automation first
    await automations.gotoNew();
    await automations.nameInput.fill("To Be Deleted");
    await automations.selectFrequency("every week");
    await automations.selectWorkflow("E2E Workflow");
    await expect(automations.saveButton).toBeEnabled({ timeout: 5_000 });
    await automations.saveButton.click();

    // Land on listings, click into the new row to reach the editor.
    await expect(testPage).toHaveURL(/automations$/, { timeout: 15_000 });
    await expect(automations.table).toBeVisible({ timeout: 10_000 });
    await automations.openByName("To Be Deleted");
    await expect(testPage).toHaveURL(/automations\/[a-f0-9-]+$/, { timeout: 10_000 });

    // The confirmation must identify the persisted automation, not an
    // unsaved draft name from the editor.
    await automations.nameInput.fill("Unsaved Draft Name");
    await automations.deleteButton.click();
    await expect(automations.deleteConfirmation).toBeVisible();
    await expect(automations.deleteConfirmation).toContainText(
      "This will permanently delete To Be Deleted. This action cannot be undone.",
    );
    await expect(automations.deleteConfirmation).not.toContainText("Unsaved Draft Name");

    // Cancelling must leave the editor and the automation untouched.
    await automations.deleteConfirmation.getByRole("button", { name: "Cancel" }).click();
    await expect(automations.deleteConfirmation).not.toBeVisible();
    await expect(testPage).toHaveURL(/automations\/[a-f0-9-]+$/, { timeout: 5_000 });
    await expect(automations.deleteButton).toBeVisible();

    // Confirming performs the existing delete flow.
    await automations.deleteButton.click();
    await expect(automations.deleteConfirmation).toBeVisible();
    await automations.deleteConfirmButton.click();

    // Should redirect to list page
    await expect(testPage).toHaveURL(/automations$/, { timeout: 10_000 });

    // The deleted automation should not appear in the list
    await expect(testPage.getByText("To Be Deleted")).not.toBeVisible({ timeout: 10_000 });
  });

  test("delete automation from list requires confirmation", async ({
    testPage,
    seedData,
    apiClient,
    prCapture,
  }) => {
    const automation = await apiClient.seedAutomation({
      workspaceId: seedData.workspaceId,
      name: "List Delete Automation",
      workflowId: seedData.workflowId,
      workflowStepId: seedData.startStepId,
    });
    const automations = new AutomationsPage(testPage, seedData.workspaceId);
    await automations.goto();

    const row = automations.automationRow(automation.id);
    const deleteButton = row.getByTestId(`automation-delete-button-${automation.id}`);
    await deleteButton.click();

    await expect(automations.deleteConfirmation).toBeVisible();
    await expect(automations.deleteConfirmation).toContainText(
      "This will permanently delete List Delete Automation. This action cannot be undone.",
    );
    await prCapture.screenshot("automation-delete-confirmation-desktop", {
      caption: "Desktop automation deletion confirmation",
    });
    await automations.deleteConfirmation.getByRole("button", { name: "Cancel" }).click();
    await expect(automations.deleteConfirmation).not.toBeVisible();
    await expect(row).toBeVisible();

    await deleteButton.click();
    await automations.deleteConfirmButton.click();
    await expect(row).toHaveCount(0, { timeout: 10_000 });
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
    await expect(automations.saveButton).toBeEnabled({ timeout: 5_000 });
    await automations.saveButton.click();

    // Close the dialog and wait for listings
    await expect(testPage.getByTestId("webhook-created-dialog")).toBeVisible({ timeout: 10_000 });
    await testPage.getByTestId("webhook-created-dialog-close").click();
    await expect(testPage).toHaveURL(/automations$/, { timeout: 15_000 });
    await expect(automations.table).toBeVisible({ timeout: 10_000 });

    // Click into the automation row to open the editor
    await automations.openByName("Reveal Me");
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

  test("repository starts empty with no workspace fallback", async ({ testPage, seedData }) => {
    const automations = new AutomationsPage(testPage, seedData.workspaceId);
    await automations.gotoNew();

    await expect(testPage.getByTestId("repo-chip")).toHaveCount(0);
    await expect(testPage.getByText(/run without repository files/i)).toBeVisible();
    await expect(testPage.getByText(/use workspace default/i)).toHaveCount(0);
  });

  test("shares workflow previews and repository branch chips with New Task", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const automations = new AutomationsPage(testPage, seedData.workspaceId);
    await automations.gotoNew();

    await automations.workflowSelector.click();
    await expect(testPage.getByText(seedData.steps[0].name, { exact: true })).toBeVisible();
    await testPage.keyboard.press("Escape");

    const { repositories } = await apiClient.listRepositories(seedData.workspaceId);
    const repository = repositories[0];
    expect(repository).toBeTruthy();
    await testPage.getByRole("button", { name: "Add repository" }).click();
    await testPage.getByTestId("repo-chip-trigger").click();
    await testPage.getByText(repository!.name, { exact: true }).last().click();

    await expect(testPage.getByTestId("repo-chip")).toHaveAttribute(
      "data-repository-id",
      repository!.id,
    );
    await expect(testPage.getByTestId("branch-chip-trigger")).toBeEnabled();
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
    await automations.selectFrequency("every day");
    await automations.selectWorkflow("E2E Workflow");
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
    await testPage.goto(
      `/settings/workspaces/${seedData.workspaceId}/automations/${automation.id}`,
    );
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

  test("delete all only removes the runs in the active status view", async ({
    testPage,
    seedData,
    apiClient,
  }) => {
    // Seed mixed-status runs: the delete-all control must be scoped to the
    // active filter, so clearing the Skipped view leaves the succeeded run
    // alone instead of wiping the whole automation.
    const automation = await apiClient.seedAutomation({
      workspaceId: seedData.workspaceId,
      name: "Filtered Run Delete Test",
      workflowId: seedData.workflowId,
      workflowStepId: seedData.startStepId,
    });
    await apiClient.seedAutomationRun(automation.id, "skipped");
    await apiClient.seedAutomationRun(automation.id, "skipped");
    await apiClient.seedAutomationRun(automation.id, "succeeded");

    await testPage.goto(
      `/settings/workspaces/${seedData.workspaceId}/automations/${automation.id}`,
    );
    await testPage.getByTestId("automation-editor").waitFor({ state: "visible", timeout: 15_000 });

    const scrollContainer = testPage.getByTestId("settings-scroll-container");
    await scrollContainer.evaluate((el) => (el.scrollTop = el.scrollHeight));

    const recentRunsButton = testPage.locator("button", { hasText: /Recent Runs/ });
    await recentRunsButton.waitFor({ state: "visible", timeout: 10_000 });
    await recentRunsButton.click();

    const tbody = testPage.locator("table tbody");
    await tbody.waitFor({ state: "visible", timeout: 5_000 });
    await expect(tbody.locator("tr")).toHaveCount(3, { timeout: 10_000 });

    // Filter to Skipped: only the two skipped rows remain visible.
    await testPage.getByTestId("run-filter-skipped").click();
    await expect(tbody.locator("tr")).toHaveCount(2, { timeout: 5_000 });

    // The delete-all control lives in the table header, aligned with the
    // per-row delete buttons, not beside the Recent Runs heading.
    const thead = testPage.locator("table thead");
    await expect(thead.getByTestId("delete-all-runs")).toBeVisible();

    // The confirmation names the scoped status rather than promising every
    // run record for the automation.
    await thead.getByTestId("delete-all-runs").click();
    await expect(
      testPage.getByText(/permanently remove the Skipped runs shown in this view/),
    ).toBeVisible();
    await testPage.getByTestId("delete-all-runs-confirm").click();

    // Only the skipped runs are gone; the succeeded run survives.
    await expect(tbody.locator("tr")).toHaveCount(1, { timeout: 5_000 });

    // Switching back to All still shows the succeeded run.
    await testPage.getByTestId("run-filter-all").click();
    await expect(tbody.locator("tr")).toHaveCount(1, { timeout: 5_000 });
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

    await testPage.goto(
      `/settings/workspaces/${seedData.workspaceId}/automations/${automation.id}`,
    );
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
    const archivedRow = testPage.locator(`table tbody tr[data-task-id="${archivedTask.id}"]`);
    await expect(archivedRow.getByText("Archived", { exact: true })).toBeVisible();

    // A task whose current (primary) session was genuinely cancelled —
    // the signal a real Stop leaves behind — reads "Cancelled", distinct
    // from "Archived".
    const cancelledRow = testPage.locator(`table tbody tr[data-task-id="${cancelledTask.id}"]`);
    await expect(cancelledRow.getByText("Cancelled", { exact: true })).toBeVisible();

    // The still-open task's run is unaffected.
    const openRow = testPage.locator(`table tbody tr[data-task-id="${openTask.id}"]`);
    await expect(openRow.getByText("Running", { exact: true })).toBeVisible();
  });
});

/**
 * The one-time notice that automation runs stopped landing on the kanban.
 *
 * Withdrawing execution modes made every firing land in one hidden place, but
 * `task` was the stored default — so on upgrade a working setup simply stops
 * producing the cards someone built their day around. The server derives
 * `legacy_board_card` from the retained `execution_mode` column, and the API
 * ignores that field on input by design, so these tests backdate the column
 * through the E2E seeding endpoint (`seedAutomation({ legacyBoardCard: true })`)
 * — the same on-disk state a real upgraded install carries. The flag the UI
 * reads is then produced by the production SQL, not by a stub.
 */
test.describe("Automations board-move migration notice", () => {
  test("tells a workspace whose automations used to put cards on the board", async ({
    testPage,
    seedData,
    apiClient,
  }) => {
    await apiClient.seedAutomation({
      workspaceId: seedData.workspaceId,
      name: "Legacy Board Automation",
      workflowId: seedData.workflowId,
      workflowStepId: seedData.startStepId,
      legacyBoardCard: true,
    });

    const automations = new AutomationsPage(testPage, seedData.workspaceId);
    await automations.goto();

    const notice = testPage.getByTestId("automation-board-move-notice");
    await expect(notice).toBeVisible({ timeout: 15_000 });
    await expect(notice).toContainText("no longer appear on the board");
    // Above the table, because it explains why the table's automations stopped
    // showing up where the reader last saw them.
    await expect(automations.table).toBeVisible();
    const [noticeTop, tableTop] = await Promise.all([
      notice.evaluate((el) => el.getBoundingClientRect().top),
      automations.table.evaluate((el) => el.getBoundingClientRect().top),
    ]);
    expect(noticeTop).toBeLessThan(tableTop);
  });

  test("stays gone once dismissed, across a reload", async ({ testPage, seedData, apiClient }) => {
    await apiClient.seedAutomation({
      workspaceId: seedData.workspaceId,
      name: "Dismissible Legacy Automation",
      workflowId: seedData.workflowId,
      workflowStepId: seedData.startStepId,
      legacyBoardCard: true,
    });

    const automations = new AutomationsPage(testPage, seedData.workspaceId);
    await automations.goto();

    const notice = testPage.getByTestId("automation-board-move-notice");
    await expect(notice).toBeVisible({ timeout: 15_000 });
    await testPage.getByTestId("automation-board-move-notice-dismiss").click();
    await expect(notice).toHaveCount(0);

    // The dismissal is durable — a migration notice that came back on every
    // visit would be worse than not shipping it.
    await testPage.reload();
    await expect(automations.table).toBeVisible({ timeout: 15_000 });
    await expect(notice).toHaveCount(0);
  });

  test("says nothing to a workspace that never ran in the withdrawn mode", async ({
    testPage,
    seedData,
    apiClient,
  }) => {
    // Created the way everything is created now: nothing was lost here, so
    // there is no news to break.
    await apiClient.seedAutomation({
      workspaceId: seedData.workspaceId,
      name: "Run Mode Automation",
      workflowId: seedData.workflowId,
      workflowStepId: seedData.startStepId,
    });

    const automations = new AutomationsPage(testPage, seedData.workspaceId);
    await automations.goto();

    // Assert against a rendered table, not an empty page — the notice sits
    // beside the automations it is about, and an empty list would hide it for
    // the wrong reason.
    await expect(automations.table).toBeVisible({ timeout: 15_000 });
    await expect(testPage.getByText("Run Mode Automation")).toBeVisible();
    await expect(testPage.getByTestId("automation-board-move-notice")).toHaveCount(0);
  });
});
