import { test, expect } from "../../fixtures/test-base";
import { PrAssetCapture } from "../../helpers/pr-asset-capture";
import {
  openConfigurationChat,
  confirmConfigurationChatRestart,
  sendConfigurationPrompt,
  waitForConfigurationResponse,
  openUncertainExpandedConfigChat,
} from "../../helpers/config-chat-restart";

test.describe("Mobile Configuration Chat restart", () => {
  test("refreshes an uncertain restart from expanded chat with touch", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const capture = new PrAssetCapture(testPage, "mobile-config-chat-restart.spec.ts", {
      captureKey: "expanded",
    });
    await testPage.setViewportSize({ width: 320, height: 844 });
    await apiClient.updateWorkspace(seedData.workspaceId, {
      default_config_agent_profile_id: seedData.agentProfileId,
    });
    const old = await apiClient.startConfigChat(
      seedData.workspaceId,
      seedData.agentProfileId,
      'e2e:message("phone expanded original")',
    );
    await waitForConfigurationResponse(apiClient, old.session_id, "phone expanded original");
    const dialog = await openUncertainExpandedConfigChat(
      testPage,
      seedData.workspaceId,
      old.session_id,
      true,
    );
    const refresh = dialog.getByRole("button", { name: "Refresh status", exact: true });
    await expect(refresh).toHaveCount(1);
    expect((await refresh.boundingBox())!.height).toBeGreaterThanOrEqual(44);
    await capture.screenshot("phone-expanded-restart-recovery", {
      caption: "Phone expanded Configuration Chat offers a touch-sized status refresh",
    });
    capture.flush();
    expect(
      await refresh.evaluate((element) => {
        const rect = element.getBoundingClientRect();
        return element.contains(
          document.elementFromPoint(rect.x + rect.width / 2, rect.y + rect.height / 2),
        );
      }),
    ).toBe(true);
    await refresh.tap();
    await expect(dialog.getByText("phone expanded original", { exact: true })).toBeVisible();
    await expect(refresh).toHaveCount(0);
    const viewport = testPage.viewportSize()!;
    expect(await testPage.evaluate(() => document.documentElement.scrollWidth)).toBeLessThanOrEqual(
      viewport.width,
    );
  });
  test("uses touch-sized header controls and an inset confirmation drawer to recover", async ({
    testPage,
    apiClient,
    seedData,
    prCapture,
  }) => {
    await apiClient.updateWorkspace(seedData.workspaceId, {
      default_config_agent_profile_id: seedData.agentProfileId,
    });
    const old = await apiClient.startConfigChat(
      seedData.workspaceId,
      seedData.agentProfileId,
      'e2e:message("phone old response")',
    );
    const panel = await openConfigurationChat(testPage, true);
    await waitForConfigurationResponse(apiClient, old.session_id, "phone old response");
    await expect(panel.getByText("phone old response", { exact: true })).toBeVisible();
    const restart = panel.getByRole("button", { name: "Restart session", exact: true });
    for (const label of ["Restart session", "Open in Quick Chat", "Close configuration chat"]) {
      const control = panel.getByRole("button", { name: label, exact: true });
      await expect
        .poll(async () => (await control.boundingBox())?.height ?? 0)
        .toBeGreaterThanOrEqual(44);
      expect((await control.boundingBox())!.width).toBeGreaterThanOrEqual(44);
    }
    await restart.tap();
    const confirmation = testPage.getByTestId("config-chat-restart-confirmation");
    await expect(confirmation).toBeVisible();
    await expect(confirmation.locator("xpath=ancestor::*[@role='dialog']")).toBeVisible();
    await expect(confirmation.getByRole("button", { name: "Cancel", exact: true })).toBeFocused();
    const viewport = testPage.viewportSize()!;
    await expect
      .poll(async () => {
        const bounds = await confirmation.boundingBox();
        return bounds ? bounds.y + bounds.height : Infinity;
      })
      .toBeLessThanOrEqual(viewport.height + 1);
    const box = (await confirmation.boundingBox())!;
    expect(box.x).toBeGreaterThanOrEqual(0);
    expect(box.x + box.width).toBeLessThanOrEqual(viewport.width + 1);
    expect(box.y + box.height).toBeLessThanOrEqual(viewport.height + 1);
    expect(
      (await confirmation.getByTestId("config-chat-confirm-restart").boundingBox())!.height,
    ).toBeGreaterThanOrEqual(44);
    await prCapture.screenshot("phone-restart-confirmation", {
      caption: "Phone Configuration Chat restart uses the shared bottom confirmation drawer",
    });
    await confirmation.getByRole("button", { name: "Cancel", exact: true }).tap();
    await expect(restart).toBeFocused();
    await expect(panel.getByText("phone old response", { exact: true })).toBeVisible();
    const replacement = await confirmConfigurationChatRestart(testPage, panel, true);
    expect(replacement.session_id).not.toBe(old.session_id);
    await expect(panel.getByText("phone old response", { exact: true })).toHaveCount(0);
    await expect
      .poll(async () => (await apiClient.getTaskSession(replacement.session_id)).session.state)
      .toBe("WAITING_FOR_INPUT");
    const { turns } = await apiClient.listSessionTurns(replacement.session_id);
    expect(turns.filter((turn) => turn.metadata?.lifecycle_only !== true)).toEqual([]);
    expect(turns.every((turn) => Boolean(turn.completed_at))).toBe(true);
    const { messages } = await apiClient.listSessionMessages(replacement.session_id);
    expect(
      messages.filter(
        (message) =>
          ["message", "content", "thinking"].includes(message.type ?? "") ||
          message.type?.startsWith("tool_"),
      ),
    ).toEqual([]);
    await sendConfigurationPrompt(
      panel,
      'e2e:message("phone recovered response")',
      apiClient,
      replacement.session_id,
      true,
    );
    await waitForConfigurationResponse(
      apiClient,
      replacement.session_id,
      "phone recovered response",
    );
    await expect(panel.getByText("phone recovered response", { exact: true })).toBeVisible();
    expect(
      await testPage.evaluate(() => document.documentElement.scrollWidth > window.innerWidth),
    ).toBe(false);
    await prCapture.screenshot("phone-recovered-chat", {
      caption: "A fresh Configuration Chat session accepts prompts after restart on a phone",
    });
  });
});
