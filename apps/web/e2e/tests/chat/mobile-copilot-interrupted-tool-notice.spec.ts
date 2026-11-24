import { test, expect, type Page } from "../../fixtures/test-base";
import { SessionPage } from "../../pages/session-page";
import {
  cleanupCopilotInterruptionTest,
  expectCopilotInterruptionWarningFrame,
  expectCopilotNoticeBeforeReply,
  expectFreshCopilotSessionLoad,
  seedCopilotInterruptionTask,
  waitForCopilotInterruptionACPStartup,
} from "./copilot-interrupted-tool-notice-helpers";

async function summaryContrast(page: Page, statusSelector: string): Promise<number> {
  return page
    .locator(statusSelector)
    .locator("p")
    .first()
    .evaluate((element) => {
      const canvas = document.createElement("canvas");
      canvas.width = canvas.height = 1;
      const context = canvas.getContext("2d", { willReadFrequently: true });
      if (!context) throw new Error("Canvas color conversion is unavailable");
      const parse = (color: string): [number, number, number, number] => {
        context.clearRect(0, 0, 1, 1);
        context.fillStyle = color;
        context.fillRect(0, 0, 1, 1);
        const [red, green, blue, alpha] = context.getImageData(0, 0, 1, 1).data;
        return [red, green, blue, alpha / 255];
      };
      const blend = (
        foreground: [number, number, number, number],
        background: [number, number, number],
      ): [number, number, number] => [
        foreground[0] * foreground[3] + background[0] * (1 - foreground[3]),
        foreground[1] * foreground[3] + background[1] * (1 - foreground[3]),
        foreground[2] * foreground[3] + background[2] * (1 - foreground[3]),
      ];
      const ancestors: Element[] = [];
      let parent: Element | null = element;
      while (parent) {
        ancestors.push(parent);
        parent = parent.parentElement;
      }
      let background: [number, number, number] = [255, 255, 255];
      for (const ancestor of ancestors.reverse()) {
        background = blend(parse(getComputedStyle(ancestor).backgroundColor), background);
      }
      const foreground = parse(getComputedStyle(element).color);
      const linear = (value: number) => {
        const channel = value / 255;
        return channel <= 0.04045 ? channel / 12.92 : ((channel + 0.055) / 1.055) ** 2.4;
      };
      const luminance = (rgb: [number, number, number]) =>
        0.2126 * linear(rgb[0]) + 0.7152 * linear(rgb[1]) + 0.0722 * linear(rgb[2]);
      const textLuminance = luminance(blend(foreground, background));
      const backgroundLuminance = luminance(background);
      return (
        (Math.max(textLuminance, backgroundLuminance) + 0.05) /
        (Math.min(textLuminance, backgroundLuminance) + 0.05)
      );
    });
}

test.describe("Copilot interrupted tool notice on mobile", () => {
  test.setTimeout(150_000);

  test("skips by touch and keeps the status readable and contained after restart", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    const releaseEnvironment = await backend.useEnv({ KANDEV_E2E_MOCK_COPILOT: "true" });
    let disposeFixture: (() => Promise<void>) | undefined;
    try {
      const fixture = await seedCopilotInterruptionTask(testPage, apiClient, backend, seedData);
      disposeFixture = fixture.dispose;
      const initialACP = await waitForCopilotInterruptionACPStartup(fixture.tracePath);

      await backend.restart();
      await testPage.reload();
      const session = new SessionPage(testPage);
      await session.waitForLoad();
      await expectFreshCopilotSessionLoad(
        fixture.tracePath,
        initialACP.session_id,
        initialACP.process_id!,
      );
      await expect(session.clarificationOverlay()).toBeVisible({ timeout: 45_000 });

      await session.clarificationSkip().tap();
      await expect(session.clarificationOverlay()).toBeHidden();
      await session.waitForDirectInput();
      await session.sendMessageViaButton("Continue after skipping the saved question");
      await session.waitForChatIdle({ timeout: 45_000 });
      await expectCopilotInterruptionWarningFrame(fixture.tracePath, initialACP.session_id);

      const status = testPage.getByTestId("copilot-interruption-status");
      await expect(status).toBeVisible();
      await expect(status).toContainText("The pending question was interrupted.");
      const detailsButton = status.getByRole("button", { name: "Details" });
      await expect(detailsButton).toHaveAttribute("aria-expanded", "false");
      const target = await detailsButton.boundingBox();
      expect(target?.height).toBeGreaterThanOrEqual(44);
      await detailsButton.tap();
      await expect(status.getByTestId("copilot-interruption-details")).toContainText(
        "Warning: 1 tool call(s) had no recorded result at resume.",
      );
      await expect(session.chat).toContainText("Here is the next step.");

      const statusBox = await status.boundingBox();
      const chatBox = await session.chat.boundingBox();
      expect(statusBox).toBeTruthy();
      expect(chatBox).toBeTruthy();
      expect(statusBox!.x).toBeGreaterThanOrEqual(chatBox!.x - 1);
      expect(statusBox!.x + statusBox!.width).toBeLessThanOrEqual(chatBox!.x + chatBox!.width + 1);
      const detailsFits = await status
        .getByTestId("copilot-interruption-details")
        .evaluate((element) => element.scrollWidth <= element.clientWidth);
      expect(detailsFits).toBe(true);

      const root = testPage.locator("html");
      const originalClasses = await root.getAttribute("class");
      try {
        for (const theme of ["light", "dark"] as const) {
          await root.evaluate((element, className) => {
            element.classList.remove("light", "dark");
            element.classList.add(className);
          }, theme);
          await expect
            .poll(() =>
              root.evaluate((element, className) => element.classList.contains(className), theme),
            )
            .toBe(true);
          expect(
            await summaryContrast(testPage, '[data-testid="copilot-interruption-status"]'),
          ).toBeGreaterThanOrEqual(4.5);
        }
      } finally {
        await root.evaluate((element, className) => {
          element.className = className ?? "";
        }, originalClasses);
      }

      const { statusID, replyID } = await expectCopilotNoticeBeforeReply(
        apiClient,
        fixture.sessionId,
      );
      expect(statusID).not.toBe(replyID);
      await testPage.reload();
      await session.waitForLoad();
      await expect(testPage.getByTestId("copilot-interruption-status")).toHaveCount(1);
      await expect(
        testPage
          .getByTestId("copilot-interruption-status")
          .getByTestId("copilot-interruption-details"),
      ).toHaveCount(0);
      const restoredStatus = testPage.getByTestId("copilot-interruption-status");
      await expect(restoredStatus).toContainText("The pending question was interrupted.");
      await restoredStatus.getByRole("button", { name: "Details" }).tap();
      await expect(restoredStatus.getByTestId("copilot-interruption-details")).toContainText(
        "Warning: 1 tool call(s) had no recorded result at resume.",
      );
      await expectCopilotNoticeBeforeReply(apiClient, fixture.sessionId);
    } finally {
      await cleanupCopilotInterruptionTest(disposeFixture, releaseEnvironment);
    }
  });
});
