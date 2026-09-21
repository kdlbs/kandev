import { test, expect } from "../../fixtures/test-base";
import { resetWorkspaceOrchestrators } from "../../helpers/orchestration";

test("Orchestrator opens at the latest messages and respects reading older messages", async ({
  testPage: page,
  backend,
  apiClient,
  seedData,
}) => {
  await backend.restart({
    KANDEV_FEATURES_ORCHESTRATION: "true",
    KANDEV_FEATURES_OFFICE: "false",
  });
  const ws = seedData.workspaceId;
  await resetWorkspaceOrchestrators(page.request, backend.baseUrl, ws);
  await page.goto(`/settings/workspaces/${ws}/orchestration/new`);
  await page.getByTestId("orchestrator-profile").click();
  await page.getByRole("option").first().click();
  await page.getByTestId("persona-executor-profile").click();
  await page.getByRole("option").filter({ hasNotText: "Inherit" }).first().click();
  await page.getByRole("button", { name: "Add orchestrator", exact: true }).click();
  await expect(page).not.toHaveURL(/\/new$/);
  const chief = new URL(page.url()).pathname.split("/").pop()!;
  const response = await page.request.post(
    `${backend.baseUrl}/api/v1/orchestration/workspaces/${ws}/orchestrators/${chief}/conversation`,
  );
  expect(response.ok()).toBeTruthy();
  const { task_id: taskId } = await response.json();
  const seedMessage = (body: string) =>
    apiClient.seedComment({ taskId, authorType: "user", authorId: "example-user", body });
  for (let index = 0; index < 24; index++) {
    await seedMessage(`Example request ${index + 1}: review the sample checklist.`);
  }
  const chat = page.getByTestId("orchestrator-conversation");
  const bottomGap = () =>
    chat.evaluate((element) => element.scrollHeight - element.clientHeight - element.scrollTop);
  for (const viewport of [
    { width: 1440, height: 1000 },
    { width: 390, height: 844 },
  ]) {
    await page.setViewportSize(viewport);
    await page.goto(`/workspaces/${ws}/coordinator?orchestratorId=${chief}`);
    await expect(chat.getByText("Example request 24: review the sample checklist.")).toBeAttached();
    if (viewport.width < 768) {
      await page.getByRole("tab", { name: "Chat", exact: true }).click();
    }
    // Prove the history actually overflows before checking the initial scroll position.
    await expect
      .poll(() => chat.evaluate((el) => el.scrollHeight - el.clientHeight))
      .toBeGreaterThan(500);
    await expect.poll(bottomGap).toBeLessThanOrEqual(2);

    const latest = await seedMessage(`Summarize the sample checklist at width ${viewport.width}.`);
    await expect(chat.locator(`#comment-${latest.comment_id}`)).toBeAttached();
    await expect.poll(bottomGap).toBeLessThanOrEqual(2);

    await chat.hover();
    await page.mouse.wheel(0, -10000);
    await expect.poll(() => chat.evaluate((el) => el.scrollTop)).toBe(0);
    const later = await seedMessage(`Review the example headings at width ${viewport.width}.`);
    await expect(chat.locator(`#comment-${later.comment_id}`)).toBeAttached();
    await expect.poll(() => chat.evaluate((el) => el.scrollTop)).toBe(0);

    await page.goto(`/?workspaceId=${ws}`);
    await page.goto(`/workspaces/${ws}/coordinator?orchestratorId=${chief}`);
    await expect(chat.locator(`#comment-${later.comment_id}`)).toBeAttached();
    if (viewport.width < 768) {
      await page.getByRole("tab", { name: "Chat", exact: true }).click();
    }
    await expect.poll(bottomGap).toBeLessThanOrEqual(2);
  }
});
