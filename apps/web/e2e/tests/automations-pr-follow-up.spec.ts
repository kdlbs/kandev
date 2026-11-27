import { test, expect } from "../fixtures/test-base";
import type { SeedData } from "../fixtures/test-base";
import type { Page } from "@playwright/test";
import type { ApiClient } from "../helpers/api-client";
import { AutomationsPage } from "../pages/automations-page";

/**
 * GitHub PR follow-up automation editor flows.
 * Spec: docs/specs/office/requirements/automations-pr-follow-up.md
 * Firing behavior is covered by backend integration tests; these flows prove
 * the editor saves and reloads every new setting.
 */

const REPO = { owner: "acme", name: "widget" };

async function seedPRAutomation(apiClient: ApiClient, seedData: SeedData, name: string) {
  const automation = await apiClient.seedAutomation({
    workspaceId: seedData.workspaceId,
    name,
    workflowId: seedData.workflowId,
    taskMode: "normal_task",
  });
  await apiClient.seedTrigger({
    automationId: automation.id,
    type: "github_pr",
    config: { events: ["opened"], repos: [REPO] },
    enabled: true,
  });
  return automation;
}

async function openEditor(page: Page, workspaceId: string, automationId: string) {
  await page.goto(`/settings/workspace/${workspaceId}/automations/${automationId}`);
  await page.getByTestId("automation-editor").waitFor({ state: "visible", timeout: 15_000 });
}

async function saveAndReload(page: Page, automations: AutomationsPage) {
  await expect(automations.saveButton).toBeEnabled({ timeout: 5_000 });
  await automations.saveButton.click();
  await automations.waitForSaved();
  await page.reload();
  await expect(automations.editor).toBeVisible({ timeout: 15_000 });
}

test.describe("automations — GitHub PR follow-up", () => {
  test("saves New commits, quiet period, Review requested, reviewers, and already-open PRs", async ({
    testPage,
    seedData,
    apiClient,
  }) => {
    const name = "PR follow-up events";
    const automation = await seedPRAutomation(apiClient, seedData, name);
    const automations = new AutomationsPage(testPage, seedData.workspaceId);
    await openEditor(testPage, seedData.workspaceId, automation.id);

    await testPage
      .getByRole("button", { name: /PR: Opened/ })
      .first()
      .click();
    const card = testPage.getByTestId("trigger-card-github_pr");
    await expect(card.getByLabel(/Wait until pushes stop for/)).toHaveCount(0);
    await card.getByRole("checkbox", { name: "New commits" }).click();
    const quiet = card.getByLabel(/Wait until pushes stop for/);
    await quiet.fill("10");
    await quiet.blur();
    await card.getByRole("checkbox", { name: "Review requested" }).click();
    const reviewers = card.getByLabel(/Reviewers/);
    await reviewers.fill("kandev-bot, acme/reviewers");
    await reviewers.blur();
    const includeExisting = card.getByRole("checkbox", {
      name: "Also run on pull requests already open",
    });
    await expect(includeExisting).not.toBeChecked();
    await includeExisting.click();

    await saveAndReload(testPage, automations);

    await testPage
      .getByRole("button", { name: /PR: Opened, New commits, Review requested/ })
      .first()
      .click();
    const reopened = testPage.getByTestId("trigger-card-github_pr");
    await expect(reopened.getByRole("checkbox", { name: "New commits" })).toBeChecked();
    await expect(reopened.getByRole("checkbox", { name: "Review requested" })).toBeChecked();
    await expect(reopened.getByLabel(/Wait until pushes stop for/)).toHaveValue("10");
    await expect(reopened.getByLabel(/Reviewers/)).toHaveValue("kandev-bot, acme/reviewers");
    await expect(
      reopened.getByRole("checkbox", { name: "Also run on pull requests already open" }),
    ).toBeChecked();
  });

  test("saves per-PR continuation, continuation step, and end-of-PR actions", async ({
    testPage,
    seedData,
    apiClient,
  }) => {
    const name = "PR follow-up continuation";
    const automation = await seedPRAutomation(apiClient, seedData, name);
    const automations = new AutomationsPage(testPage, seedData.workspaceId);
    const reviewStep = seedData.steps[1];
    await openEditor(testPage, seedData.workspaceId, automation.id);

    await expect(testPage.getByTestId("automation-follow-up-prompt-editor")).toHaveCount(0);
    await testPage.getByRole("radio", { name: "One task per pull request" }).click();
    const followUp = testPage.getByTestId("automation-follow-up-prompt-editor");
    await expect(followUp).toBeVisible();
    await expect(testPage.getByText("Follow-up instructions", { exact: true })).toBeVisible();
    // @covers AC-OFFICE-AUTOMATIONS-PR-FOLLOW-UP-006.4, AC-OFFICE-AUTOMATIONS-SETTINGS-002.4
    // The choice sits above the field it reveals, and the follow-up sits
    // directly under the first-run instructions.
    const choiceBox = await testPage
      .getByRole("radiogroup", { name: "Task for each run" })
      .boundingBox();
    const instructionsBox = await testPage.getByTestId("automation-prompt-editor").boundingBox();
    const followUpBox = await followUp.boundingBox();
    const placeholdersBox = await testPage
      .getByRole("button", { name: "Show placeholders" })
      .boundingBox();
    expect(choiceBox!.y).toBeLessThan(instructionsBox!.y);
    expect(instructionsBox!.y).toBeLessThan(followUpBox!.y);
    expect(followUpBox!.y).toBeLessThan(placeholdersBox!.y);

    const step = testPage.getByTestId("pr-continuation-step");
    await expect(step).toHaveText("Stay in current step");
    // @covers AC-OFFICE-AUTOMATIONS-SETTINGS-002.2
    // The workflow whose steps the picker lists is chosen above it.
    const workflowBox = await testPage.getByText("Workflow", { exact: true }).boundingBox();
    expect(workflowBox!.y).toBeLessThan((await step.boundingBox())!.y);
    await step.click();
    await testPage.getByRole("option", { name: reviewStep.name, exact: true }).click();

    await testPage.getByTestId("pr-merged-action").click();
    await testPage.getByRole("option", { name: "Archive task" }).click();
    await testPage.getByTestId("pr-closed-action").click();
    await testPage.getByRole("option", { name: "Delete task" }).click();
    await expect(
      testPage.getByText(/Deleting removes the task's conversation and files/),
    ).toBeVisible();

    await saveAndReload(testPage, automations);

    await expect(testPage.getByRole("radio", { name: "One task per pull request" })).toBeChecked();
    await expect(testPage.getByTestId("pr-continuation-step")).toHaveText(reviewStep.name);
    await expect(testPage.getByTestId("pr-merged-action")).toHaveText("Archive task");
    await expect(testPage.getByTestId("pr-closed-action")).toHaveText("Delete task");
  });
});
