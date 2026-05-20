import { test, expect } from "../../fixtures/test-base";
import {
  assertHorizontalPaddingSymmetric,
  assertNoDescendantOverflowsRight,
  assertNoDocumentHorizontalOverflow,
} from "../../helpers/layout-assertions";

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
    await assertNoDocumentHorizontalOverflow(testPage, "AI Agents");
    await assertNoDescendantOverflowsRight(dialog, "AI Agents");
    // The DialogTitle ("AI Agents") renders synchronously regardless of
    // loading state — StepAgents only mounts the `.grid.gap-2` once the
    // /api/v1/agents/available probe resolves, so wait for an actual row
    // before asserting padding or the selector matches nothing.
    await expect(dialog.locator(".grid.gap-2 > *").first()).toBeVisible();
    // Padding around the agent row must match left/right — i.e. the gap from
    // the agent row to the dialog's left edge equals the gap to the right
    // edge. The `pr-1` scrollbar-gutter on the grid used to break this:
    // surrounding paragraphs sat at 17/17 while the row sat at 17/21.
    await assertHorizontalPaddingSymmetric(dialog, ".grid.gap-2 > *", "AI Agents agent row");

    // Step 1 — Executors
    await dialog.getByRole("button", { name: /next/i }).click();
    await expect(testPage.getByRole("heading", { name: "Executors" })).toBeVisible();
    await assertNoDocumentHorizontalOverflow(testPage, "Executors");
    await assertNoDescendantOverflowsRight(dialog, "Executors");

    // Step 2 — Agentic Workflows (the workflow step rows use whitespace-nowrap
    // by default, which is exactly the kind of layout that overflows a phone-
    // sized dialog).
    await dialog.getByRole("button", { name: /next/i }).click();
    await expect(testPage.getByRole("heading", { name: "Agentic Workflows" })).toBeVisible();
    await assertNoDocumentHorizontalOverflow(testPage, "Agentic Workflows");
    await assertNoDescendantOverflowsRight(dialog, "Agentic Workflows");

    // Step 3 — Command Panel
    await dialog.getByRole("button", { name: /next/i }).click();
    await expect(testPage.getByRole("heading", { name: "Command Panel" })).toBeVisible();
    await assertNoDocumentHorizontalOverflow(testPage, "Command Panel");
    await assertNoDescendantOverflowsRight(dialog, "Command Panel");
  });
});
