import { expect, test } from "../../fixtures/test-base";
import type { SeedData } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";
import { openTaskSession } from "../../helpers/session";
import { routeSessionEntryRecovery } from "../../helpers/session-entry-recovery";

async function createEntryTask(apiClient: ApiClient, seedData: SeedData, title: string) {
  return apiClient.createTaskWithAgent(seedData.workspaceId, title, seedData.agentProfileId, {
    description: "/e2e:simple-message",
    workflow_id: seedData.workflowId,
    workflow_step_id: seedData.startStepId,
    repository_ids: [seedData.repositoryId],
  });
}

test.describe("session entry recovery", () => {
  test.describe.configure({ retries: 1 });

  test("completes entry when status and subscription acknowledgements take seven seconds", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(90_000);
    const proxy = await routeSessionEntryRecovery(testPage);
    const task = await createEntryTask(apiClient, seedData, `Delayed entry ${Date.now()}`);

    proxy.delayNextResponses(
      "task.session.status",
      1,
      7_000,
      "expose a delayed status acknowledgement without crossing the entry deadline",
    );
    proxy.delayNextResponses(
      "session.subscribe",
      1,
      7_000,
      "expose a delayed subscription acknowledgement without a reload",
    );

    const session = await openTaskSession(testPage, task.id);
    await expect(session.activeChat()).toContainText("simple mock response", { timeout: 45_000 });
    await expect(testPage.getByTestId("ensure-session-error-banner")).toHaveCount(0);
    expect(proxy.delayedResponseCount("task.session.status")).toBe(1);
    expect(proxy.delayedResponseCount("session.subscribe")).toBe(1);
  });

  test("retries a timed out subscription automatically while the page remains open", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);
    const proxy = await routeSessionEntryRecovery(testPage);
    const task = await createEntryTask(apiClient, seedData, `Retry entry ${Date.now()}`);

    proxy.delayNextResponses(
      "session.subscribe",
      1,
      11_000,
      "force the first subscription acknowledgement past the ten-second deadline",
    );

    const session = await openTaskSession(testPage, task.id);
    await expect
      .poll(() => proxy.requestCount("session.subscribe"), {
        timeout: 30_000,
        message: "Waiting for the automatic second session.subscribe request",
      })
      .toBeGreaterThan(1);
    await expect(session.activeChat()).toContainText("simple mock response", { timeout: 60_000 });
    await expect(testPage.getByTestId("ensure-session-error-banner")).toHaveCount(0);
    expect(proxy.delayedResponseCount("session.subscribe")).toBe(1);
  });

  test("shows one recoverable history notice and restores it with Retry", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(150_000);
    const proxy = await routeSessionEntryRecovery(testPage);
    const task = await createEntryTask(apiClient, seedData, `History recovery ${Date.now()}`);

    proxy.delayNextResponses(
      "message.list",
      2,
      11_000,
      "force both bounded history attempts to expose the unavailable state",
    );

    const session = await openTaskSession(testPage, task.id);
    const historyNotice = session.activeChat().getByTestId("session-history-unavailable");
    await expect(historyNotice).toBeVisible({ timeout: 45_000 });
    await expect(
      session.activeChat().getByText("No messages yet. Start the conversation!", { exact: true }),
    ).toHaveCount(0);

    const details = session.activeChat().getByTestId("session-history-details");
    await expect(details).toBeVisible();
    await expect(details).not.toHaveAttribute("open", "");
    await details.getByTestId("session-history-details-summary").click();
    await expect(details).toHaveAttribute("open", "");
    await expect(details).toContainText("WebSocket request timed out: message.list");

    await historyNotice.getByTestId("session-history-retry").click();
    await expect(historyNotice).toHaveCount(0);
    await expect(session.activeChat()).toContainText("simple mock response", { timeout: 30_000 });
    expect(proxy.delayedResponseCount("message.list")).toBe(2);
  });

  test("labels exhausted status checks accurately and retries only the status read", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(150_000);
    const proxy = await routeSessionEntryRecovery(testPage);
    const task = await createEntryTask(apiClient, seedData, `Status recovery ${Date.now()}`);

    proxy.delayNextResponses(
      "task.session.status",
      2,
      11_000,
      "force both bounded status attempts to expose the compact status notice",
    );

    await openTaskSession(testPage, task.id);
    const statusNotice = testPage.getByTestId("session-status-unavailable");
    await expect(statusNotice).toBeVisible({ timeout: 45_000 });
    await expect(statusNotice).not.toContainText("Couldn't start a session");

    const details = statusNotice.getByTestId("session-status-details");
    await expect(details).not.toHaveAttribute("open", "");
    await details.getByTestId("session-status-details-summary").click();
    await expect(details).toContainText("WebSocket request timed out: task.session.status");

    const launchRequestCountBeforeRetry = proxy.requestCount("session.launch");
    await statusNotice.getByTestId("session-status-retry").click();
    await expect(statusNotice).toHaveCount(0);
    expect(proxy.requestCount("task.session.status")).toBeGreaterThanOrEqual(3);
    expect(proxy.requestCount("session.launch")).toBe(launchRequestCountBeforeRetry);
    expect(proxy.delayedResponseCount("task.session.status")).toBe(2);
  });
});
