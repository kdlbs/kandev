// AC-COORDINATOR-COPILOT-004.3, -004.4: full width below 640px
// (docs/plans/workspace-coordinator/task-06-copilot-wired.md).
import { test, expect } from "../../fixtures/test-base";
import { waitForHttp } from "../../helpers/causal-waits";
import { linkToCoordinatorNeedsYou } from "../../../lib/coordinator/links";

test.describe("Mobile coordinator copilot", () => {
  test("opens full width below 640px and answers a question", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(60_000);
    await testPage.setViewportSize({ width: 390, height: 667 });

    const coordinator = await apiClient.createCoordinator(seedData.workspaceId, {
      name: "Mobile Copilot Coordinator",
      agent_profile_id: seedData.agentProfileId,
      executor_profile_id: seedData.worktreeExecutorProfileId,
    });

    await testPage.goto(linkToCoordinatorNeedsYou(seedData.workspaceId, coordinator.id));

    const coordinatorRead = waitForHttp(testPage, "GET", /\/coordinators\/[^/]+$/);
    const conversationOpened = waitForHttp(
      testPage,
      "POST",
      /\/coordinators\/[^/]+\/conversation$/,
    );
    const launcher = testPage.getByTestId("coordinator-copilot-launcher");
    await expect(launcher).toBeVisible({ timeout: 10_000 });
    await launcher.tap();
    await coordinatorRead;
    await conversationOpened;

    const popover = testPage.getByTestId("coordinator-copilot-popover");
    await expect(popover).toBeVisible();

    const box = await popover.boundingBox();
    expect(box).not.toBeNull();
    expect(box!.x).toBeGreaterThanOrEqual(0);
    expect(box!.x + box!.width).toBeLessThanOrEqual(390);
    // Full width below 640px: the shell's `min(420px, 100vw - 2rem)` cap
    // resolves to the viewport-minus-margin branch here, not the 420px
    // desktop cap.
    expect(box!.width).toBeGreaterThan(390 - 40);
    expect(box!.width).toBeLessThan(420);
    expect(
      await testPage.evaluate(() => document.documentElement.scrollWidth > window.innerWidth),
    ).toBe(false);

    const editor = popover.getByTestId("chat-input-editor");
    await expect(editor).toHaveAttribute("contenteditable", "true", { timeout: 15_000 });
    await editor.pressSequentially("/e2e:simple-message", { timeout: 15_000 });
    await expect(editor).toContainText("/e2e:simple-message");
    // Coarse-pointer devices submit via the inline Send button, not the
    // desktop Cmd/Ctrl+Enter shortcut (see mobile-clarification.spec.ts).
    await popover.getByTestId("submit-message-button").tap();

    await expect(
      popover.getByText("simple mock response for e2e testing", { exact: false }),
    ).toBeVisible({ timeout: 30_000 });

    await testPage.keyboard.press("Escape");
    await expect(popover).not.toBeVisible();
  });
});
