import { test, expect, type Page } from "../fixtures/test-base";
import { AutomationsPage } from "../pages/automations-page";

/**
 * E2E coverage for T01: the webhook trigger's three configuration-only alert
 * ingest fields — dedup_key, filters, repository.selector_path.
 *
 * Spec: docs/plans/alert-ingest/task-01-webhook-alert-ingest-path.md
 *
 * The UI round-trip test drives the three new fields entirely through the
 * automation editor. The three "fires for real" tests below it seed the
 * config via the API (mirroring existing trigger-type precedent) and then
 * POST directly at the production webhook route with the revealed secret —
 * the same path an external vendor uses — so the assertions are against the
 * real backend admission/dedup/filter/repository-binding logic and its
 * rendering in the runs list, not a mock.
 */

async function revealWebhookSecret(
  testPage: Page,
  triggerCard = testPage.getByTestId("trigger-card-webhook"),
) {
  const secretInput = triggerCard.getByTestId("automation-webhook-secret-input");
  await expect(secretInput).toHaveValue(/^•+$/, { timeout: 10_000 });
  await triggerCard.getByTestId("automation-webhook-secret-toggle").click();
  await expect(secretInput).not.toHaveValue(/^•+$/);
  const secret = await secretInput.inputValue();
  expect(secret).toBeTruthy();
  return secret;
}

async function postWebhook(testPage: Page, webhookUrl: string, secret: string, body: unknown) {
  const res = await testPage.request.post(webhookUrl, {
    headers: { "X-Webhook-Secret": secret },
    data: body,
  });
  expect(res.status()).toBe(200);
  const json = (await res.json()) as { status: string };
  expect(json.status).toBe("triggered");
}

/** Poll the runs list by clicking Refresh until `predicate` is satisfied. */
async function waitForRuns(testPage: Page, predicate: () => Promise<boolean>) {
  await expect
    .poll(
      async () => {
        await testPage.getByRole("button", { name: "Refresh" }).click();
        return predicate();
      },
      { timeout: 20_000, intervals: [500, 1000, 1000, 2000] },
    )
    .toBe(true);
}

async function openWebhookAutomation(testPage: Page, workspaceId: string, automationId: string) {
  await testPage.goto(`/settings/workspace/${workspaceId}/automations/${automationId}`);
  await testPage.getByTestId("automation-editor").waitFor({ state: "visible", timeout: 15_000 });
  await testPage.locator("button", { hasText: "Webhook" }).click();
  const triggerCard = testPage.getByTestId("trigger-card-webhook");
  await expect(triggerCard).toBeVisible();
  return triggerCard;
}

/** The runs table renders only once the "Recent Runs" disclosure is expanded. */
async function expandRunsSection(testPage: Page) {
  await testPage.getByText(/Recent Runs?\s*\(/).click();
  await expect(testPage.getByRole("button", { name: "Refresh" })).toBeVisible({ timeout: 5_000 });
}

test.describe("automations — webhook alert ingest (T01)", () => {
  test("dedup key, filters and repository selector round-trip through save/reload", async ({
    testPage,
    seedData,
  }) => {
    const automations = new AutomationsPage(testPage, seedData.workspaceId);
    await automations.gotoNew();

    await automations.nameInput.fill("Webhook Admission Round-trip");
    await testPage.getByText("Or use a webhook instead").click();
    await automations.selectWorkflow("E2E Workflow");

    // Expand the webhook trigger card.
    await testPage.locator("button", { hasText: "Webhook" }).click();
    const triggerCard = testPage.getByTestId("trigger-card-webhook");

    // Dedup key.
    await triggerCard.getByPlaceholder("issue.id").fill("issue.id");

    // One filter: path/op/values.
    await triggerCard.getByRole("button", { name: "Add filter" }).click();
    await triggerCard.getByPlaceholder("severity").fill("severity");
    await triggerCard.getByRole("combobox").click();
    await testPage.getByRole("option", { name: "In list", exact: true }).click();
    const valuesInput = triggerCard.getByPlaceholder("critical, fatal");
    await valuesInput.fill("critical, fatal");
    await valuesInput.blur();

    // Repository selector.
    await triggerCard.getByPlaceholder("service").fill("service");

    // Save — new webhook automations show the reveal dialog, not a redirect.
    await expect(automations.saveButton).toBeEnabled({ timeout: 5_000 });
    await automations.saveButton.click();
    await expect(testPage.getByTestId("webhook-created-dialog")).toBeVisible({ timeout: 10_000 });
    await testPage.getByTestId("webhook-created-dialog-close").click();
    await expect(testPage).toHaveURL(/automations$/, { timeout: 15_000 });

    // Reopen and verify every field persisted.
    await expect(automations.table).toBeVisible({ timeout: 10_000 });
    await automations.openByName("Webhook Admission Round-trip");
    await expect(testPage).toHaveURL(/automations\/[a-f0-9-]+$/, { timeout: 10_000 });
    await expect(automations.editor).toBeVisible();
    await testPage.locator("button", { hasText: "Webhook" }).click();
    const reopened = testPage.getByTestId("trigger-card-webhook");

    await expect(reopened.getByPlaceholder("issue.id")).toHaveValue("issue.id");
    await expect(reopened.getByPlaceholder("severity")).toHaveValue("severity");
    await expect(reopened.getByRole("combobox")).toContainText("In list");
    await expect(reopened.getByPlaceholder("critical, fatal")).toHaveValue("critical, fatal");
    await expect(reopened.getByPlaceholder("service")).toHaveValue("service");
  });

  test("two POSTs resolving the same dedup key create one task; the duplicate is recorded skipped with its reason", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const automation = await apiClient.seedAutomation({
      workspaceId: seedData.workspaceId,
      name: "Dedup E2E",
      workflowId: seedData.workflowId,
      workflowStepId: seedData.startStepId,
    });
    await apiClient.seedTrigger({
      automationId: automation.id,
      type: "webhook",
      config: { dedup_key: "issue.id" },
      enabled: true,
    });

    const triggerCard = await openWebhookAutomation(testPage, seedData.workspaceId, automation.id);
    const secret = await revealWebhookSecret(testPage, triggerCard);
    await expandRunsSection(testPage);
    const webhookUrl = `${new URL(testPage.url()).origin}/api/v1/automations/webhook/${automation.id}`;

    const payload = { issue: { id: "e2e-dedup-key-001" } };
    await postWebhook(testPage, webhookUrl, secret, payload);
    await postWebhook(testPage, webhookUrl, secret, payload);

    await waitForRuns(testPage, async () => (await testPage.getByTestId(/^run-row-/).count()) >= 2);

    const rows = testPage.getByTestId(/^run-row-/);
    await expect(rows).toHaveCount(2);

    const duplicateRows = rows.filter({ hasText: "duplicate trigger: dedup key already fired" });
    await expect(duplicateRows).toHaveCount(1);
    await expect(duplicateRows.getByText("Skipped", { exact: true })).toBeVisible();

    const admittedRows = rows.filter({ hasNotText: "duplicate trigger: dedup key already fired" });
    await expect(admittedRows).toHaveCount(1);
    await expect(admittedRows.getByText("Skipped", { exact: true })).toHaveCount(0);
  });

  test("a trigger with no dedup_key fires undeduplicated on repeated identical POSTs", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const automation = await apiClient.seedAutomation({
      workspaceId: seedData.workspaceId,
      name: "No Dedup Configured E2E",
      workflowId: seedData.workflowId,
      workflowStepId: seedData.startStepId,
    });
    await apiClient.seedTrigger({
      automationId: automation.id,
      type: "webhook",
      config: {},
      enabled: true,
    });

    const triggerCard = await openWebhookAutomation(testPage, seedData.workspaceId, automation.id);
    const secret = await revealWebhookSecret(testPage, triggerCard);
    await expandRunsSection(testPage);
    const webhookUrl = `${new URL(testPage.url()).origin}/api/v1/automations/webhook/${automation.id}`;

    const payload = { issue: { id: "same-value-every-time" } };
    await postWebhook(testPage, webhookUrl, secret, payload);
    await postWebhook(testPage, webhookUrl, secret, payload);

    await waitForRuns(testPage, async () => (await testPage.getByTestId(/^run-row-/).count()) >= 2);

    const rows = testPage.getByTestId(/^run-row-/);
    await expect(rows).toHaveCount(2);
    // Neither delivery is recorded as a duplicate — no dedup_key means today's
    // behavior (fire on every POST) is unchanged.
    await expect(rows.filter({ hasText: "duplicate trigger" })).toHaveCount(0);
  });

  test("a payload failing a filter creates no task and records the rejecting predicate", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const automation = await apiClient.seedAutomation({
      workspaceId: seedData.workspaceId,
      name: "Filter Rejection E2E",
      workflowId: seedData.workflowId,
      workflowStepId: seedData.startStepId,
    });
    await apiClient.seedTrigger({
      automationId: automation.id,
      type: "webhook",
      config: { filters: [{ path: "severity", op: "eq", values: ["fatal"] }] },
      enabled: true,
    });

    const triggerCard = await openWebhookAutomation(testPage, seedData.workspaceId, automation.id);
    const secret = await revealWebhookSecret(testPage, triggerCard);
    await expandRunsSection(testPage);
    const webhookUrl = `${new URL(testPage.url()).origin}/api/v1/automations/webhook/${automation.id}`;

    await postWebhook(testPage, webhookUrl, secret, { severity: "info" });

    await waitForRuns(testPage, async () => (await testPage.getByTestId(/^run-row-/).count()) >= 1);

    const rows = testPage.getByTestId(/^run-row-/);
    await expect(rows).toHaveCount(1);
    await expect(rows.getByText("Skipped", { exact: true })).toBeVisible();
    await expect(rows.getByTestId("run-outcome")).toContainText("filter_rejected: 0");
    // A filtered payload consumed no dedup key: no reason suffix is rendered.
    await expect(rows.getByTestId("run-outcome-reason")).toHaveCount(0);
  });

  test("a payload naming a repository outside the automation's configured repositories binds nothing (security: the webhook secret alone must not select an unconfigured repository)", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const { repositories } = await apiClient.listRepositories(seedData.workspaceId);
    const configuredRepo =
      repositories.find((r) => r.id === seedData.repositoryId) ?? repositories[0];
    expect(configuredRepo).toBeTruthy();

    const automation = await apiClient.seedAutomation({
      workspaceId: seedData.workspaceId,
      name: "Repository Selector Security E2E",
      workflowId: seedData.workflowId,
      workflowStepId: seedData.startStepId,
      repositoryMode: "selected",
      repositoryIds: [configuredRepo!.id],
      agentProfileId: seedData.agentProfileId,
      executorProfileId: seedData.worktreeExecutorProfileId,
    });
    await apiClient.seedTrigger({
      automationId: automation.id,
      type: "webhook",
      config: { repository: { selector_path: "service" } },
      enabled: true,
    });

    const triggerCard = await openWebhookAutomation(testPage, seedData.workspaceId, automation.id);
    const secret = await revealWebhookSecret(testPage, triggerCard);
    await expandRunsSection(testPage);
    const webhookUrl = `${new URL(testPage.url()).origin}/api/v1/automations/webhook/${automation.id}`;

    // The payload's "service" value looks exactly like what
    // resolveGitHubPRTriggerRepository would have consumed as owner/name —
    // the shape a vendor console emits — and matches no configured
    // repository's Name. It must never be resolved as a new repository.
    await postWebhook(testPage, webhookUrl, secret, { service: "acme/unconfigured-webapp" });

    // Row count flips as soon as the run is admitted, but repository_reason is
    // only written moments later by the async task-creation path (BindRunTask).
    // Poll for the reason itself, not just the row's existence.
    await waitForRuns(testPage, async () => {
      const rows = testPage.getByTestId(/^run-row-/);
      if ((await rows.count()) < 1) return false;
      return (await rows.first().getByTestId("run-outcome-reason").count()) > 0;
    });

    const noMatchRow = testPage.getByTestId(/^run-row-/).first();
    await expect(noMatchRow.getByTestId("run-outcome-reason")).toContainText(
      "Repository selector matched no configured repository: acme/unconfigured-webapp",
    );

    // The same selector path, when its resolved value matches a configured
    // repository's Name exactly, DOES bind — proven in the same test so the
    // comparator can't pass vacuously.
    await postWebhook(testPage, webhookUrl, secret, { service: configuredRepo!.name });
    await waitForRuns(testPage, async () => {
      const rows = testPage.getByTestId(/^run-row-/);
      if ((await rows.count()) < 2) return false;
      return (await rows.filter({ has: testPage.getByTestId("run-outcome-reason") }).count()) >= 1;
    });

    const rows = testPage.getByTestId(/^run-row-/);
    await expect(rows).toHaveCount(2);
    // Exactly one of the two rows carries the no-match reason; the other (the
    // successful bind) records nothing, because the binding is its own record.
    await expect(rows.filter({ has: testPage.getByTestId("run-outcome-reason") })).toHaveCount(1);
  });
});
