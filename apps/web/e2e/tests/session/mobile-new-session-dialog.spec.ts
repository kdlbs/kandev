import { test, expect } from "../../fixtures/test-base";
import { SessionPage } from "../../pages/session-page";

const DONE_STATES = ["COMPLETED", "WAITING_FOR_INPUT"];
const PROMPT_NAME = "e2e-mobile-new-agent-prompt";
const PROMPT_CONTENT = "Review this task from the mobile launch flow.";

test.describe("New session dialog on mobile", () => {
  test.afterEach(async ({ apiClient }) => {
    const { prompts } = await apiClient.listPrompts();
    for (const prompt of prompts) {
      if (!prompt.builtin && prompt.name === PROMPT_NAME) {
        await apiClient.deletePrompt(prompt.id);
      }
    }
  });

  test("selects a saved prompt and launches from the mobile session controls", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);
    await apiClient.createPrompt(PROMPT_NAME, PROMPT_CONTENT);

    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Mobile Saved Prompt Task",
      seedData.agentProfileId,
      {
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );

    await expect
      .poll(
        async () => {
          const { sessions } = await apiClient.listTaskSessions(task.id);
          return DONE_STATES.includes(sessions[0]?.state ?? "");
        },
        { timeout: 30_000 },
      )
      .toBe(true);

    await testPage.goto(`/t/${task.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await session.openMobileNewSessionDialog();
    await expect(session.newSessionDialog()).toBeVisible({ timeout: 5_000 });

    const dialogBox = await session.sessionLaunchDialog().boundingBox();
    const viewport = testPage.viewportSize();
    expect(dialogBox).not.toBeNull();
    expect(viewport).not.toBeNull();
    expect(dialogBox!.x).toBeGreaterThanOrEqual(0);
    expect(dialogBox!.y).toBeGreaterThanOrEqual(0);
    expect(dialogBox!.x + dialogBox!.width).toBeLessThanOrEqual(viewport!.width);
    expect(dialogBox!.y + dialogBox!.height).toBeLessThanOrEqual(viewport!.height);

    const prompt = session.newSessionPromptInput();
    await prompt.tap();
    await prompt.pressSequentially("@e2e-mobile");
    await expect(testPage.getByText(/Mention tasks, files, prompts/i)).toBeVisible();
    await testPage.getByRole("option", { name: new RegExp(PROMPT_NAME) }).tap();
    await expect(prompt).toHaveValue(PROMPT_CONTENT);
    await expect(session.newSessionDialog()).toBeVisible();

    expect(
      await testPage.evaluate(
        () => document.documentElement.scrollWidth <= document.documentElement.clientWidth,
      ),
    ).toBe(true);

    await session.newSessionStartButton().tap();
    await expect(session.newSessionDialog()).not.toBeVisible({ timeout: 10_000 });
    await expect
      .poll(async () => (await apiClient.listTaskSessions(task.id)).sessions.length, {
        timeout: 30_000,
      })
      .toBe(2);
  });
});
