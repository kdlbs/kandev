import fs from "node:fs";
import path from "node:path";
import { test, expect } from "../../fixtures/test-base";
import { openQuickChatSetup, selectAgentIfNeeded } from "./quick-chat-helpers";
import { completeQuickChatOpening, expectTouchTarget } from "./quick-chat-opening-composer-helpers";

test.describe("Quick Chat opening composer", () => {
  test("starts the opening prompt from the centered desktop composer", async ({
    testPage,
    apiClient,
  }, testInfo) => {
    await testPage.setViewportSize({ width: 1440, height: 900 });
    await testPage.goto("/");
    await testPage.waitForLoadState("networkidle");
    const dialog = await openQuickChatSetup(testPage, false);
    const setup = dialog.getByTestId("quick-chat-setup");
    const scrollRegion = dialog.getByTestId("quick-chat-setup-scroll");
    const editor = dialog.getByTestId("task-description-input");
    const composer = editor.locator("xpath=..");

    await expect(editor).toBeFocused();
    await selectAgentIfNeeded(dialog, testPage);
    await expect(dialog.locator("#quick-chat-agent-label")).toBeVisible();
    await expect(dialog.locator("#quick-chat-repositories-label")).toBeVisible();

    const desktopGeometry = await Promise.all([
      dialog.boundingBox(),
      composer.boundingBox(),
      setup.boundingBox(),
    ]);
    expect(desktopGeometry.every(Boolean)).toBe(true);
    const [dialogBox, composerBox, setupBox] = desktopGeometry;
    if (!dialogBox || !composerBox || !setupBox) throw new Error("desktop composer bounds missing");
    expect(
      Math.abs(composerBox.x + composerBox.width / 2 - (dialogBox.x + dialogBox.width / 2)),
    ).toBeLessThanOrEqual(4);
    expect(composerBox.y).toBeGreaterThan(setupBox.y);
    await testInfo.attach("quick-chat-opening-desktop", {
      body: await testPage.screenshot(),
      contentType: "image/png",
    });

    await editor.fill("Keep this draft while switching setup modes");
    const configurationToggle = dialog.getByRole("switch", { name: "Configuration chat" });
    await configurationToggle.click();
    await expect(configurationToggle).toHaveAttribute("aria-checked", "true");
    await expect(dialog.locator("#quick-chat-repositories-label")).toHaveCount(0);
    await expect(editor).toHaveValue("Keep this draft while switching setup modes");
    await configurationToggle.click();
    await expect(dialog.locator("#quick-chat-repositories-label")).toBeVisible();
    await expect(editor).toHaveValue("Keep this draft while switching setup modes");
    await editor.fill("");

    await testPage.setViewportSize({ width: 767, height: 850 });
    const narrowAgent = dialog.getByTestId("agent-profile-selector");
    const narrowAttach = dialog.getByRole("button", { name: "Attach files" });
    await expect
      .poll(async () => (await narrowAttach.boundingBox())?.width ?? 0)
      .toBeGreaterThanOrEqual(44);
    await expectTouchTarget(narrowAttach);
    await expectTouchTarget(dialog.getByTestId("quick-chat-send"));
    await expect(narrowAgent).toBeVisible();
    const narrowBox = await narrowAgent.boundingBox();
    expect(narrowBox?.height).toBeGreaterThanOrEqual(44);

    await testPage.setViewportSize({ width: 768, height: 850 });
    await expect.poll(async () => (await narrowAgent.boundingBox())?.height ?? 0).toBeLessThan(44);

    await testPage.setViewportSize({ width: 1440, height: 560 });
    const scrollMetrics = await scrollRegion.evaluate((element) => ({
      scrollHeight: element.scrollHeight,
      clientHeight: element.clientHeight,
      overflowY: getComputedStyle(element).overflowY,
    }));
    expect(scrollMetrics.overflowY).toBe("auto");
    expect(scrollMetrics.scrollHeight).toBeGreaterThanOrEqual(scrollMetrics.clientHeight);
    await expect(dialog.getByTestId("quick-chat-send")).toBeVisible();

    const prompt = "Explain the startup path in this repository";
    const attachmentName = "startup-notes.txt";
    const attachmentPath = path.join(testInfo.outputDir, attachmentName);
    fs.mkdirSync(testInfo.outputDir, { recursive: true });
    fs.writeFileSync(attachmentPath, "Follow the native launcher into the task session.");
    await completeQuickChatOpening(testPage, dialog, apiClient, prompt, {
      submit: (send) => send.click(),
      prepare: async () => {
        await dialog.locator('input[type="file"]').first().setInputFiles(attachmentPath);
        await expect(dialog.getByText(attachmentName, { exact: true })).toBeVisible();
      },
      expectedAttachmentName: attachmentName,
    });
  });

  test("keeps a failed creation draft and retries without duplicating the opening message", async ({
    testPage,
    apiClient,
  }) => {
    test.setTimeout(120_000);
    await testPage.goto("/");
    await testPage.waitForLoadState("networkidle");
    const dialog = await openQuickChatSetup(testPage, false);
    await selectAgentIfNeeded(dialog, testPage);
    const setup = dialog.getByTestId("quick-chat-setup");
    const editor = dialog.getByTestId("task-description-input");
    const send = dialog.getByTestId("quick-chat-send");
    const prompt = "Keep this prompt until the chat starts";
    let createAttempts = 0;

    await testPage.route("**/api/v1/workspaces/*/quick-chat", async (route) => {
      if (route.request().method() !== "POST") return route.continue();
      createAttempts += 1;
      if (createAttempts === 1) {
        return route.fulfill({
          status: 503,
          contentType: "application/json",
          body: JSON.stringify({ error: "Temporary start failure" }),
        });
      }
      return route.continue();
    });

    await editor.fill(prompt);
    await send.click();
    await expect(setup.getByRole("alert")).toBeVisible();
    await expect(editor).toHaveValue(prompt);
    await expect(send).toBeEnabled();
    expect(createAttempts).toBe(1);

    const startResponse = testPage.waitForResponse(
      (response) =>
        response.url().includes("/quick-chat") &&
        response.request().method() === "POST" &&
        response.ok(),
    );
    await send.click();
    const started = (await (await startResponse).json()) as { session_id: string };
    await expect
      .poll(async () => {
        const { messages } = await apiClient.listSessionMessages(started.session_id);
        return messages.filter(
          (message) => message.author_type === "user" && message.content === prompt,
        ).length;
      })
      .toBe(1);
    expect(createAttempts).toBe(2);
  });
});
