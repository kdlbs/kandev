// This file starts with `mobile-` so Playwright runs it on the Pixel 5 project.
import { expect, test } from "../../fixtures/test-base";
import { SessionPage } from "../../pages/session-page";

const DONE_STATES = ["COMPLETED", "WAITING_FOR_INPUT"];

test.describe("Prompt history panel on mobile", () => {
  test("opens from Panels and returns to Chat for a prompt jump", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(90_000);

    const seedPrompt = "Mobile prompt history seeded prompt";
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Mobile prompt history task",
      seedData.agentProfileId,
      {
        description: seedPrompt,
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );
    if (!task.session_id) throw new Error("Mobile prompt history task did not create a session");

    await expect
      .poll(
        async () => {
          const { sessions } = await apiClient.listTaskSessions(task.id);
          return DONE_STATES.includes(sessions[0]?.state ?? "");
        },
        { timeout: 45_000 },
      )
      .toBe(true);

    const { messages } = await apiClient.listSessionMessages(task.session_id);
    const promptMessage = messages.find(
      (message) => message.author_type === "user" && message.content.includes(seedPrompt),
    );
    if (!promptMessage) throw new Error("Mobile prompt history prompt was not persisted");

    await testPage.goto(`/t/${task.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();

    const panelsButton = testPage.getByRole("button", { name: "Panels" });
    await expect(panelsButton).toBeVisible({ timeout: 15_000 });
    expect((await panelsButton.boundingBox())?.height).toBeGreaterThanOrEqual(44);
    await panelsButton.tap();

    const historyOption = testPage.getByTestId("mobile-prompt-history-option");
    await expect(historyOption).toBeVisible({ timeout: 10_000 });
    expect((await historyOption.boundingBox())?.height).toBeGreaterThanOrEqual(44);
    await historyOption.tap();

    const historyPanel = testPage.getByTestId("prompt-history-panel");
    await expect(historyPanel).toBeVisible({ timeout: 10_000 });
    const row = testPage.getByTestId("prompt-history-row-0");
    await expect(row).toContainText(seedPrompt);
    const prompt = row.locator('[role="button"]').first();
    const promptBox = await prompt.boundingBox();
    expect(promptBox?.height).toBeGreaterThanOrEqual(44);

    await prompt.tap();
    await expect(testPage.locator(`#msg-${promptMessage.id}`)).toBeAttached();
  });
});
