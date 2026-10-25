import { expect, type Page } from "@playwright/test";
import type { SeedData } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";
import { SessionPage } from "../../pages/session-page";

export const TASK_TITLE = "Investigate **terminal** turn capture retry loop";

export async function sendTaskMentions(page: Page, api: ApiClient, seed: SeedData) {
  const target = await api.createTask(seed.workspaceId, TASK_TITLE, {
    workflow_id: seed.workflowId,
    workflow_step_id: seed.startStepId,
  });
  const source = await api.createTaskWithAgent(
    seed.workspaceId,
    "Check the terminal retry fix",
    seed.agentProfileId,
    {
      description: "/e2e:simple-message",
      workflow_id: seed.workflowId,
      workflow_step_id: seed.startStepId,
      repository_ids: [seed.repositoryId],
    },
  );
  await page.goto(`/t/${source.id}`);
  const session = new SessionPage(page);
  await session.waitForLoad();
  await session.waitForChatIdle();
  const editor = await session.fillMessage("check if ");
  for (let index = 0; index < 2; index++) {
    await editor.pressSequentially("@Investigate");
    const option = page.getByRole("option").filter({ hasText: TASK_TITLE });
    await expect(option).toBeVisible();
    await option.click();
    await editor.pressSequentially(" ");
  }
  await editor.pressSequentially("is fixing it? @ordinary");
  await editor.press("Escape");
  const isTouch = await page.evaluate(() => matchMedia("(pointer: coarse)").matches);
  if (isTouch) await session.tapSubmitWhenReady();
  else await session.submitMessageWithKeyboard(editor);
  const bubble = session
    .activeChat()
    .getByTestId("user-message-bubble")
    .filter({ hasText: "is fixing it?" });
  await expect(bubble).toBeVisible();
  return { target, source, session, bubble };
}
