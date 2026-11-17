import { waitForHttp } from "../../helpers/causal-waits";
import { waitForFiniteAnimations } from "../../helpers/pr-capture";
import { test, expect } from "../../fixtures/test-base";

// @covers AC-TASKS-CONVERSATION-SEARCH-001.1, AC-TASKS-CONVERSATION-SEARCH-001.3
test("finds conversation text, opens its session, and removes edited text", async ({
  testPage,
  apiClient,
  seedData,
  prCapture,
}) => {
  const task = await apiClient.createTask(seedData.workspaceId, "Conversation destination", {
    workflow_id: seedData.workflowId,
    workflow_step_id: seedData.startStepId,
  });
  const session = await apiClient.seedTaskSession(task.id, {
    state: "COMPLETED",
    startedAt: new Date(Date.now() - 60000).toISOString(),
    completedAt: new Date().toISOString(),
  });
  const message = await apiClient.seedSessionMessage(session.session_id, {
    type: "message",
    content: "Discuss the quartzneedle budget here.",
    authorType: "user",
  });
  await testPage.goto("/");
  await testPage.keyboard.press("Control+k");
  const palette = testPage.getByRole("dialog", { name: "Command Palette", exact: true });
  const input = palette.getByRole("combobox");
  const row = palette.getByRole("option").filter({ hasText: "Conversation destination" });
  await expect(row).toBeVisible();
  expect((await row.boundingBox())!.height).toBeLessThanOrEqual(36);
  await input.fill("Conversation destination");
  await expect(row).toBeVisible();
  expect((await row.boundingBox())!.height).toBeLessThanOrEqual(36);
  await input.fill("quartzneedle");
  await expect(row).toBeVisible();
  await expect(row.locator("mark")).toHaveText("quartzneedle");
  if (prCapture.capturing) {
    await waitForFiniteAnimations(palette);
    await prCapture.screenshot("desktop-conversation-search");
  }
  await row.click();
  await expect(testPage).toHaveURL(new RegExp(`/t/${task.id}\\?sessionId=${session.session_id}`));
  await apiClient.updateSessionMessage(message.messageId, "The replacement conversation text.");
  await testPage.keyboard.press("Control+k");
  const searched = waitForHttp(testPage, "GET", /\/tasks\/conversations$/, {
    predicate: (response) => new URL(response.url()).searchParams.get("query") === "quartzneedle",
  });
  await palette.getByRole("combobox").fill("quartzneedle");
  const response = await searched;
  const result = await response.json();
  expect(result.tasks).toEqual([]);
  expect(["ready", "indexing"]).toContain(result.state);
  await expect(
    palette.getByRole("option").filter({ hasText: "Conversation destination" }),
  ).toHaveCount(0);
});
