import { type Locator, type Page } from "@playwright/test";
import { test, expect } from "../../fixtures/test-base";
import type { SeedData } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";
import { assertNoDocumentHorizontalOverflow } from "../../helpers/layout-assertions";
import { waitForSessionDone } from "../../helpers/session";
import { SessionPage } from "../../pages/session-page";

const LATEST_MARKER = "MOBILE_JUMP_TO_LATEST_REPLY_5N7P";

function messageCommand(content: string): string {
  return `e2e:message(${JSON.stringify(content)})`;
}

function assistantReply(chat: Locator, marker: string): Locator {
  return chat
    .locator("[data-agent-message-body][data-message-id]")
    .filter({ hasText: marker })
    .first();
}

async function seedOverflowingTask(
  testPage: Page,
  apiClient: ApiClient,
  seedData: SeedData,
): Promise<SessionPage> {
  const messages = Array.from({ length: 30 }, (_, index) =>
    messageCommand(
      `Mobile jump history ${index + 1}: lorem ipsum dolor sit amet consectetur adipiscing elit sed do eiusmod tempor incididunt ut labore et dolore magna aliqua`,
    ),
  );
  messages.push(messageCommand(LATEST_MARKER));
  const task = await apiClient.createTaskWithAgent(
    seedData.workspaceId,
    "Mobile jump to latest navigation",
    seedData.agentProfileId,
    {
      description: messages.join("\n"),
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    },
  );
  if (!task.session_id) throw new Error("createTaskWithAgent did not return a session_id");
  await waitForSessionDone(
    apiClient,
    task.id,
    task.session_id,
    "mobile latest history should finish",
  );
  await testPage.goto(`/t/${task.id}`);
  const session = new SessionPage(testPage);
  await session.waitForLoad();
  await session.waitForChatIdle({ timeout: 30_000 });
  return session;
}

async function scrollUpWithTouch(testPage: Page, list: ReturnType<SessionPage["activeChat"]>) {
  const box = await list.boundingBox();
  expect(box).not.toBeNull();
  const startTop = await list.evaluate((element) => element.scrollTop);
  const scrollEnded = list.evaluate(
    (element) =>
      new Promise<void>((resolve) => {
        element.addEventListener("scrollend", () => resolve(), { once: true });
      }),
  );
  const client = await testPage.context().newCDPSession(testPage);
  const point = { x: box!.x + box!.width / 2, y: box!.y + box!.height / 2 };
  await client.send("Input.dispatchTouchEvent", { type: "touchStart", touchPoints: [point] });
  await client.send("Input.dispatchTouchEvent", {
    type: "touchMove",
    touchPoints: [{ ...point, y: point.y + 30 }],
  });
  await client.send("Input.dispatchTouchEvent", { type: "touchEnd", touchPoints: [] });
  await client.detach();
  await scrollEnded;
  await expect
    .poll(async () => startTop - (await list.evaluate((element) => element.scrollTop)))
    .toBeGreaterThan(5);
}

test("phone Jump to latest fits coarse targets at 390, 767, and 768 pixels", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  test.setTimeout(180_000);
  const session = await seedOverflowingTask(testPage, apiClient, seedData);
  const chat = session.activeChat();
  const list = chat.locator(".chat-message-list");
  const statusBar = chat.getByTestId("chat-status-bar");
  const button = statusBar.getByTestId("jump-to-latest-button");
  const editor = chat.locator(".tiptap.ProseMirror").first();

  for (const width of [390, 767, 768]) {
    await testPage.setViewportSize({ width, height: 844 });
    await expect
      .poll(async () =>
        list.evaluate((element) => element.scrollHeight - element.scrollTop - element.clientHeight),
      )
      .toBeLessThan(10);
    await expect(button).toHaveCount(0);
    await scrollUpWithTouch(testPage, list);
    await expect(button).toBeVisible();

    const buttonBox = await button.boundingBox();
    const editorBox = await editor.boundingBox();
    expect(buttonBox).not.toBeNull();
    expect(editorBox).not.toBeNull();
    expect(Math.round(buttonBox!.width)).toBeGreaterThanOrEqual(44);
    expect(Math.round(buttonBox!.height)).toBeGreaterThanOrEqual(44);
    expect(buttonBox!.x).toBeGreaterThanOrEqual(0);
    expect(buttonBox!.x + buttonBox!.width).toBeLessThanOrEqual(width + 1);
    expect(buttonBox!.y + buttonBox!.height).toBeLessThanOrEqual(editorBox!.y + 1);
    await assertNoDocumentHorizontalOverflow(testPage);

    if (width === 390) {
      await testPage.screenshot({ path: test.info().outputPath("jump-to-latest-phone.png") });
    }

    await button.tap();
    await expect
      .poll(async () =>
        list.evaluate((element) => element.scrollHeight - element.scrollTop - element.clientHeight),
      )
      .toBeLessThan(10);
    await expect(assistantReply(chat, LATEST_MARKER)).toBeInViewport();
    await expect(button).toHaveCount(0);
    await expect
      .poll(async () => list.evaluate((element) => document.activeElement === element))
      .toBe(true);
  }
});
