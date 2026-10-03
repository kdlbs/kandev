import fs from "node:fs";
import path from "node:path";
import { test, expect } from "../../fixtures/test-base";
import { openQuickChatSetup, selectAgentIfNeeded } from "./quick-chat-helpers";
import { completeQuickChatOpening, expectTouchTarget } from "./quick-chat-opening-composer-helpers";

function findMaterializedAttachment(root: string, name: string): string | null {
  const pending: string[] = [root];
  while (pending.length > 0) {
    const directory = pending.pop();
    if (!directory) continue;
    let entries: fs.Dirent[];
    try {
      entries = fs.readdirSync(directory, { withFileTypes: true });
    } catch {
      continue;
    }
    for (const entry of entries) {
      const fullPath = path.join(directory, entry.name);
      if (entry.isDirectory()) {
        if (entry.name !== "node_modules" && entry.name !== ".git") pending.push(fullPath);
      } else if (entry.isFile() && entry.name === name && fullPath.includes(`${path.sep}.kandev`)) {
        return fullPath;
      }
    }
  }
  return null;
}

test.describe("Quick Chat opening composer", () => {
  test("starts the opening prompt from the centered desktop composer", async ({
    testPage,
    apiClient,
    backend,
    prCapture,
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
    await expect(dialog.getByTestId("agent-profile-selector")).toContainText("Mock");
    await expect(dialog.getByTestId("add-repository")).toBeEnabled();

    const attach = dialog.getByRole("button", { name: "Attach files" });
    const addRepo = dialog.getByTestId("add-repository");
    const send = dialog.getByTestId("quick-chat-send");
    const toolbarPadding = await send.locator("xpath=../..").evaluate((element) => {
      const style = getComputedStyle(element);
      return { top: style.paddingTop, bottom: style.paddingBottom };
    });
    expect(toolbarPadding.top).toBe(toolbarPadding.bottom);
    expect(parseFloat(toolbarPadding.top)).toBeGreaterThan(0);
    const bounds = await Promise.all([
      addRepo.boundingBox(),
      attach.boundingBox(),
      send.boundingBox(),
    ]);
    const [repoBounds, attachBounds, sendBounds] = bounds;
    if (!repoBounds || !attachBounds || !sendBounds) throw new Error("toolbar bounds missing");
    expect(repoBounds.x + repoBounds.width).toBeLessThanOrEqual(attachBounds.x);
    expect(
      Math.abs(attachBounds.y + attachBounds.height / 2 - sendBounds.y - sendBounds.height / 2),
    ).toBeLessThanOrEqual(1);
    await addRepo.click();
    const repositoryPicker = testPage.getByTestId("quick-chat-repository-picker");
    await expect(repositoryPicker).toBeVisible();
    await expect(dialog.getByTestId("repo-chip")).toHaveCount(0);
    await repositoryPicker.getByRole("option").first().click();
    await expect(composer.getByTestId("repo-chip")).toHaveCount(1);
    await expect(dialog.getByTestId("branch-chip-trigger")).toContainText("main");
    const chipRowBox = await dialog.getByTestId("quick-chat-repository-chips").boundingBox();
    const chipBox = await dialog.getByTestId("repo-chip").boundingBox();
    if (!chipRowBox || !chipBox) throw new Error("repository chip bounds missing");
    expect(
      Math.abs(
        chipBox.y - chipRowBox.y - (chipRowBox.y + chipRowBox.height - chipBox.y - chipBox.height),
      ),
    ).toBeLessThanOrEqual(1);
    await prCapture.screenshot("toolbar-desktop", {
      caption: "Quick Chat composer with repository chips and compact toolbar",
    });
    await composer.getByTestId("remove-repo-chip").click();

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
    const introductionBox = await dialog.getByTestId("quick-chat-introduction").boundingBox();
    const agentBox = await dialog.getByTestId("agent-profile-selector").boundingBox();
    if (!introductionBox || !agentBox) throw new Error("composer group bounds missing");
    expect(
      Math.abs(
        (introductionBox.y + agentBox.y + agentBox.height) / 2 - (setupBox.y + setupBox.height / 2),
      ),
    ).toBeLessThanOrEqual(4);
    await testInfo.attach("quick-chat-opening-desktop", {
      body: await testPage.screenshot(),
      contentType: "image/png",
    });

    await editor.fill("Keep this draft while switching setup modes");
    const configurationToggle = dialog.getByRole("switch", { name: "Configuration chat" });
    await configurationToggle.hover();
    await expect(testPage.getByRole("tooltip")).toContainText(
      "settings, workflows, agent profiles",
    );
    const inactiveColor = await configurationToggle.evaluate(
      (element) => getComputedStyle(element).backgroundColor,
    );
    await configurationToggle.click();
    await expect(configurationToggle).toHaveAttribute("data-variant", "default");
    expect(
      await configurationToggle.evaluate((element) => getComputedStyle(element).backgroundColor),
    ).not.toBe(inactiveColor);
    await expect(configurationToggle).toHaveAttribute("aria-checked", "true");
    await expect(dialog.getByTestId("add-repository")).toBeDisabled();
    await expect(composer.getByRole("status")).toHaveText("Configuration chat");
    await expect(editor).toHaveValue("Keep this draft while switching setup modes");
    await configurationToggle.click();
    await expect(dialog.getByTestId("add-repository")).toBeEnabled();
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

    await testPage.setViewportSize({ width: 1440, height: 300 });
    await expect
      .poll(() => scrollRegion.evaluate((element) => element.scrollHeight - element.clientHeight), {
        timeout: 5_000,
      })
      .toBeGreaterThan(0);
    const scrollMetrics = await scrollRegion.evaluate((element) => ({
      scrollHeight: element.scrollHeight,
      clientHeight: element.clientHeight,
      overflowY: getComputedStyle(element).overflowY,
    }));
    expect(scrollMetrics.overflowY).toBe("auto");
    expect(scrollMetrics.scrollHeight).toBeGreaterThan(scrollMetrics.clientHeight);
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

    await expect
      .poll(() => findMaterializedAttachment(backend.tmpDir, attachmentName) !== null, {
        timeout: 30_000,
        message: "Wait for the opening attachment to be materialized into the session",
      })
      .toBe(true);
    const materialized = findMaterializedAttachment(backend.tmpDir, attachmentName);
    if (materialized === null) throw new Error("Materialized opening attachment disappeared");
    expect(fs.readFileSync(materialized, "utf8")).toBe(
      "Follow the native launcher into the task session.",
    );
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
