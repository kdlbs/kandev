import path from "node:path";
import { type Locator, type Page } from "@playwright/test";
import { test, expect, resetSeedRepositoryCheckout } from "../../fixtures/test-base";
import { GitHelper, makeGitEnv } from "../../helpers/git-helper";
import { SessionPage } from "../../pages/session-page";
import type { GitStatusData } from "../../../lib/types/git-events";
import { dwell, watchWs } from "../../helpers/causal-waits";
import { waitForFiniteAnimations } from "../../helpers/animations";

const TARGET = "a-review-viewed.ts";
const TRAILING = "z-review-viewed.ts";

async function scrollReview(page: Page, root: Locator, mobile: boolean, direction: 1 | -1) {
  const before = await root.evaluate((element) => element.scrollTop);
  const box = await root.boundingBox();
  if (!box) throw new Error("Review scroller is missing");
  if (mobile) {
    const header = await root.getByTestId("review-file-header").first().boundingBox();
    if (!header) throw new Error("Review file header is missing");
    const cdp = await page.context().newCDPSession(page);
    // Diff rows can be replaced during rendering. Keep the gesture on the persistent header.
    const x = header.x + header.width / 2;
    const startY = header.y + header.height / 2;
    const distance = direction === 1 ? Math.min(200, startY - 4) : 200;
    try {
      await cdp.send("Input.dispatchTouchEvent", {
        type: "touchStart",
        touchPoints: [{ x, y: startY }],
      });
      for (let step = 1; step <= 10; step++) {
        await cdp.send("Input.dispatchTouchEvent", {
          type: "touchMove",
          touchPoints: [{ x, y: startY - (direction * step * distance) / 10 }],
        });
        await page.evaluate(
          () => new Promise<void>((resolve) => requestAnimationFrame(() => resolve())),
        );
      }
      await cdp.send("Input.dispatchTouchEvent", { type: "touchEnd", touchPoints: [] });
    } finally {
      await cdp.detach();
    }
  } else {
    await page.mouse.move(box.x + box.width / 2, box.y + box.height / 2);
    await page.mouse.wheel(0, direction * 200);
  }
  await root.evaluate(async (element) => {
    let previous = element.scrollTop;
    let stable = 0;
    for (let frame = 0; frame < 120 && stable < 3; frame++) {
      await new Promise<void>((resolve) => requestAnimationFrame(() => resolve()));
      stable = element.scrollTop === previous ? stable + 1 : 0;
      previous = element.scrollTop;
    }
    if (stable < 3) throw new Error("Review scrolling did not settle");
  });
  if (direction === 1) {
    await expect.poll(() => root.evaluate((element) => element.scrollTop)).toBeGreaterThan(before);
  } else {
    await expect.poll(() => root.evaluate((element) => element.scrollTop)).toBeLessThan(before);
  }
}

export function reviewViewedTogglingScenario(mobile: boolean) {
  test.setTimeout(120_000);
  // @covers AC-UI-REVIEW-VIEWED-001.1, AC-UI-REVIEW-VIEWED-001.2, AC-UI-REVIEW-VIEWED-001.5
  test("keeps a file unchecked until scrolling back and passing it again", async ({
    testPage,
    apiClient,
    seedData,
    backend,
    prCapture,
  }) => {
    resetSeedRepositoryCheckout(seedData, backend.tmpDir);
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Review viewed checkbox",
      seedData.agentProfileId,
      {
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );
    const gateway = watchWs(testPage);
    await testPage.goto(`/t/${task.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await session.waitForChatIdle();
    const git = new GitHelper(
      path.join(backend.tmpDir, "repos", "e2e-repo"),
      makeGitEnv(backend.tmpDir),
    );
    const filesPublished = gateway.waitForEvent("session.git.event", {
      where: (payload) => {
        const status = payload.status as GitStatusData | undefined;
        return (
          payload.session_id === task.session_id &&
          payload.type === "status_update" &&
          Boolean(status?.files?.[TARGET] && status.files[TRAILING])
        );
      },
    });
    git.createFile(
      TARGET,
      Array.from({ length: 100 }, (_, index) => `export const line${index} = ${index};\n`).join(""),
    );
    git.createFile(TRAILING, "export const trailing = true;\n".repeat(80));
    git.stageFile(TARGET);
    git.stageFile(TRAILING);
    await filesPublished;
    if (mobile) {
      await testPage
        .getByRole("navigation")
        .getByRole("button", { name: /Changes$/ })
        .tap();
      const changes = testPage.getByTestId("mobile-changes-panel");
      await expect(changes.getByTestId(`file-row-${TARGET}`)).toBeVisible();
      await changes.getByRole("button", { name: "Review", exact: true }).tap();
    } else {
      await session.clickTab("Changes");
      await expect(session.changes.getByTestId(`file-row-${TARGET}`)).toBeVisible();
      await session.openChangesDiff();
      await testPage.getByRole("button", { name: "Expand review" }).click();
    }
    const dialog = testPage.getByRole("dialog", { name: "Review Changes" });
    const root = dialog.getByTestId("review-diff-scroll");
    const header = dialog.locator(`[data-testid="review-file-header"][data-file-path="${TARGET}"]`);
    const checkbox = mobile
      ? header.getByRole("checkbox")
      : dialog
          .getByTestId("review-dialog-sidebar")
          .locator(`[data-testid="review-file-row"][data-file-path="${TARGET}"]`)
          .getByRole("checkbox");
    await expect(header).toBeVisible();
    await expect(dialog.getByTestId("review-diff-pending")).toHaveCount(0);
    await expect
      .poll(() =>
        root
          .getByTestId("review-file-header")
          .evaluateAll((headers) =>
            headers.map((entry) => entry.getAttribute("data-file-path")).sort(),
          ),
      )
      .toEqual([TARGET, TRAILING]);
    await expect(dialog.getByText("export const line0 = 0;", { exact: true })).toBeVisible();
    await waitForFiniteAnimations(dialog);
    await expect(checkbox).not.toBeChecked();
    await scrollReview(testPage, root, mobile, 1);
    await expect(checkbox).toBeChecked();
    const position = await root.evaluate((element) => element.scrollTop);
    if (mobile) await checkbox.tap();
    else await checkbox.click();
    await expect(checkbox).not.toBeChecked();
    await expect.poll(() => root.evaluate((element) => element.scrollTop)).toBe(position);
    await scrollReview(testPage, root, mobile, 1);
    await dwell(
      testPage,
      250,
      "negative-assertion",
      "asserts that a manual uncheck survives observer delivery and later downward scrolling",
    );
    await expect(checkbox).not.toBeChecked();
    await expect(dialog.getByText(/^0 of \d+ files reviewed$/)).toBeVisible();
    await prCapture.screenshot(mobile ? "unchecked-mobile" : "unchecked-desktop", {
      caption: "A manually cleared review checkbox stays unchecked at the current reading position",
    });
    // Return the start sentinel into the viewport, then pass it with manual input.
    while ((await root.evaluate((element) => element.scrollTop)) > 0) {
      await scrollReview(testPage, root, mobile, -1);
    }
    await scrollReview(testPage, root, mobile, 1);
    await expect(checkbox).toBeChecked();
    await expect(dialog.getByText(/^1 of \d+ files reviewed$/)).toBeVisible();
    await dialog
      .locator("button")
      .filter({ has: testPage.locator(".tabler-icon-settings") })
      .click();
    await testPage.getByRole("menuitem", { name: "Auto-mark reviewed on scroll" }).click();
    await testPage.keyboard.press("Escape");
    if (mobile) await checkbox.tap();
    else await checkbox.click();
    await expect(checkbox).not.toBeChecked();
    while ((await root.evaluate((element) => element.scrollTop)) > 0) {
      await scrollReview(testPage, root, mobile, -1);
    }
    await scrollReview(testPage, root, mobile, 1);
    await dwell(
      testPage,
      250,
      "negative-assertion",
      "disabled auto-marking must not reselect after a new pass",
    );
    await expect(checkbox).not.toBeChecked();
    await expect(testPage.locator("html")).toHaveJSProperty(
      "scrollWidth",
      await testPage.locator("html").evaluate((element) => element.clientWidth),
    );
  });
}
