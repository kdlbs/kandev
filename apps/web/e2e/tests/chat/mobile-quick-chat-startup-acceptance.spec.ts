import { test, expect } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";
import { openMobileQuickChatSetup } from "./quick-chat-saved-prompt-delivery-helpers";
import { selectAgentIfNeeded } from "./quick-chat-helpers";
import {
  expectNoHorizontalOverflow,
  expectTouchTarget,
} from "./quick-chat-opening-composer-helpers";

type QuickChatAcceptance = {
  task_id: string;
  session_id: string;
  creation_id: string;
  creation_complete: boolean;
};

async function countOpeningMessages(
  apiClient: ApiClient,
  sessionId: string,
  prompt: string,
): Promise<number> {
  const { messages } = await apiClient.listSessionMessages(sessionId);
  return messages.filter((message) => message.author_type === "user" && message.content === prompt)
    .length;
}

test("keeps phone recovery reachable while a Quick Chat creation is incomplete", async ({
  testPage,
  apiClient,
}) => {
  test.setTimeout(120_000);
  await testPage.setViewportSize({ width: 390, height: 844 });
  const dialog = await openMobileQuickChatSetup(testPage);
  await selectAgentIfNeeded(dialog, testPage);
  const originalPrompt = "Keep this phone opening request until startup is accepted.";
  const newerDraft = "Keep this newer phone draft for later.";
  let acceptance: QuickChatAcceptance | null = null;
  let statusLookups = 0;
  let resolveCreationStatus = false;
  const creationIds: string[] = [];

  await testPage.route("**/api/v1/workspaces/*/quick-chat", async (route) => {
    if (route.request().method() !== "POST") return route.continue();
    const body = route.request().postDataJSON() as { creation_id?: string };
    expect(body.creation_id).toBeTruthy();
    creationIds.push(body.creation_id!);
    const upstream = await route.fetch();
    const upstreamBody = await upstream.text();
    acceptance = JSON.parse(upstreamBody) as QuickChatAcceptance;
    await route.fulfill({ status: 200, contentType: "application/json", body: "{malformed" });
  });
  await testPage.route(
    /\/api\/v1\/workspaces\/[^/]+\/quick-chat\/creations\/[^/]+$/,
    async (route) => {
      statusLookups += 1;
      if (!resolveCreationStatus) {
        await expect.poll(() => acceptance, { timeout: 30_000 }).not.toBeNull();
        const actual = acceptance as QuickChatAcceptance;
        await route.fulfill({
          status: 200,
          contentType: "application/json",
          body: JSON.stringify({
            task_id: actual.task_id,
            creation_id: actual.creation_id,
            creation_complete: false,
          }),
        });
        return;
      }
      await route.continue();
    },
  );

  const editor = dialog.getByTestId("task-description-input");
  await editor.fill(originalPrompt);
  const send = dialog.getByTestId("quick-chat-send");
  await expect(send).toBeEnabled();
  await expectTouchTarget(send);
  await send.tap();
  await expect
    .poll(() => acceptance, {
      timeout: 30_000,
      message: "the real backend must allocate before the malformed response",
    })
    .not.toBeNull();

  const setup = dialog.getByTestId("quick-chat-setup");
  const status = setup.getByTestId("quick-chat-creation-status");
  await expect(status).toBeVisible({ timeout: 30_000 });
  await expect(status).toContainText("This conversation is saved, but setup is still in progress");
  await expect(editor).toHaveValue(originalPrompt);
  await expect(setup.getByTestId("quick-chat-setup-error")).toHaveCount(0);
  await expect.poll(() => statusLookups, { timeout: 10_000 }).toBeGreaterThanOrEqual(1);

  const check = setup.getByTestId("quick-chat-creation-check");
  const retry = setup.getByTestId("quick-chat-creation-retry");
  const remove = setup.getByTestId("quick-chat-creation-delete");
  await expectTouchTarget(check);
  await expectTouchTarget(retry);
  await expectTouchTarget(remove);
  await expectNoHorizontalOverflow(testPage);

  await editor.fill(newerDraft);
  await remove.tap();
  const deleteDialog = testPage.getByRole("alertdialog");
  await expect(deleteDialog).toBeVisible();
  await deleteDialog.getByRole("button", { name: "Cancel" }).tap();
  await expect(deleteDialog).toBeHidden();
  await expect(status).toBeVisible();
  await expect(editor).toHaveValue(newerDraft);

  const checkResponse = testPage.waitForResponse(
    (response) =>
      response.request().method() === "GET" &&
      response.url().includes(`/quick-chat/creations/${creationIds[0]}`),
  );
  resolveCreationStatus = true;
  await check.tap();
  expect((await checkResponse).ok()).toBe(true);

  const accepted = acceptance;
  if (!accepted) throw new Error("The accepted Quick Chat identity is missing");
  await expect(
    dialog.locator(`[data-tab-reference="conversation:${accepted.session_id}"]`),
  ).toHaveCount(1);
  await expect(dialog.getByTestId("quick-chat-setup")).toHaveCount(0);
  await expect
    .poll(() => countOpeningMessages(apiClient, accepted.session_id, originalPrompt), {
      timeout: 30_000,
    })
    .toBe(1);
  expect(creationIds).toHaveLength(1);

  await dialog.getByTestId("quick-chat-add-menu-trigger").tap();
  await testPage.getByTestId("quick-chat-new-agent").tap();
  await expect(dialog.getByTestId("task-description-input")).toHaveValue(newerDraft);
  await expectNoHorizontalOverflow(testPage);
});
