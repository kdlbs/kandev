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
  if (touch) await expand.tap();
  else await expand.click();
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
