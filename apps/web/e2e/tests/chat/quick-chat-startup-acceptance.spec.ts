import { test, expect } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";
import { dwell } from "../../helpers/causal-waits";
import { openQuickChatSetup, selectAgentIfNeeded } from "./quick-chat-helpers";

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

test.describe("Quick Chat startup acceptance", () => {
  test("keeps an uncertain draft until exact status confirms the created chat", async ({
    testPage,
    apiClient,
  }) => {
    test.setTimeout(120_000);
    await testPage.goto("/");
    await testPage.waitForLoadState("networkidle");
    const dialog = await openQuickChatSetup(testPage, false);
    await selectAgentIfNeeded(dialog, testPage);
    const originalPrompt = "Retain the opening request through a gateway timeout.";
    const newerDraft = "Keep this newer text for my next chat.";
    const created: QuickChatAcceptance[] = [];
    const creationIds: string[] = [];
    let statusLookups = 0;
    let resolveCreationStatus = false;

    await testPage.route("**/api/v1/workspaces/*/quick-chat", async (route) => {
      if (route.request().method() !== "POST") return route.continue();
      const body = route.request().postDataJSON() as { creation_id?: string };
      expect(body.creation_id).toBeTruthy();
      creationIds.push(body.creation_id!);
      const upstream = await route.fetch();
      const acceptance = (await upstream.json()) as QuickChatAcceptance;
      created.push(acceptance);
      await route.fulfill({ status: 502, contentType: "text/plain", body: "Bad Gateway" });
    });
    await testPage.route(
      /\/api\/v1\/workspaces\/[^/]+\/quick-chat\/creations\/[^/]+$/,
      async (route) => {
        statusLookups += 1;
        if (statusLookups === 1 && created.length === 0) {
          await route.fulfill({
            status: 503,
            contentType: "application/json",
            body: JSON.stringify({ error: "Status service is temporarily unavailable" }),
          });
          return;
        }
        if (!resolveCreationStatus) {
          const actual = created[0];
          if (!actual) {
            await route.fulfill({
              status: 503,
              contentType: "application/json",
              body: JSON.stringify({ error: "Creation status is not ready" }),
            });
            return;
          }
          await route.fulfill({
            status: 200,
            contentType: "application/json",
            body: JSON.stringify({
              task_id: actual.task_id,
              session_id: actual.session_id,
              creation_id: "another-browser-submission",
              creation_complete: true,
            }),
          });
          return;
        }
        await route.continue();
      },
    );

    await dialog.getByTestId("task-description-input").fill(originalPrompt);
    await dialog.getByTestId("quick-chat-send").click();
    await expect
      .poll(() => created.length, {
        timeout: 30_000,
        message: "the backend must allocate the chat before the proxy error",
      })
      .toBe(1);
    await expect(dialog.getByTestId("quick-chat-creation-status")).toBeVisible({ timeout: 30_000 });
    const setup = dialog.getByTestId("quick-chat-setup");
    const editor = dialog.getByTestId("task-description-input");
    await expect(editor).toHaveValue(originalPrompt);
    await expect(setup.getByTestId("quick-chat-setup-error")).toHaveCount(0);
    await expect(setup.getByTestId("quick-chat-creation-status")).toContainText(
      "The server has not confirmed this conversation yet",
    );

    await setup.getByTestId("quick-chat-creation-check").click();
    await expect.poll(() => statusLookups, { timeout: 10_000 }).toBeGreaterThan(1);
    await expect(setup.getByTestId("quick-chat-creation-status")).toContainText(
      "The server has not confirmed this conversation yet",
    );
    await expect(
      dialog.locator(`[data-tab-reference="conversation:${created[0]!.session_id}"]`),
    ).toHaveCount(1);
    await expect(setup).toBeVisible();

    await editor.fill(newerDraft);
    const checkResponse = testPage.waitForResponse(
      (response) =>
        response.request().method() === "GET" &&
        response.url().includes(`/quick-chat/creations/${creationIds[0]}`),
    );
    resolveCreationStatus = true;
    await setup.getByTestId("quick-chat-creation-check").click();
    expect((await checkResponse).ok()).toBe(true);

    const accepted = created[0]!;
    await expect(
      dialog.locator(`[data-tab-reference="conversation:${accepted.session_id}"]`),
    ).toHaveCount(1);
    await expect(dialog.getByTestId("quick-chat-setup")).toHaveCount(0);
    await expect(
      dialog
        .getByTestId("quick-chat-messages")
        .getByTestId("user-message-bubble")
        .filter({ hasText: originalPrompt }),
    ).toBeVisible({ timeout: 30_000 });
    await expect
      .poll(() => countOpeningMessages(apiClient, accepted.session_id, originalPrompt), {
        timeout: 30_000,
      })
      .toBe(1);

    await dialog.getByTestId("quick-chat-add-menu-trigger").click();
    await testPage.getByTestId("quick-chat-new-agent").click();
    await expect(dialog.getByTestId("task-description-input")).toHaveValue(newerDraft);
    expect(creationIds).toHaveLength(1);
  });

  test("reloads an unknown creation without reposting and retries with the same identity", async ({
    testPage,
    apiClient,
  }) => {
    test.setTimeout(120_000);
    await testPage.goto("/");
    await testPage.waitForLoadState("networkidle");
    const dialog = await openQuickChatSetup(testPage, false);
    await selectAgentIfNeeded(dialog, testPage);
    const prompt = "Send this opening message once after the same-token retry.";
    const created: QuickChatAcceptance[] = [];
    const creationIds: string[] = [];
    let statusLookups = 0;

    await testPage.route("**/api/v1/workspaces/*/quick-chat", async (route) => {
      if (route.request().method() !== "POST") return route.continue();
      const body = route.request().postDataJSON() as { creation_id?: string };
      expect(body.creation_id).toBeTruthy();
      creationIds.push(body.creation_id!);
      const upstream = await route.fetch();
      const upstreamBody = await upstream.text();
      created.push(JSON.parse(upstreamBody) as QuickChatAcceptance);
      if (creationIds.length === 1) {
        await route.fulfill({ status: 502, contentType: "text/plain", body: "upstream closed" });
      } else {
        await route.fulfill({
          status: upstream.status(),
          headers: upstream.headers(),
          body: upstreamBody,
        });
      }
    });
    await testPage.route(
      /\/api\/v1\/workspaces\/[^/]+\/quick-chat\/creations\/[^/]+$/,
      async (route) => {
        statusLookups += 1;
        await route.fulfill({ status: 404, contentType: "application/json", body: "{}" });
      },
    );

    await dialog.getByTestId("task-description-input").fill(prompt);
    await dialog.getByTestId("quick-chat-send").click();
    await expect.poll(() => created.length, { timeout: 30_000 }).toBe(1);
    await expect(dialog.getByTestId("quick-chat-creation-status")).toContainText(
      "The server has not confirmed this conversation yet",
    );
    await expect.poll(() => statusLookups, { timeout: 10_000 }).toBeGreaterThanOrEqual(1);
    await expect(dialog.getByTestId("task-description-input")).toHaveValue(prompt);
    await expect(dialog.getByTestId("quick-chat-send")).toBeDisabled();
    const editor = dialog.getByTestId("task-description-input");
    await editor.press("Control+Enter");
    await dwell(
      testPage,
      100,
      "negative-assertion",
      "a blocked submit emits no request or WebSocket event to observe",
    );
    expect(creationIds).toHaveLength(1);
    expect(creationIds[0]).toBeTruthy();
    const allocated = created[0]!;
    await expect
      .poll(() => countOpeningMessages(apiClient, allocated.session_id, prompt), {
        timeout: 10_000,
      })
      .toBe(0);

    await testPage.reload();
    const reopened = await openQuickChatSetup(testPage, false);
    await expect(reopened.getByTestId("quick-chat-creation-status")).toBeVisible();
    await expect(reopened.getByTestId("task-description-input")).toHaveValue(prompt);
    expect(creationIds).toHaveLength(1);
    await expect
      .poll(() => countOpeningMessages(apiClient, allocated.session_id, prompt), {
        timeout: 10_000,
      })
      .toBe(0);

    const retryResponse = testPage.waitForResponse(
      (response) =>
        response.request().method() === "POST" &&
        new URL(response.url()).pathname.endsWith("/quick-chat"),
    );
    await reopened.getByTestId("quick-chat-creation-retry").click();
    expect((await retryResponse).ok()).toBe(true);
    await expect.poll(() => creationIds.length, { timeout: 30_000 }).toBe(2);
    expect(creationIds[1]).toBe(creationIds[0]);

    await expect(
      reopened.locator(`[data-tab-reference="conversation:${allocated.session_id}"]`),
    ).toHaveCount(1);
    await expect(
      reopened
        .getByTestId("quick-chat-messages")
        .getByTestId("user-message-bubble")
        .filter({ hasText: prompt }),
    ).toBeVisible({ timeout: 30_000 });
    await expect
      .poll(() => countOpeningMessages(apiClient, allocated.session_id, prompt), {
        timeout: 30_000,
      })
      .toBe(1);
    const sessions = await apiClient.listTaskSessions(allocated.task_id);
    expect(sessions.sessions.filter((session) => session.id === allocated.session_id)).toHaveLength(
      1,
    );
    await reopened.getByTestId("quick-chat-add-menu-trigger").click();
    await testPage.getByTestId("quick-chat-new-agent").click();
    await expect(reopened.getByTestId("task-description-input")).toHaveValue("");
  });
});
