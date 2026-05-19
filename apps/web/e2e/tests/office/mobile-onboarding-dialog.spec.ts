import type { Locator } from "@playwright/test";
import { test, expect } from "../../fixtures/test-base";

// Runs under the mobile-chrome project (Pixel 5, viewport 393x851).
//
// Covers the OnboardingDialog that opens from `/` when the
// `kandev.onboarding.completed` localStorage flag is unset. It is distinct
// from the office /office/setup wizard (separate spec) — this dialog has
// four steps: AI Agents, Executors, Agentic Workflows, Command Panel.

test.describe("OnboardingDialog — mobile layout", () => {
  test("dialog and each step stay inside the viewport on Pixel 5", async ({ testPage }) => {
    // Undo the test-base init script that pre-marks onboarding completed —
    // otherwise the dialog never opens on `/`.
    await testPage.addInitScript(() => {
      localStorage.removeItem("kandev.onboarding.completed");
    });
    await testPage.goto("/");

    const dialog = testPage.getByRole("dialog");
    await expect(dialog).toBeVisible();
    await expect(testPage.getByRole("heading", { name: "AI Agents" })).toBeVisible();

    // Step 0 — AI Agents
    await assertNoHorizontalOverflow(dialog, "AI Agents");
    await assertChildrenFitInDialog(dialog, "AI Agents");

    // Step 1 — Executors
    await dialog.getByRole("button", { name: /next/i }).click();
    await expect(testPage.getByRole("heading", { name: "Executors" })).toBeVisible();
    await assertNoHorizontalOverflow(dialog, "Executors");
    await assertChildrenFitInDialog(dialog, "Executors");

    // Step 2 — Agentic Workflows (the workflow step rows use whitespace-nowrap
    // by default, which is exactly the kind of layout that overflows a phone-
    // sized dialog).
    await dialog.getByRole("button", { name: /next/i }).click();
    await expect(testPage.getByRole("heading", { name: "Agentic Workflows" })).toBeVisible();
    await assertNoHorizontalOverflow(dialog, "Agentic Workflows");
    await assertChildrenFitInDialog(dialog, "Agentic Workflows");

    // Step 3 — Command Panel
    await dialog.getByRole("button", { name: /next/i }).click();
    await expect(testPage.getByRole("heading", { name: "Command Panel" })).toBeVisible();
    await assertNoHorizontalOverflow(dialog, "Command Panel");
    await assertChildrenFitInDialog(dialog, "Command Panel");
  });
});

async function assertNoHorizontalOverflow(dialog: Locator, label: string): Promise<void> {
  const widths = await dialog.evaluate((el) => ({
    scroll: el.scrollWidth,
    client: el.clientWidth,
  }));
  expect(
    widths.scroll,
    `${label}: dialog scrollWidth (${widths.scroll}) exceeds clientWidth (${widths.client})`,
  ).toBeLessThanOrEqual(widths.client + 1);
}

async function assertChildrenFitInDialog(dialog: Locator, label: string): Promise<void> {
  const dialogBox = await dialog.boundingBox();
  expect(dialogBox, `${label}: dialog has no bounding box`).not.toBeNull();
  if (!dialogBox) return;
  const dialogRight = dialogBox.x + dialogBox.width;

  // Walk every visible descendant. Anything whose visible right edge exceeds
  // the dialog right edge is "content coming out of the modal on the right".
  // Done in one evaluate round-trip so this stays cheap on a deep DOM.
  const overflowing: { tag: string; text: string; right: number }[] = await dialog.evaluate(
    (root, dialogRightArg) => {
      const limit = dialogRightArg as number;
      const results: { tag: string; text: string; right: number }[] = [];
      const skip = new Set(["SVG", "PATH", "CIRCLE", "RECT", "LINE", "G"]);
      const all = root.querySelectorAll("*");
      for (const node of all) {
        if (skip.has(node.tagName)) continue;
        const rect = node.getBoundingClientRect();
        if (rect.width === 0 || rect.height === 0) continue;
        if (rect.right > limit + 1) {
          results.push({
            tag: node.tagName.toLowerCase(),
            text: (node.textContent ?? "").trim().slice(0, 80),
            right: rect.right,
          });
        }
      }
      return results;
    },
    dialogRight,
  );

  expect(
    overflowing,
    `${label}: ${overflowing.length} element(s) overflow the dialog right edge (${dialogRight.toFixed(
      1,
    )}). First few:\n${overflowing
      .slice(0, 5)
      .map((o) => `  <${o.tag}> right=${o.right.toFixed(1)} text="${o.text}"`)
      .join("\n")}`,
  ).toHaveLength(0);
}
