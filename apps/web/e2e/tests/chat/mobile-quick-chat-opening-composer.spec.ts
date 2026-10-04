import { test, expect } from "../../fixtures/test-base";
import { openMobileQuickChatSetup } from "./quick-chat-saved-prompt-delivery-helpers";
import {
  completeQuickChatOpening,
  expectContextRowSpacing,
  expectMixedContextRow,
  expectNoHorizontalOverflow,
  expectTouchTarget,
} from "./quick-chat-opening-composer-helpers";

test("opens the phone picker and delivers the opening prompt", async ({
  testPage,
  apiClient,
  prCapture,
}, testInfo) => {
  test.setTimeout(120_000);
  await testPage.setViewportSize({ width: 390, height: 844 });
  const { agents } = await apiClient.listAgents();
  const mockAgent = agents.find((agent) => agent.name === "mock-agent");
  if (!mockAgent) throw new Error("mock-agent is required for the phone profile picker test");

  const temporaryProfiles = await Promise.all(
    Array.from({ length: 8 }, (_, index) =>
      apiClient.createAgentProfile(
        mockAgent.id,
        `Quick Chat Picker ${testInfo.workerIndex} ${Date.now()} ${index}`,
        { model: "mock-fast" },
      ),
    ),
  );
  try {
    const dialog = await openMobileQuickChatSetup(testPage);
    await expect
      .poll(async () => {
        const box = await dialog.boundingBox();
        return box !== null && box.x <= 1 && box.y <= 1;
      })
      .toBe(true);
    const dialogBox = await dialog.boundingBox();
    expect(dialogBox).not.toBeNull();
    expect(dialogBox?.x ?? -1).toBeLessThanOrEqual(1);
    expect(dialogBox?.y ?? -1).toBeLessThanOrEqual(1);
    expect(Math.abs((dialogBox?.width ?? 0) - 390)).toBeLessThanOrEqual(2);
    expect(Math.abs((dialogBox?.height ?? 0) - 844)).toBeLessThanOrEqual(2);

    const editor = dialog.getByTestId("task-description-input");
    const send = dialog.getByTestId("quick-chat-send");
    const attach = dialog.getByRole("button", { name: "Attach files" });
    const setupScroll = dialog.getByTestId("quick-chat-setup-scroll");
    await expectTouchTarget(attach);
    await expectTouchTarget(send);
    const padding = await send.locator("xpath=../..").evaluate((element) => {
      const style = getComputedStyle(element);
      return { top: style.paddingTop, bottom: style.paddingBottom };
    });
    expect(padding.top).toBe(padding.bottom);
    expect(parseFloat(padding.top)).toBeGreaterThan(0);
    await expect(editor).toBeVisible();

    await testPage.setViewportSize({ width: 390, height: 360 });
    await expect
      .poll(() => setupScroll.evaluate((element) => element.scrollHeight - element.clientHeight), {
        timeout: 5_000,
      })
      .toBeGreaterThan(0);
    const shortViewportMetrics = await setupScroll.evaluate((element) => ({
      scrollHeight: element.scrollHeight,
      clientHeight: element.clientHeight,
    }));
    expect(shortViewportMetrics.scrollHeight).toBeGreaterThan(shortViewportMetrics.clientHeight);
    const shortSendBox = await send.boundingBox();
    expect(shortSendBox).not.toBeNull();
    expect(shortSendBox!.y + shortSendBox!.height).toBeLessThanOrEqual(360);
    await expect(send).toBeVisible();
    await expectNoHorizontalOverflow(testPage);

    await testPage.setViewportSize({ width: 390, height: 844 });
    await testInfo.attach("quick-chat-opening-phone", {
      body: await testPage.screenshot(),
      contentType: "image/png",
    });

    const agentTrigger = dialog.getByTestId("agent-profile-selector");
    await expectTouchTarget(agentTrigger);
    const addRepository = dialog.getByTestId("add-repository");
    await expectTouchTarget(addRepository);
    await addRepository.tap();
    const repoPicker = testPage.getByTestId("quick-chat-repository-picker");
    await expect(repoPicker).toBeVisible();
    await expect(dialog.getByTestId("repo-chip")).toHaveCount(0);
    await repoPicker.getByRole("option").first().tap();
    await expect(repoPicker).toBeHidden();
    await expect(attach).toBeFocused();
    await expect(
      dialog.getByTestId("quick-chat-repository-chips").getByTestId("repo-chip"),
    ).toHaveCount(1);
    const branch = dialog.getByTestId("branch-chip-trigger");
    await expectTouchTarget(branch);
    await expect(branch).toContainText("main");
    await expectContextRowSpacing(dialog, dialog.getByTestId("repo-chip"));
    await expectMixedContextRow(dialog, false);
    await expectNoHorizontalOverflow(testPage);
    await prCapture.screenshot("toolbar-phone", {
      caption: "Phone Quick Chat with repositories and images in one wrapping context row",
    });
    await dialog.getByTestId("context-chip-remove").nth(1).click();
    await dialog.getByTestId("context-chip-remove").click();
    await branch.tap();
    const branchPicker = testPage.getByTestId("quick-chat-branch-picker");
    await expect(branchPicker).toBeVisible();
    await branchPicker.getByRole("option").first().tap();
    await expect(branchPicker).toBeHidden();
    await expect(branch).toBeFocused();
    await expectTouchTarget(dialog.getByTestId("remove-repo-chip"));

    const configurationAction = dialog.getByTestId("quick-chat-configuration-action");
    await expectTouchTarget(configurationAction);
    await configurationAction.tap();
    await expect(testPage.getByRole("heading", { name: "Configuration chat" })).toBeVisible();
    await expect(
      testPage
        .getByText(
          "Let the agent update Kandev settings, workflows, agent profiles, and MCP configuration.",
          { exact: true },
        )
        .last(),
    ).toBeVisible();
    await testPage.getByRole("switch", { name: "Configuration chat" }).tap();
    await expect(configurationAction).toHaveAttribute("data-variant", "default");
    await expect(addRepository).toBeDisabled();
    const configurationLabel = dialog.getByTestId("quick-chat-setup").getByRole("status");
    await expect(configurationLabel).toHaveText("Configuration chat");
    await expectContextRowSpacing(dialog, configurationLabel);
    await configurationAction.tap();
    await testPage.getByRole("switch", { name: "Configuration chat" }).tap();
    await expect(dialog.getByTestId("repo-chip")).toHaveCount(1);
    await dialog.getByTestId("remove-repo-chip").tap();
    await expect(addRepository).toBeEnabled();

    await agentTrigger.tap();
    const picker = testPage.getByTestId("quick-chat-agent-picker-content");
    await expect(picker).toBeVisible();
    const list = picker.locator("[cmdk-list]");
    await expect(list.getByRole("option")).toHaveCount(9, { timeout: 10_000 });
    const listMetrics = await list.evaluate((element) => ({
      scrollHeight: element.scrollHeight,
      clientHeight: element.clientHeight,
    }));
    expect(listMetrics.scrollHeight).toBeGreaterThan(listMetrics.clientHeight);
    await list.evaluate((element) => {
      element.scrollTop = element.scrollHeight;
    });
    await expect.poll(() => list.evaluate((element) => element.scrollTop)).toBeGreaterThan(0);
    await testPage.keyboard.press("Escape");
    await expect(picker).toBeHidden();
    await expect(agentTrigger).toBeFocused();
    await agentTrigger.tap();
    await picker.getByRole("option").last().tap();
    await expect(agentTrigger).not.toContainText("Select agent");

    const prompt = "Review this feature from my phone";
    await completeQuickChatOpening(testPage, dialog, apiClient, prompt, {
      submit: (button) => button.tap(),
    });
    await expectNoHorizontalOverflow(testPage);
  } finally {
    await Promise.all(
      temporaryProfiles.map((profile) =>
        apiClient.deleteAgentProfile(profile.id, true).catch(() => undefined),
      ),
    );
  }
});
