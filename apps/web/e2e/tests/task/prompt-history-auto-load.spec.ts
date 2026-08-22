import { expect, type Route } from "@playwright/test";
import { test as kandevTest } from "../../fixtures/test-base";
import { SessionPage } from "../../pages/session-page";
import {
  FIRST_PROMPT_MARKER,
  SECOND_PROMPT_MARKER,
  seedLongPromptHistory,
} from "../../helpers/prompt-history-long-seed";

kandevTest.describe("Prompt history auto-load", () => {
  kandevTest(
    "auto-loads older pages until #1 with no button",
    async ({ testPage, apiClient, seedData }) => {
      kandevTest.setTimeout(180_000);

      const taskId = await seedLongPromptHistory(apiClient, {
        workspaceId: seedData.workspaceId,
        agentProfileId: seedData.agentProfileId,
        workflowId: seedData.workflowId,
        startStepId: seedData.startStepId,
        repositoryId: seedData.repositoryId,
      });

      // Hold every older-page request until the test scrolls the panel and
      // observes the loading state; releases continue immediately afterwards.
      let panelScrollTriggered = false;
      const heldOlderRequests: Route[] = [];
      let releaseHeld: (() => void) | null = null;
      const olderRequestGate = new Promise<void>((resolve) => {
        releaseHeld = resolve;
      });
      await testPage.route("**/api/v1/task-sessions/*/messages*", async (route) => {
        const url = route.request().url();
        if (url.includes("before=") && !panelScrollTriggered) {
          heldOlderRequests.push(route);
          await olderRequestGate;
        }
        await route.continue();
      });

      await testPage.goto(`/t/${taskId}`);
      const session = new SessionPage(testPage);
      await session.waitForLoad();
      await session.waitForDockviewReady();

      // Open the panel: the newest page is loaded (row-0 = #121), and the
      // #2 marker is NOT rendered before scrolling.
      await session.addPanelButton().click();
      await testPage.getByTestId("add-panel-prompt-history-item").click();
      const panel = testPage.getByTestId("prompt-history-panel");
      await expect(panel).toBeVisible();
      await expect(panel.getByTestId("prompt-history-row-0")).toContainText(
        "auto-load filler prompt 119",
      );
      await expect(panel.getByTestId("prompt-history-number-0")).toHaveText("#121");
      await expect(panel.getByText(SECOND_PROMPT_MARKER)).toHaveCount(0);

      // Baseline the scroll layout BEFORE the sentinel fires: scrollTop below
      // triggers the held older-page request, and an in-flow loading row would
      // already have grown scrollHeight by the time the indicator is visible.
      const scroller = panel.getByTestId("prompt-history-scroll");
      const scrollHeightBefore = await scroller.evaluate((el) => el.scrollHeight);
      const scrollableBefore = await scroller.evaluate((el) => el.scrollHeight > el.clientHeight);
      // The seeded history (100 initial rows in a 900x900 viewport) must
      // overflow the panel; otherwise the floating assertions below would
      // silently skip instead of proving the no-reflow contract.
      expect(scrollableBefore).toBe(true);
      await scroller.evaluate((el) => {
        el.scrollTop = el.scrollHeight;
      });
      await expect(panel.getByTestId("prompt-history-loading-older")).toBeVisible({
        timeout: 10_000,
      });
      // The panel content scrolls, so the loading message floats at the panel
      // bottom, out of the content flow: its presence must not change the
      // scrollable height (the flicker regression - an in-flow row would grow
      // scrollHeight).
      expect(await scroller.evaluate((el) => el.scrollHeight)).toBe(scrollHeightBefore);
      await expect(panel.getByTestId("prompt-history-loading-older")).toHaveClass(/absolute/);
      await expect(panel.getByText(SECOND_PROMPT_MARKER)).toHaveCount(0);
      expect(heldOlderRequests.length).toBeGreaterThanOrEqual(1);

      // Release the held response(s) and let the coordinator merge.
      panelScrollTriggered = true;
      releaseHeld?.();

      // Bounded loop: scroll to the bottom repeatedly until the #1 description
      // row renders (do not assume a 20-message page — the first-request-wins
      // limit may return 100 rows).
      const firstRow = panel.locator('[data-testid^="prompt-history-row-"]', {
        hasText: FIRST_PROMPT_MARKER,
      });
      for (let attempt = 0; attempt < 10 && (await firstRow.count()) === 0; attempt++) {
        const rowsBefore = await panel.locator('[data-testid^="prompt-history-row-"]').count();
        await panel.evaluate((el) => {
          el.scrollTop = el.scrollHeight;
        });
        await expect
          .poll(async () => await panel.locator('[data-testid^="prompt-history-row-"]').count(), {
            timeout: 5_000,
          })
          .toBeGreaterThan(rowsBefore);
      }
      await expect(firstRow).toBeAttached({ timeout: 10_000 });
      await expect(firstRow.locator('[data-testid^="prompt-history-number-"]')).toHaveText("#1");

      // The #2 marker row is attached with its absolute ordinal.
      const secondRow = panel.locator('[data-testid^="prompt-history-row-"]', {
        hasText: SECOND_PROMPT_MARKER,
      });
      await expect(secondRow).toBeAttached();
      await expect(secondRow.locator('[data-testid^="prompt-history-number-"]')).toHaveText("#2");

      // No load-more button inside the panel: auto-load only.
      await expect(panel.getByTestId("load-older-messages")).toHaveCount(0);
    },
  );
});
