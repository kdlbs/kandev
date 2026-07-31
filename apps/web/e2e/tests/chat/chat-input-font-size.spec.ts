import { type Page } from "@playwright/test";
import { test, expect, type SeedData } from "../../fixtures/test-base";
import { SessionPage } from "../../pages/session-page";
import type { ApiClient } from "../../helpers/api-client";
import type { CreateTaskResponse } from "../../../lib/types/http";

/** Widths on both sides of Tailwind's `lg` (1024px) breakpoint. The composer
 *  used to carry `text-base … lg:text-sm`, so dragging a desktop window under
 *  1024px grew the prompt text from 14px to 16px. */
const WIDTHS = [1440, 1100, 900, 700] as const;

async function createReadyTask(
  apiClient: ApiClient,
  seedData: SeedData,
): Promise<CreateTaskResponse> {
  return apiClient.createTaskWithAgent(
    seedData.workspaceId,
    "Chat Input Font Size",
    seedData.agentProfileId,
    {
      description: "/e2e:simple-message",
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    },
  );
}

async function openTaskChat(page: Page, taskId: string): Promise<SessionPage> {
  await page.goto(`/t/${taskId}`);
  const session = new SessionPage(page);
  await session.waitForLoad();
  await session.waitForChatIdle({ timeout: 30_000 });
  return session;
}

test.describe("Chat input font size", () => {
  test("stays constant while the desktop window is resized", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const task = await createReadyTask(apiClient, seedData);
    await openTaskChat(testPage, task.id);

    // Multiple TipTap instances can be mounted in dockview layouts; scope to
    // the first visible one.
    const editor = testPage.locator(".tiptap.ProseMirror:visible").first();
    await expect(editor).toBeVisible();

    // Desktop Chrome exposes a fine pointer, so the globals.css 16px
    // coarse-pointer floor must not apply here. Assert it, otherwise this test
    // would pass trivially with every width pinned to 16px.
    const coarse = await testPage.evaluate(() => matchMedia("(any-pointer: coarse)").matches);
    expect(coarse).toBe(false);

    const sizes: number[] = [];
    for (const width of WIDTHS) {
      await testPage.setViewportSize({ width, height: 900 });
      // Narrow widths swap the dockview layout, which remounts the composer.
      // Re-await visibility so the measurement never lands on a detached node
      // mid-remount (getComputedStyle on a detached element yields "" → NaN).
      await expect(editor).toBeVisible();
      await expect
        .poll(async () => editor.evaluate((el) => getComputedStyle(el).fontSize))
        .not.toBe("");
      sizes.push(await editor.evaluate((el) => parseFloat(getComputedStyle(el).fontSize)));
    }

    expect(sizes).toEqual(sizes.map(() => sizes[0]));
    expect(sizes[0]).toBeCloseTo(14, 1);
  });
});
