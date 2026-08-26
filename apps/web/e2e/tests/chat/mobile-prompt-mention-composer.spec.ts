import { type Page } from "@playwright/test";
import { test, expect, type SeedData } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";
import { SessionPage } from "../../pages/session-page";

const PROMPT_NAME = "mobile-prompt-mention-viewport";
const PROMPT_CONTENT = "Mobile custom prompt content";

async function openReadyTask(page: Page, apiClient: ApiClient, seedData: SeedData) {
  const task = await apiClient.createTaskWithAgent(
    seedData.workspaceId,
    "Mobile Prompt Mention Viewport",
    seedData.agentProfileId,
    {
      description: "/e2e:simple-message",
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    },
  );
  await page.goto(`/t/${task.id}`);
  const session = new SessionPage(page);
  await session.waitForLoad();
  await session.waitForChatIdle({ timeout: 30_000 });
  return session;
}

// @covers AC-UI-COMPOSER-OVERLAY-001.1
// @covers AC-UI-COMPOSER-OVERLAY-001.3
// @covers AC-UI-COMPOSER-OVERLAY-001.4
// @covers AC-UI-COMPOSER-OVERLAY-001.5
test("keeps custom prompts directly above a keyboard-resized mobile composer", async ({
  testPage,
  apiClient,
  seedData,
  prCapture,
}) => {
  const prompt = await apiClient.createPrompt(PROMPT_NAME, PROMPT_CONTENT);
  try {
    const session = await openReadyTask(testPage, apiClient, seedData);
    const editor = await session.composerReady();
    await editor.tap();
    await editor.fill("");
    const viewport = testPage.viewportSize();
    expect(viewport).not.toBeNull();
    // Playwright cannot open an OS keyboard. Resize the page itself so the
    // mobile layout and focused composer reflow into the reduced visible area.
    // Overriding visualViewport alone would describe the composer as occluded.
    await testPage.setViewportSize({ width: viewport!.width, height: 420 });
    await editor.pressSequentially("@mobile-prompt");

    const menu = testPage.getByRole("listbox", { name: /Mention tasks, files, prompts/i });
    const option = menu.getByRole("option").filter({ hasText: PROMPT_NAME });
    await expect(menu).toBeVisible();
    await expect(option).toBeVisible();

    const [menuBox, composerBox] = await Promise.all([
      menu.locator("..").boundingBox(),
      session.activeChat().getByTestId("chat-input-editor-shell").boundingBox(),
    ]);
    expect(menuBox).not.toBeNull();
    expect(composerBox).not.toBeNull();
    expect(Math.abs(menuBox!.y + menuBox!.height - composerBox!.y)).toBeLessThanOrEqual(8);

    const geometry = await menu.evaluate((listbox) => {
      const surface = listbox.parentElement;
      const rect = surface?.getBoundingClientRect();
      return {
        top: rect?.top ?? -1,
        bottom: rect?.bottom ?? -1,
        viewportTop: window.visualViewport?.offsetTop ?? 0,
        viewportBottom: window.visualViewport
          ? window.visualViewport.offsetTop + window.visualViewport.height
          : window.innerHeight,
        rowHeight: listbox.querySelector<HTMLElement>("[role='option']")?.getBoundingClientRect()
          .height,
        hasHorizontalOverflow:
          document.documentElement.scrollWidth > document.documentElement.clientWidth,
      };
    });
    expect(geometry.top).toBeGreaterThanOrEqual(geometry.viewportTop);
    expect(geometry.bottom).toBeLessThanOrEqual(geometry.viewportBottom + 1);
    expect(geometry.rowHeight).toBeGreaterThanOrEqual(44);
    expect(geometry.hasHorizontalOverflow).toBe(false);

    await prCapture.screenshot("mobile-composer-prompt-menu", {
      caption:
        "Saved-prompt suggestions stay attached directly above the keyboard-resized composer",
    });

    await option.tap();
    await expect(menu).toHaveCount(0);
    await expect(editor).toContainText(PROMPT_NAME);
    await expect(editor).toBeFocused();
  } finally {
    await apiClient.deletePrompt(prompt.id).catch(() => undefined);
  }
});
