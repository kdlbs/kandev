import { expect } from "@playwright/test";
import { test } from "../../fixtures/test-base";
import { SessionPage } from "../../pages/session-page";

const DONE_STATES = ["COMPLETED", "WAITING_FOR_INPUT"];

test.describe("Prompt turn duration on mobile", () => {
  test("keeps the settled prompt duration visible and contained", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const prompt = "Mobile prompt turn duration seeded prompt";
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Mobile prompt duration task",
      seedData.agentProfileId,
      {
        description: prompt,
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );
    if (!task.session_id) throw new Error("Mobile prompt duration task did not create a session");
    await expect
      .poll(
        async () => {
          const { sessions } = await apiClient.listTaskSessions(task.id);
          return DONE_STATES.includes(sessions[0]?.state ?? "");
        },
        { timeout: 45_000 },
      )
      .toBe(true);

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
    const row = session.activeChat().locator(`#msg-${messageId!}`);
    await expect(row).toHaveCount(1);
    const duration = row.getByTestId("message-turn-duration");
    const actions = duration.locator("xpath=..");
    await expect(duration).toBeVisible();
    await expect(actions).toHaveCSS("opacity", "1");
    await expect(duration).toHaveCSS("white-space", "nowrap");
    const durationBox = await duration.boundingBox();
    const actionsBox = await actions.boundingBox();
    const fits = await actions.evaluate(
      (action) => (action as HTMLElement).scrollWidth <= (action as HTMLElement).clientWidth,
    );
    const contained =
      !!durationBox &&
      !!actionsBox &&
      durationBox.x >= actionsBox.x &&
      durationBox.x + durationBox.width <= actionsBox.x + actionsBox.width;
    expect({ fits, contained }).toEqual({ fits: true, contained: true });

    await testPage.setViewportSize({ width: 700, height: 800 });
    await testPage.reload();
    await session.waitForLoad();
    expect(await testPage.evaluate(() => window.matchMedia("(pointer: coarse)").matches)).toBe(
      true,
    );

    const tabletRow = session.activeChat().locator(`#msg-${messageId!}`);
    await expect(tabletRow).toHaveCount(1);
    const tabletDuration = tabletRow.getByTestId("message-turn-duration");
    await expect(tabletDuration.locator("xpath=..")).toHaveCSS("opacity", "1");
  });
});
