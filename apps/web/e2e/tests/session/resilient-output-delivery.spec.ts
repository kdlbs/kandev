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
// @covers AC-PLATFORM-BOUNDED-TOOL-DATA-004.2
test("near-limit search reaches its terminal and admits one distinct next instruction", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  test.setTimeout(180_000);
  const task = await createStartedTask(apiClient, seedData, "Resilient output delivery");
  await testPage.goto(`/t/${task.id}`);
  const session = new SessionPage(testPage);
  await session.waitForLoad();

  await session.sendMessage("/e2e:large-search");
  await expect(session.chat.getByText("SEARCH_DELIVERY_COMPLETE", { exact: false })).toBeVisible({
    timeout: 90_000,
  });
  const searchRow = session.chat.getByText(/Found \d+ files?/).first();
  await expect(searchRow).toBeVisible();
  await searchRow.click();
  await expect(session.chat.getByText("a.go", { exact: false }).first()).toBeVisible();
  await expect(testPage.getByTestId("failed-session-banner")).toHaveCount(0);
  await expect(testPage.getByTestId("session-recovery-card")).toHaveCount(0);

  await session.waitForDirectInput();
  await session.sendMessage("/e2e:simple-message");
  await expect(
    session.chat.getByText("This is a simple mock response for e2e testing.", { exact: false }),
  ).toBeVisible({ timeout: 60_000 });
  await assertNoDocumentHorizontalOverflow(testPage, "near-limit search delivery");
});

// @covers AC-PLATFORM-BOUNDED-TOOL-DATA-003.1
// @covers AC-PLATFORM-BOUNDED-TOOL-DATA-003.2
test("larger search results keep the existing truncation representation", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  test.setTimeout(180_000);
  const task = await createStartedTask(apiClient, seedData, "Bounded tool display");
  await testPage.goto(`/t/${task.id}`);
  const session = new SessionPage(testPage);
  await session.waitForLoad();

  await session.sendMessage("/e2e:large-search-overflow");
  await expect(session.chat.getByText("SEARCH_OVERFLOW_COMPLETE", { exact: false })).toBeVisible({
    timeout: 90_000,
  });
  const toolRow = session.chat.getByText(/Found \d+ files?/).first();
  await expect(toolRow).toBeVisible();
  await toolRow.click();
  await expect(
    session.chat.getByText("...and more files (truncated)", { exact: false }),
  ).toBeVisible({
    timeout: 30_000,
  });
  await expect(testPage.getByTestId("failed-session-banner")).toHaveCount(0);
});

// @covers AC-PLATFORM-DURABLE-AGENT-DELIVERY-001.2
// @covers AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.12
test("a real oversized write recovers the writer, keeps a draft, and admits one next instruction", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  test.setTimeout(180_000);
  const task = await createStartedTask(apiClient, seedData, "Real delivery write recovery");
  await testPage.goto(`/t/${task.id}`);
  const session = new SessionPage(testPage);
  await session.waitForLoad();

  await session.sendMessage("/e2e:oversized-output");
  await session.waitForDirectInput(90_000);
  await expect(testPage.getByTestId("failed-session-banner")).toHaveCount(0);
  await expect(testPage.getByTestId("session-recovery-card")).toHaveCount(0);

  const editor = await session.fillMessage("draft kept after real delivery recovery");
  await testPage.reload();
  await session.waitForLoad();
  await expect(editor).toContainText("draft kept after real delivery recovery");

  // The retained draft makes the composer non-empty, so submit directly
  // instead of waiting for the idle placeholder again.
  await editor.fill("/e2e:simple-message");
  await session.submitMessageWithKeyboard(editor);
  await expect(
    session.chat.getByText("This is a simple mock response for e2e testing.", { exact: false }),
  ).toBeVisible({ timeout: 60_000 });
});

// @covers AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.5
// @covers AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.25
test("an unresolved writer failure surfaces recovery without resending the instruction", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  test.setTimeout(180_000);
  const capture = attachGatewayTrafficCapture(testPage);
  const task = await createStartedTask(apiClient, seedData, "Real delivery failure recovery");
  await testPage.goto(`/t/${task.id}`);
  const session = new SessionPage(testPage);
  await session.waitForLoad();

  const prompt = "/e2e:oversized-output-hold";
  await session.sendMessage(prompt);
  const banner = testPage
    .getByTestId("session-recovery-card")
    .or(testPage.getByTestId("failed-session-banner"))
    .first();
  await expect(banner).toBeVisible({ timeout: 60_000 });
  await expect(banner.getByTestId("recovery-retry-connection-button")).toBeVisible();
  await expect(banner.getByTestId("recovery-stop-button")).toBeEnabled();
  const userPrompts = async () => {
    const { messages } = await apiClient.listSessionMessages(task.session_id!);
    return messages.filter(
      (message) => message.author_type === "user" && message.content?.includes(prompt),
    ).length;
  };
  const promptsBeforeRetry = await userPrompts();
  expect(promptsBeforeRetry).toBe(1);

  await banner.getByTestId("recovery-retry-connection-button").click();
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
  await banner.getByTestId("recovery-stop-button").click();
  await expect
    .poll(() => sentActionCount(capture.frames, "session.stop"), { timeout: 15_000 })
    .toBe(1);
});
