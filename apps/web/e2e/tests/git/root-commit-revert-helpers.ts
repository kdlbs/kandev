import { expect, type Page } from "@playwright/test";
import type { AppState } from "../../../lib/state/store";
import { dwell } from "../../helpers/causal-waits";
import { waitForFiniteAnimations } from "../../helpers/animations";
import { seedHistoryRelation } from "./changes-history-regression-helpers";

export const ROOT_ROW = "commit-row-ccccccc";

export async function setParent(page: Page, parent: string) {
  await seedHistoryRelation(page, "stale");
  await page.evaluate((parentSha) => {
    const state = (
      window as Window & { __KANDEV_E2E_STORE__: { getState: () => AppState } }
    ).__KANDEV_E2E_STORE__.getState();
    const session = state.tasks.activeSessionId;
    if (!session) throw new Error("Active session missing");
    const environment = state.environmentIdBySessionId[session];
    if (!environment) throw new Error("Active environment missing");
    const commits = state.sessionCommits.byEnvironmentId[environment];
    if (!commits?.length) throw new Error("Commit fixture missing");
    state.setSessionCommits(
      session,
      commits.map((commit) => ({ ...commit, parent_sha: parentSha })),
    );
  }, parent);
  const toggle = page.getByTestId("commits-section-collapse-toggle");
  if ((await toggle.getAttribute("aria-expanded")) !== "true") await toggle.click();
  await expect(page.getByTestId(ROOT_ROW)).toBeVisible();
}

export async function longPressCommit(page: Page) {
  const row = page.getByTestId(ROOT_ROW);
  await row.scrollIntoViewIfNeeded();
  const box = await row.boundingBox();
  if (!box) throw new Error("Commit row has no bounds");
  const cdp = await page.context().newCDPSession(page);
  try {
    await cdp.send("Input.dispatchTouchEvent", {
      type: "touchStart",
      touchPoints: [{ x: box.x + box.width / 2, y: box.y + box.height / 2 }],
    });
    await dwell(page, 800, "library-timer", "Radix context menu long-press threshold");
    await cdp.send("Input.dispatchTouchEvent", { type: "touchEnd", touchPoints: [] });
    await expect(page.getByRole("menu")).toBeVisible();
  } finally {
    await cdp.detach();
  }
}

export async function expectMenu(page: Page, parented: boolean, capture?: () => Promise<void>) {
  const menu = page.getByRole("menu");
  await expect(menu.getByRole("menuitem", { name: "Amend message" })).toBeVisible();
  await expect(menu.getByRole("menuitem", { name: "Reset to this commit" })).toBeVisible();
  await expect(menu.getByRole("menuitem", { name: "Revert commit" })).toHaveCount(parented ? 1 : 0);
  if (capture) {
    await waitForFiniteAnimations(menu);
    await capture();
  }
  await page.keyboard.press("Escape");
  await expect(menu).toBeHidden();
}
