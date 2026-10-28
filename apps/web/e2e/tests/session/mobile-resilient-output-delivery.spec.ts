import { expect } from "@playwright/test";
import { test } from "../../fixtures/test-base";
import { SessionPage } from "../../pages/session-page";
import { attachGatewayTrafficCapture } from "../../helpers/ws-traffic";
import { dwell } from "../../helpers/causal-waits";
import { assertNoDocumentHorizontalOverflow } from "../../helpers/layout-assertions";
import type { SeedData } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";

async function createStartedTask(apiClient: ApiClient, seedData: SeedData, title: string) {
  return apiClient.createTaskWithAgent(seedData.workspaceId, title, seedData.agentProfileId, {
    workflow_id: seedData.workflowId,
    workflow_step_id: seedData.startStepId,
    repository_ids: [seedData.repositoryId],
    executor_profile_id: seedData.worktreeExecutorProfileId,
  });
}

function sentActionCount(
  frames: ReturnType<typeof attachGatewayTrafficCapture>["frames"],
  action: string,
) {
  return frames.filter((frame) => frame.direction === "sent" && frame.action === action).length;
}

// @covers AC-PLATFORM-BOUNDED-TOOL-DATA-001.1
test("a phone reaches the terminal of a near-limit search and accepts a distinct instruction", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  test.setTimeout(180_000);
  const task = await createStartedTask(apiClient, seedData, "Mobile resilient output delivery");
  await testPage.goto(`/t/${task.id}`);
  const session = new SessionPage(testPage);
  await session.waitForLoad();

  await session.sendMessageViaButton("/e2e:large-search");
  await expect(session.chat.getByText("SEARCH_DELIVERY_COMPLETE", { exact: false })).toBeVisible({
    timeout: 90_000,
  });
  await expect(testPage.getByTestId("failed-session-banner")).toHaveCount(0);
  await assertNoDocumentHorizontalOverflow(testPage, "mobile near-limit search delivery");

  await session.waitForDirectInput();
  await session.sendMessageViaButton("/e2e:simple-message");
  await expect(
    session.chat.getByText("This is a simple mock response for e2e testing.", { exact: false }),
  ).toBeVisible({ timeout: 60_000 });
  await assertNoDocumentHorizontalOverflow(testPage, "mobile distinct instruction");
});

// @covers AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.5
// @covers AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.25
test("a phone surfaces a real writer failure with reachable recovery controls", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  test.setTimeout(180_000);
  const capture = attachGatewayTrafficCapture(testPage);
  const task = await createStartedTask(apiClient, seedData, "Mobile real delivery failure");
  await testPage.goto(`/t/${task.id}`);
  const session = new SessionPage(testPage);
  await session.waitForLoad();

  const prompt = "/e2e:oversized-output-hold";
  await session.sendMessageViaButton(prompt);
  const banner = testPage
    .getByTestId("session-recovery-card")
    .or(testPage.getByTestId("failed-session-banner"))
    .first();
  await expect(banner).toBeVisible({ timeout: 60_000 });
  const retry = banner.getByTestId("recovery-retry-connection-button");
  const stop = banner.getByTestId("recovery-stop-button");
  await expect(retry).toBeVisible();
  await expect(stop).toBeEnabled();
  const retryBox = await retry.boundingBox();
  expect(retryBox).not.toBeNull();
  expect(retryBox!.height).toBeGreaterThanOrEqual(43);
  await assertNoDocumentHorizontalOverflow(testPage, "mobile delivery recovery controls");

  const userPrompts = async () => {
    const { messages } = await apiClient.listSessionMessages(task.session_id!);
    return messages.filter(
      (message) => message.author_type === "user" && message.content?.includes(prompt),
    ).length;
  };
  const promptsBeforeRetry = await userPrompts();
  expect(promptsBeforeRetry).toBe(1);
  await retry.tap();
  await expect
    .poll(() => sentActionCount(capture.frames, "session.recover"), { timeout: 15_000 })
    .toBe(1);
  await dwell(
    testPage,
    1_000,
    "negative-assertion",
    "Retry must never resend the original instruction",
  );
  expect(await userPrompts()).toBe(promptsBeforeRetry);

  await testPage.reload();
  await session.waitForLoad();
  await expect(banner).toBeVisible({ timeout: 30_000 });
  await expect(banner.getByTestId("recovery-stop-button")).toBeEnabled();
  await banner.getByTestId("recovery-stop-button").tap();
  await expect
    .poll(() => sentActionCount(capture.frames, "session.stop"), { timeout: 15_000 })
    .toBe(1);
  await assertNoDocumentHorizontalOverflow(testPage, "mobile delivery recovery after reload");
});
