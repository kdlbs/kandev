import { expect } from "@playwright/test";
import { test } from "../../fixtures/test-base";
import { SessionPage } from "../../pages/session-page";

const DONE_STATES = ["COMPLETED", "WAITING_FOR_INPUT"];
const DURATION_SHAPE = /^\d+s$|^\d+m \d+s$|^\d+h \d+m \d+s$/;

test.describe("Prompt turn duration", () => {
  test("reveals a settled prompt duration on hover and keyboard focus", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const prompt = "Prompt turn duration seeded prompt";
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Prompt duration task",
      seedData.agentProfileId,
      {
        description: prompt,
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );
    if (!task.session_id) throw new Error("Prompt duration task did not create a session");
    const settled = async () => {
      const { sessions } = await apiClient.listTaskSessions(task.id);
      return DONE_STATES.includes(sessions[0]?.state ?? "");
    };
    await expect.poll(settled, { timeout: 45_000 }).toBe(true);

    let messageId: string | null = null;
    await expect
      .poll(async () => {
        const { messages } = await apiClient.listSessionMessages(task.session_id!);
        messageId =
          messages.find((message) => message.author_type === "user" && message.content === prompt)
            ?.id ?? null;
        return messageId;
      })
      .not.toBeNull();

    await testPage.goto(`/t/${task.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await session.waitForDockviewReady();
    const chat = session.activeChat();
    const row = chat.locator(`#msg-${messageId!}`);
    await expect(row).toHaveCount(1);
    const duration = row.getByTestId("message-turn-duration");
    const actions = duration.locator("xpath=..");
    await expect(actions).toHaveCSS("opacity", "0");
    await row.locator(".group").hover();
    await expect(actions).toHaveCSS("opacity", "1");
    await expect(duration).toHaveText(DURATION_SHAPE);

    await testPage.mouse.move(0, 0);
    await expect(actions).toHaveCSS("opacity", "0");
    await row.locator("button").first().focus();
    await expect(actions).toHaveCSS("opacity", "1");
  });
});
