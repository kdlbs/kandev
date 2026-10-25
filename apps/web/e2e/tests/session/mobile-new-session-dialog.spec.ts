import { test, expect } from "../../fixtures/test-base";
import { waitForSessionDone } from "../../helpers/session";
import { SessionPage } from "../../pages/session-page";
import { createRecentUseProfiles, seedTaskSessionRecency } from "./new-session-recency-helpers";

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

  test("uses task-session recency for the mobile Add Agent default", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(300_000);
    const { profileA, profileB } = await createRecentUseProfiles(apiClient);

    try {
      const { targetTaskId } = await seedTaskSessionRecency({
        testPage,
        apiClient,
        seedData,
        profileA,
        profileB,
        mobile: true,
      });
      await testPage.goto(`/t/${targetTaskId}`);
      const session = new SessionPage(testPage);
      await session.waitForLoad();
      await session.waitForChatIdle({ timeout: 30_000 });
      await session.openMobileNewSessionDialog();

      const selector = session.newSessionDialogPage.agentSelector();
      await expect(selector).toContainText(profileB.name);
      await selector.tap();
      const options = session.newSessionDialogPage.agentOptions();
      await expect(options.filter({ hasText: profileB.name })).toHaveCount(1);
      await expect(options.first()).toContainText(profileB.name);
      await testPage.keyboard.press("Escape");

      await session.newSessionPromptInput().fill("/e2e:simple-message");
      await session.newSessionStartButton().tap();
      await expect(session.newSessionDialog()).not.toBeVisible({ timeout: 10_000 });

      await expect
        .poll(
          async () => {
            const { sessions } = await apiClient.listTaskSessions(targetTaskId);
            return sessions.find((candidate) => candidate.agent_profile_id === profileB.id)
              ?.agent_profile_id;
          },
          { timeout: 30_000, message: "Waiting for the recent mobile profile session" },
        )
        .toBe(profileB.id);
    } finally {
      await apiClient.deleteAgentProfile(profileA.id, true).catch(() => undefined);
      await apiClient.deleteAgentProfile(profileB.id, true).catch(() => undefined);
    }
  });

  test("opens New Agent from a completed session without horizontal overflow", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);

    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Mobile Completed Session New Agent Task",
      seedData.agentProfileId,
      {
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );
    if (!task.session_id) throw new Error("createTaskWithAgent did not return a session_id");

    await waitForSessionDone(apiClient, task.id, task.session_id, "Waiting for first session");
    await apiClient.seedTaskSession(task.id, {
      state: "COMPLETED",
      sessionId: task.session_id,
      agentProfileId: seedData.agentProfileId,
      completedAt: new Date().toISOString(),
    });
    await expect
      .poll(async () => {
        const { sessions } = await apiClient.listTaskSessions(task.id);
        return sessions.find((session) => session.id === task.session_id)?.state;
      })
      .toBe("COMPLETED");

    await testPage.goto(`/t/${task.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();

    const banner = session.completedSessionBanner();
    const newAgent = session.completedSessionNewAgentButton();
    await expect(banner).toBeVisible({ timeout: 15_000 });
    await expect(session.activeChat().locator(".tiptap.ProseMirror")).not.toBeVisible();

    const [bannerBox, buttonBox] = await Promise.all([
      banner.boundingBox(),
      newAgent.boundingBox(),
    ]);
    const viewport = testPage.viewportSize();
    expect(bannerBox).not.toBeNull();
    expect(buttonBox).not.toBeNull();
    expect(viewport).not.toBeNull();
    expect(buttonBox!.height).toBeGreaterThanOrEqual(44);
    expect(buttonBox!.x).toBeGreaterThanOrEqual(bannerBox!.x);
    expect(buttonBox!.x + buttonBox!.width).toBeLessThanOrEqual(bannerBox!.x + bannerBox!.width);
    expect(buttonBox!.x + buttonBox!.width).toBeLessThanOrEqual(viewport!.width);
    expect(
      await testPage.evaluate(
        () => document.documentElement.scrollWidth <= document.documentElement.clientWidth,
      ),
    ).toBe(true);

    await newAgent.tap();
    await expect(session.newSessionDialog()).toBeVisible({ timeout: 5_000 });

    const dialogBox = await session.sessionLaunchDialog().boundingBox();
    expect(dialogBox).not.toBeNull();
    expect(dialogBox!.x).toBeGreaterThanOrEqual(0);
    expect(dialogBox!.y).toBeGreaterThanOrEqual(0);
    expect(dialogBox!.x + dialogBox!.width).toBeLessThanOrEqual(viewport!.width);
    expect(dialogBox!.y + dialogBox!.height).toBeLessThanOrEqual(viewport!.height);

    await session.newSessionPromptInput().fill("/e2e:simple-message");
    await session.newSessionStartButton().tap();
    await expect(session.newSessionDialog()).not.toBeVisible({ timeout: 10_000 });
    await expect
      .poll(async () => (await apiClient.listTaskSessions(task.id)).sessions.length, {
        timeout: 30_000,
      })
      .toBe(2);
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
