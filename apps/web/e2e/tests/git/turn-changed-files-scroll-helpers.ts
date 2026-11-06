import type { Page } from "@playwright/test";
import { test, expect, type SeedData } from "../../fixtures/test-base";
import type { PrAssetCapture } from "../../helpers/pr-asset-capture";
import type { ApiClient } from "../../helpers/api-client";
import { SessionPage } from "../../pages/session-page";
import { waitForSessionDone } from "../../helpers/session";
import { closePreviewPanels } from "./diff-update-helpers";
import { resetTurnQaFile } from "./turn-changed-files-qa-helpers";
import { assertLatestTurnCardInView, waitForTurnChangeCount } from "./turn-changed-files-helpers";

export async function exerciseDelayedTurnCardScroll(
  page: Page,
  api: ApiClient,
  seed: SeedData,
  prCapture: PrAssetCapture,
) {
  resetTurnQaFile(seed.repositoryPath);
  const task = await api.createTaskWithAgent(
    seed.workspaceId,
    "Delayed changed files",
    seed.agentProfileId,
    {
      description: Array.from(
        { length: 24 },
        (_, index) =>
          `e2e:message("History paragraph ${index}: content before the file-changing turn.")`,
      ).join("\n"),
      workflow_id: seed.workflowId,
      workflow_step_id: seed.startStepId,
      repository_ids: [seed.repositoryId],
    },
  );
  const sessionId = task.session_id!;
  await waitForSessionDone(api, task.id, sessionId, "history seed should finish");
  await page.goto(`/t/${task.id}`);
  const session = new SessionPage(page);
  await session.waitForLoad();
  await closePreviewPanels(page);
  const list = session.activeChat().locator(".chat-message-list");
  let held = false;
  let release = () => {};
  let gate: Promise<void>;
  const hold = () => {
    held = false;
    gate = new Promise<void>((resolve) => {
      release = resolve;
    });
  };
  hold();
  await page.route("**/turn-changes/*/repositories/*/files?*", async (route) => {
    const response = await route.fetch();
    held = true;
    await gate;
    await route.fulfill({ response });
  });
  const bottomGap = () => list.evaluate((el) => el.scrollHeight - el.scrollTop - el.clientHeight);
  try {
    await session.sendMessageViaButton("/e2e:untracked-file-setup");
    await waitForTurnChangeCount(api, sessionId, 2);
    await session.waitForChatIdle();
    await expect.poll(() => held).toBe(true);
    await expect.poll(bottomGap).toBeLessThanOrEqual(3);
    release();
    await assertLatestTurnCardInView(session);
    await expect.poll(bottomGap).toBeLessThanOrEqual(3);

    hold();
    await page.reload();
    await session.waitForLoad();
    await expect.poll(() => held).toBe(true);
    await expect.poll(bottomGap).toBeLessThanOrEqual(3);
    release();
    await assertLatestTurnCardInView(session);
    await expect.poll(bottomGap).toBeLessThanOrEqual(3);
    await page.screenshot({ path: test.info().outputPath("turn-card-follow.png") });
    if (prCapture.capturing) {
      const viewport = test.info().project.name === "mobile-chrome" ? "mobile" : "desktop";
      await prCapture.screenshot(`turn-card-follow-${viewport}`, {
        caption: "Latest changed-files card fully visible after delayed loading and reload",
      });
    }
  } finally {
    release();
    await page.unrouteAll({ behavior: "wait" });
    resetTurnQaFile(seed.repositoryPath);
  }
}
