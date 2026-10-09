import { expect, type Page } from "@playwright/test";
import type { ApiClient } from "./api-client";
import type { PrAssetCapture } from "./pr-asset-capture";
import type { SeedData } from "../fixtures/test-base";
import { retainedCapacityFixture } from "./retained-capacity-fixture";

export async function verifyInterruptedContinuation(
  page: Page,
  api: ApiClient,
  taskId: string,
  sessionId: string,
  options: { capture: PrAssetCapture; viewport: string; seedData: SeedData; fixtureRoot: string },
) {
  const { capture, viewport } = options;
  const before = (await api.listTaskSessions(taskId)).sessions.find(
    (item) => item.id === sessionId,
  )!;
  await retainedCapacityFixture(options.fixtureRoot, sessionId, "fill");
  await expect(page.getByTestId("recovery-stop-button")).toBeEnabled();
  await page.getByTestId("recovery-retry-connection-button").click();
  const form = page.getByTestId("interrupted-session-continuation");
  await expect(form).toBeVisible({ timeout: 30_000 });
  await retainedCapacityFixture(options.fixtureRoot, sessionId, "assert_pruned");
  const instruction = "Inspect the saved changes and continue from them";
  await form.getByTestId("interrupted-recovery-instruction").fill(instruction);
  await form.getByTestId("interrupted-recovery-acknowledge").check();
  await capture.screenshot(`${viewport}-native-continuation`, {
    caption: "Explicit continuation of the saved conversation",
  });
  await form.getByTestId("interrupted-recovery-resume").click();
  await expect
    .poll(
      async () => {
        const current = (await api.listTaskSessions(taskId)).sessions.find(
          (item) => item.id === sessionId,
        );
        return (current?.metadata?.agent_delivery_recovery as { phase?: string })?.phase;
      },
      { timeout: 60_000 },
    )
    .toBe("continued");
  const after = (await api.listTaskSessions(taskId)).sessions.find(
    (item) => item.id === sessionId,
  )!;
  expect(after.id).toBe(before.id);
  expect(after.metadata?.acp).toEqual(before.metadata?.acp);
  expect(
    (after.metadata?.agent_delivery_recovery as { incarnation_id?: string })?.incarnation_id,
  ).toBe((before.metadata?.agent_delivery_recovery as { incarnation_id?: string })?.incarnation_id);
  expect(after.task_environment_id).toBe(before.task_environment_id);
  await page.getByTestId("interrupted-sessions-open").click();
  const result = page.getByTestId(`interrupted-result-${sessionId}`);
  await expect(result).toContainText("The new instruction was accepted", { timeout: 30_000 });
  await capture.screenshot(`${viewport}-batch-result`, {
    caption: "Persistent recovery results on the runtime recovery notice",
  });
  await page.reload();
  await page.getByTestId("interrupted-sessions-open").click();
  await expect(page.getByTestId(`interrupted-result-${sessionId}`)).toContainText(
    "The new instruction was accepted",
  );
  await expect(page.getByText(instruction, { exact: true })).toHaveCount(1);
  await expect(page.getByText("Running slow response", { exact: false })).toHaveCount(1);
  await verifyInterruptedBatchRetry(page, api, taskId, sessionId, options.seedData);
}

async function verifyInterruptedBatchRetry(
  page: Page,
  api: ApiClient,
  taskId: string,
  sessionId: string,
  seedData: SeedData,
) {
  const missingId = `missing-delivery-${taskId}`;
  await api.seedTaskSession(taskId, {
    state: "FAILED",
    sessionId: missingId,
    agentProfileId: seedData.agentProfileId,
    repositoryId: seedData.repositoryId,
    completedAt: new Date().toISOString(),
    metadata: {
      acp: { session_id: "retained-missing-native" },
      agent_delivery_recovery: {
        phase: "uncertain",
        revision: 1,
        session_id: missingId,
        agent_execution_id: "missing-execution",
        submission_id: "missing-submission",
        stream_id: "missing-stream",
        incarnation_id: missingId,
        harness_generation: 1,
        prompt_generation: 1,
      },
      last_agent_error: {
        code: "DURABLE_DELIVERY_UNCERTAIN",
        message: "Delivery was interrupted.",
        occurred_at: new Date().toISOString(),
      },
    },
  });
  await page.reload();
  await page.getByTestId("interrupted-sessions-open").click();
  await page.getByTestId(`interrupted-select-${sessionId}`).check();
  await page.getByTestId(`interrupted-select-${missingId}`).check();
  await page
    .getByTestId("interrupted-batch-instruction")
    .fill("Inspect retained changes before continuing");
  await page.getByTestId("interrupted-batch-acknowledge").check();
  await page.getByTestId("interrupted-batch-resume").click();
  await expect(page.getByTestId(`interrupted-result-${sessionId}`)).toContainText(
    "The new instruction was accepted",
  );
  const missing = page.getByTestId(`interrupted-result-${missingId}`);
  await expect(missing).toContainText(/saved prompt|saved session owner/i);
  await page.getByTestId("interrupted-batch-resume").click();
  await expect(missing).toContainText(/saved prompt|saved session owner/i);
  await page.reload();
  await page.getByTestId("interrupted-sessions-open").click();
  await expect(page.getByTestId(`interrupted-result-${sessionId}`)).toContainText(
    "The new instruction was accepted",
  );
  await expect(page.getByTestId(`interrupted-result-${missingId}`)).toContainText(
    /saved prompt|saved session owner/i,
  );
  const messages = (await api.listSessionMessages(sessionId)).messages;
  expect(
    messages.filter(
      (message) =>
        message.author_type === "user" &&
        message.content === "Inspect the saved changes and continue from them",
    ),
  ).toHaveLength(1);
  expect((await api.listSessionMessages(missingId)).messages).toHaveLength(0);
  const sibling = (await api.listTaskSessions(taskId)).sessions.find(
    (item) => item.id === missingId,
  )!;
  expect(sibling.metadata?.acp).toEqual({ session_id: "retained-missing-native" });
}
