import { test, expect } from "../fixtures/test-base";
import { AutomationsPage } from "../pages/automations-page";

/**
 * Mobile-viewport coverage for T01's three webhook config-only fields
 * (dedup_key, filters, repository.selector_path). Mirrors
 * mobile-automations-pr-merged-trigger.spec.ts's touch-interaction and
 * overflow-assertion pattern; the desktop suite
 * (automations-webhook-alert-ingest.spec.ts) already covers the same fields'
 * backend-admission behavior end to end.
 */
test.describe("Webhook alert ingest trigger on mobile", () => {
  test("edits the three admission fields with touch and persists the configuration", async ({
    testPage,
    seedData,
  }) => {
    const automationName = "Mobile webhook trigger";
    const automations = new AutomationsPage(testPage, seedData.workspaceId);

    await automations.gotoNew();
    await automations.nameInput.fill(automationName);
    await testPage.getByText("Or use a webhook instead").tap();
    await automations.selectWorkflow("E2E Workflow");

    await testPage.locator("button", { hasText: "Webhook" }).tap();
    const triggerCard = testPage.getByTestId("trigger-card-webhook");
    await expect(triggerCard).toBeVisible();

    // Dedup key.
    await triggerCard.getByPlaceholder("issue.id").fill("issue.id");

    // One filter: path/op/values.
    await triggerCard.getByRole("button", { name: "Add filter" }).tap();
    await triggerCard.getByPlaceholder("severity").fill("severity");
    await triggerCard.getByRole("combobox").tap();
    await testPage.getByRole("option", { name: "In list", exact: true }).tap();
    const valuesInput = triggerCard.getByPlaceholder("critical, fatal");
    await valuesInput.fill("critical, fatal");
    await valuesInput.blur();

    // Repository selector.
    await triggerCard.getByPlaceholder("service").fill("service");

    await expect(automations.saveButton).toBeEnabled({ timeout: 5_000 });

    const viewport = testPage.viewportSize();
    const selectorBox = await triggerCard.getByPlaceholder("service").boundingBox();
    expect(viewport).not.toBeNull();
    expect(selectorBox).not.toBeNull();
    expect(selectorBox!.x).toBeGreaterThanOrEqual(0);
    expect(selectorBox!.x + selectorBox!.width).toBeLessThanOrEqual(viewport!.width);

    // New webhook automations show the reveal dialog, not a redirect.
    await automations.saveButton.tap();
    await expect(testPage.getByTestId("webhook-created-dialog")).toBeVisible({ timeout: 10_000 });
    await testPage.getByTestId("webhook-created-dialog-close").tap();
    await expect(testPage).toHaveURL(/automations$/, { timeout: 15_000 });

    // Reopen and verify every field persisted.
    await expect(automations.table).toBeVisible({ timeout: 10_000 });
    await automations.table.getByText(automationName, { exact: true }).tap();
    await expect(testPage).toHaveURL(/automations\/[a-f0-9-]+$/, { timeout: 10_000 });
    await expect(automations.editor).toBeVisible();
    await testPage.locator("button", { hasText: "Webhook" }).tap();
    const reopened = testPage.getByTestId("trigger-card-webhook");

    await expect(reopened.getByPlaceholder("issue.id")).toHaveValue("issue.id");
    await expect(reopened.getByPlaceholder("severity")).toHaveValue("severity");
    await expect(reopened.getByRole("combobox")).toContainText("In list");
    await expect(reopened.getByPlaceholder("critical, fatal")).toHaveValue("critical, fatal");
    await expect(reopened.getByPlaceholder("service")).toHaveValue("service");

    const overflow = await testPage.evaluate(() => ({
      scrollWidth: document.documentElement.scrollWidth,
      clientWidth: document.documentElement.clientWidth,
    }));
    expect(overflow.scrollWidth).toBeLessThanOrEqual(overflow.clientWidth);

    await automations.deleteButton.tap();
    await expect(automations.deleteConfirmation).toBeVisible();
    await automations.deleteConfirmButton.tap();
    await expect(testPage).toHaveURL(/automations$/, { timeout: 10_000 });
    await expect(testPage.getByText(automationName, { exact: true })).not.toBeVisible();
  });
});
