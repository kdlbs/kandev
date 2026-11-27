/**
 * Docs media generator — NOT a CI assertion. Skipped unless
 * CAPTURE_DOCS_MEDIA=1 is set, so it never runs in the normal e2e shards.
 *
 * `docs/screenshots/settings-executors.png` is embedded by
 * docs/public/executors.md and docs/screenshots.md. Nothing else produces it,
 * so it goes stale whenever the executor hub gains or loses a type — which is
 * exactly what adding Remote Docker did. This spec makes the recapture a
 * command instead of a manual chore.
 *
 * Regenerate (from apps/web):
 *
 *   CAPTURE_DOCS_MEDIA=1 pnpm e2e:raw --project=chromium --workers=1 \
 *     tests/settings/executors-docs-screenshots.spec.ts
 */
import fs from "node:fs";
import path from "node:path";
import { expect, test } from "../../fixtures/test-base";

const CAPTURE = process.env.CAPTURE_DOCS_MEDIA === "1";
const SCREENSHOTS_DIR = path.resolve(__dirname, "../../../../../docs/screenshots");

// The published asset is a 2x capture; docs embed it at 760px wide, so a 1x
// image renders soft on high-density displays.
test.use({ viewport: { width: 1280, height: 860 }, deviceScaleFactor: 2 });

test.describe("Executor settings docs screenshots", () => {
  test.skip(!CAPTURE, "docs media generator — set CAPTURE_DOCS_MEDIA=1 to run");

  test("captures the executors hub", async ({ testPage }) => {
    await testPage.goto("/settings/executors");

    // Every creation card must be present before capturing; a screenshot
    // taken mid-render is what produced the stale asset's missing types.
    for (const label of [/worktree/i, /docker/i, /kubernetes/i, /sprites/i, /SSH/i]) {
      await expect(testPage.getByRole("button", { name: label }).first()).toBeVisible();
    }
    await expect(testPage.getByRole("button", { name: /remote docker/i }).first()).toBeVisible();

    // Let transient toasts clear so they do not sit in the capture.
    await testPage
      .locator("[data-sonner-toast]")
      .first()
      .waitFor({ state: "detached", timeout: 8_000 })
      .catch(() => undefined);

    fs.mkdirSync(SCREENSHOTS_DIR, { recursive: true });
    await testPage.screenshot({
      path: path.join(SCREENSHOTS_DIR, "settings-executors.png"),
      fullPage: true,
    });
  });
});
