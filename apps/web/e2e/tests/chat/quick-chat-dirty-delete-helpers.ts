import fs from "node:fs";
import path from "node:path";
import type { Page } from "@playwright/test";
import { expect } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";
import type { PrAssetCapture } from "../../helpers/pr-asset-capture";
import { waitForTaskWorktree } from "../../helpers/empty-remote-repository";
import { waitForDeletePreviewTask } from "../../helpers/task-delete-recovery";
import { assertNoDocumentHorizontalOverflow } from "../../helpers/layout-assertions";
import { openQuickChatSetup, startQuickChatFromSetup } from "./quick-chat-helpers";

export async function assertDirtyQuickChatDeletion(
  page: Page,
  api: ApiClient,
  repositoryId: string,
  mobile: boolean,
  capture: PrAssetCapture,
) {
  const chat = await openQuickChatSetup(page);
  await chat.getByTestId("add-repository").click();
  await page.locator(`[role="option"][data-value="${repositoryId}"]`).click();
  const started = await startQuickChatFromSetup(chat, page);
  await waitForDeletePreviewTask(api, started.task_id);
  const checkout = await waitForTaskWorktree(api, started.task_id, repositoryId);
  const marker = path.join(checkout, "quick-chat-delete-untracked.txt");
  fs.writeFileSync(marker, "local work\n");
  const tab = chat.getByTestId("quick-chat-tab").last();
  async function openDelete() {
    if (mobile) {
      await tab.getByRole("button", { name: /^Actions for / }).click();
      await page.getByRole("menuitem", { name: /^Close / }).click();
    } else {
      await tab.getByRole("button", { name: /^Close / }).click();
    }
    const confirmation = page.getByRole("alertdialog", { name: "Delete Quick Chat?" });
    await expect(confirmation.getByTestId("delete-discard-worktree-checkbox")).toBeVisible();
    return confirmation;
  }
  try {
    let confirmation = await openDelete();
    const consent = confirmation.getByTestId("delete-discard-worktree-checkbox");
    await expect(consent).not.toBeChecked();
    await expect(confirmation.getByRole("button", { name: "Delete", exact: true })).toBeDisabled();
    await confirmation.evaluate(async (element) => {
      await Promise.all(
        element
          .getAnimations({ subtree: true })
          .map((animation) => animation.finished.catch(() => undefined)),
      );
    });
    if (capture.capturing) {
      await capture.screenshot(mobile ? "dirty-delete-mobile" : "dirty-delete-desktop");
    }
    if (mobile) {
      expect(
        (await confirmation.getByRole("button", { name: "Cancel" }).boundingBox())?.height,
      ).toBeGreaterThanOrEqual(44);
      await assertNoDocumentHorizontalOverflow(page);
    }
    await confirmation.getByRole("button", { name: "Cancel" }).click();
    await expect(tab).toBeVisible();
    expect(fs.readFileSync(marker, "utf8")).toBe("local work\n");
    confirmation = await openDelete();
    await confirmation.getByTestId("delete-discard-worktree-checkbox").click();
    const action = confirmation.getByRole("button", { name: "Delete", exact: true });
    await expect(action).toBeEnabled();
    const deletion = page.waitForResponse(
      (response) =>
        response.request().method() === "DELETE" &&
        new URL(response.url()).pathname === `/api/v1/tasks/${started.task_id}`,
    );
    await action.click();
    expect((await deletion).ok()).toBe(true);
    await expect(tab).toHaveCount(0);
    await expect.poll(() => fs.existsSync(checkout), { timeout: 60_000 }).toBe(false);
  } finally {
    if (fs.existsSync(marker)) fs.unlinkSync(marker);
  }
}
