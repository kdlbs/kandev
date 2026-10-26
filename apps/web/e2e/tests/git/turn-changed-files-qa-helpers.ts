import { rmSync } from "node:fs";
import { join } from "node:path";
import type { Page } from "@playwright/test";
import { waitForFiniteAnimations } from "../../helpers/pr-capture";
import { expect } from "../../fixtures/test-base";

export const QA_FILE_PATH =
  "src/features/authentication/components/very-long-component-name-that-needs-disclosure.tsx";
export const QA_OLD_PATH = "src/legacy/authentication/previous-component-name.tsx";

export async function routeRenamedTurnFile(page: Page) {
  await page.route("**/turn-changes/*/repositories/*/files?*", async (route) => {
    const response = await route.fetch();
    const original = await response.json();
    await route.fulfill({
      json: {
        ...original,
        files: original.files.map((file: Record<string, unknown>) => ({
          ...file,
          path: QA_FILE_PATH,
          old_path: QA_OLD_PATH,
          kind: "renamed",
          old_mode: "100644",
          new_mode: "100644",
        })),
      },
    });
  });
}

export async function assertTurnFileDisclosure(page: Page, touch = false) {
  const card = page.getByTestId("turn-changed-files-card");
  const expand = card.getByRole("button", { name: "Expand all", exact: true });
  if (await expand.isVisible()) {
    if (touch) await expand.tap();
    else await expand.click();
  }
  await assertCardResponsiveContainment(page, card);
  await assertCardTouchTargets(page);
  const file = card.locator("[data-turn-file-change-id]");
  if (touch) await file.tap();
  else await file.click();
  const drawer = page.getByTestId("mobile-diff-sheet");
  await waitForFiniteAnimations(drawer);
  await expect(drawer.getByRole("button", { name: "Close", exact: true })).toHaveCount(1);
  const bounds = await drawer.boundingBox();
  expect(bounds!.y).toBeLessThanOrEqual(1);
  expect(bounds!.height).toBeGreaterThanOrEqual(page.viewportSize()!.height - 1);
  const viewer = page.getByTestId("historical-turn-diff");
  await expect(viewer.getByTestId("turn-change-file-path")).toHaveText(QA_FILE_PATH);
  await expect(viewer.getByTestId("turn-change-old-path")).toContainText(QA_OLD_PATH);
  await expect(viewer.getByRole("combobox", { name: "Select a changed file" })).not.toContainText(
    /[0-9a-f]{8}-[0-9a-f]{4}-/,
  );
  const path = viewer.getByTestId("turn-change-file-path");
  expect(await path.evaluate((el) => el.scrollWidth <= el.clientWidth)).toBe(true);
}

export async function assertCardResponsiveContainment(
  page: Page,
  card = page.getByTestId("turn-changed-files-card"),
) {
  await expect(card).toBeVisible();
  const documentFits = await page.evaluate(
    () => document.documentElement.scrollWidth <= document.documentElement.clientWidth,
  );
  expect(documentFits).toBe(true);

  const cardBounds = await card.boundingBox();
  expect(cardBounds).not.toBeNull();
  expect(cardBounds!.x).toBeGreaterThanOrEqual(0);
  expect(cardBounds!.x + cardBounds!.width).toBeLessThanOrEqual(page.viewportSize()!.width + 1);

  const rows = card.locator("[data-turn-file-change-id]");
  for (const row of await rows.all()) {
    const geometry = await row.evaluate((element) => {
      const rowBounds = element.getBoundingClientRect();
      const cardBounds = element
        .closest("[data-testid='turn-changed-files-card']")!
        .getBoundingClientRect();
      return {
        rowLeft: rowBounds.left,
        rowRight: rowBounds.right,
        cardLeft: cardBounds.left,
        cardRight: cardBounds.right,
        rowWidth: element.clientWidth,
        rowScrollWidth: element.scrollWidth,
      };
    });
    expect(geometry.rowLeft).toBeGreaterThanOrEqual(geometry.cardLeft - 1);
    expect(geometry.rowRight).toBeLessThanOrEqual(geometry.cardRight + 1);
    expect(geometry.rowScrollWidth).toBeLessThanOrEqual(geometry.rowWidth + 1);
    await expect(row).toContainText(/Renamed|Modified|Added|Deleted/);
  }
}

export async function assertCardTouchTargets(page: Page) {
  const card = page.getByTestId("turn-changed-files-card");
  await expect(card).toBeVisible();
  for (const button of await card.getByRole("button").all()) {
    const box = await button.boundingBox();
    expect(box?.height).toBeGreaterThanOrEqual(44);
  }
}

export function resetTurnQaFile(repositoryPath: string) {
  rmSync(join(repositoryPath, "untracked_test.txt"), { force: true });
}

export async function routeEmptyUnavailableTurn(page: Page, sessionId: string) {
  await page.route(`**/api/v1/task-sessions/${sessionId}/turn-changes?*`, async (route) => {
    const response = await route.fetch();
    const body = await response.json();
    await route.fulfill({
      response,
      json: {
        ...body,
        change_sets: body.change_sets.map((summary: Record<string, unknown>) => ({
          ...summary,
          revision: Number(summary.revision) + 1,
          availability: "unavailable",
          reason: "checkout_unavailable",
          file_count: 0,
          complete: false,
          summary_complete: false,
        })),
      },
    });
  });
}

export async function routeTurnChangeFallback(page: Page) {
  await page.route("**/api/v1/task-sessions/*/turn-changes?*", async (route) => {
    const response = await route.fetch();
    const body = await response.json();
    await route.fulfill({
      response,
      json: {
        ...body,
        change_sets: body.change_sets.map((summary: Record<string, unknown>) => ({
          ...summary,
          final_assistant_message_id: `missing-${String(summary.id)}`,
        })),
      },
    });
  });
}
