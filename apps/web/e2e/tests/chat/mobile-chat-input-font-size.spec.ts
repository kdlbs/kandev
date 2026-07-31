import { type Page } from "@playwright/test";
import { test, expect, type SeedData } from "../../fixtures/test-base";
import { SessionPage } from "../../pages/session-page";
import type { ApiClient } from "../../helpers/api-client";
import type { CreateTaskResponse } from "../../../lib/types/http";

// Runs on the mobile-chrome Playwright project (Pixel 5 emulation, touch →
// any-pointer: coarse). The composer's Tailwind class is a flat `text-sm`, so
// touch devices depend entirely on the 16px coarse-pointer rule in globals.css
// to stop iOS Safari auto-zooming on focus. This guards that dependency —
// mobile-zoom.spec.ts covers plain form controls, this covers the
// contenteditable TipTap editor.
async function createReadyTask(
  apiClient: ApiClient,
  seedData: SeedData,
): Promise<CreateTaskResponse> {
  return apiClient.createTaskWithAgent(
    seedData.workspaceId,
    "Mobile Chat Input Font Size",
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

test.describe("Mobile chat input font size", () => {
  test("composer renders at >= 16px to prevent iOS focus-zoom", async ({
    testPage,
    apiClient,
    seedData,
    prCapture,
  }) => {
    const task = await createReadyTask(apiClient, seedData);
    await openTaskChat(testPage, task.id);

    // Guard: the 16px rule is gated on `@media (any-pointer: coarse)`. Assert
    // the emulated device really exposes a coarse pointer so a fixture change
    // that drops touch emulation fails loudly instead of silently passing.
    const coarse = await testPage.evaluate(() => matchMedia("(any-pointer: coarse)").matches);
    expect(coarse).toBe(true);

    const editor = testPage.locator(".tiptap.ProseMirror:visible").first();
    await expect(editor).toBeVisible();
    await editor.tap();

    const fontSize = await editor.evaluate((el) => parseFloat(getComputedStyle(el).fontSize));
    expect(fontSize).toBeGreaterThanOrEqual(16);

    await editor.pressSequentially("The quick brown fox jumps over the lazy dog");
    await prCapture.screenshot("mobile-composer", {
      caption: "Touch composer keeps the 16px iOS focus-zoom floor from the coarse-pointer rule",
    });
  });
});
