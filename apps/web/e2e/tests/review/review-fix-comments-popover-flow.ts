import { expect, type Locator, type Page } from "@playwright/test";
import { SessionPage } from "../../pages/session-page";
import type { ApiClient } from "../../helpers/api-client";
import type { SeedData } from "../../fixtures/test-base";

// The single file the `/e2e:review-cumulative-setup` scenario produces. A
// pending comment must reference a file present in the diff for the "Fix
// Comments" button to render (see computeCommentCounts in review-dialog.tsx).
export const DIFF_FILE = "review_cumulative_test.txt";

export type SeededComment = {
  id: string;
  sessionId: string;
  source: "diff";
  filePath: string;
  startLine: number;
  endLine: number;
  side: "additions" | "deletions";
  codeContent: string;
  text: string;
  createdAt: string;
  status: "pending";
};

function makeComment(sessionId: string, index: number, filePath: string): SeededComment {
  return {
    id: `e2e-comment-${index}`,
    sessionId,
    source: "diff",
    filePath,
    startLine: index,
    endLine: index,
    side: "additions",
    codeContent: `line ${index}`,
    text: `E2E review comment number ${index} — needs a change here.`,
    createdAt: new Date(2026, 0, 1, 0, 0, index).toISOString(),
    status: "pending",
  };
}

/**
 * Seed pending diff comments into sessionStorage *before* navigation so the
 * comments store's `hydrateSession` picks them up on session mount. Mirrors the
 * addInitScript pattern in review-sidebar-resize.spec.ts.
 *
 * The first comment references the real diff file so the "Fix Comments" button
 * renders; the rest are spread across several files to exercise per-file
 * grouping and to overflow the popover's max height so it scrolls.
 */
export async function seedComments(testPage: Page, sessionId: string): Promise<SeededComment[]> {
  const comments: SeededComment[] = [
    makeComment(sessionId, 1, DIFF_FILE),
    ...Array.from({ length: 40 }, (_, k) =>
      makeComment(sessionId, k + 2, `src/module-${(k % 5) + 1}/file-${(k % 5) + 1}.ts`),
    ),
  ];
  await testPage.addInitScript(
    ({ key, val }) => {
      try {
        sessionStorage.setItem(key, val);
      } catch {
        // sessionStorage may be unavailable in some contexts; ignore.
      }
    },
    { key: `kandev.comments.${sessionId}`, val: JSON.stringify(comments) },
  );
  return comments;
}

export async function seedReviewTask(testPage: Page, apiClient: ApiClient, seedData: SeedData) {
  return apiClient.createTaskWithAgent(
    seedData.workspaceId,
    "Review Fix Comments Popover E2E",
    seedData.agentProfileId,
    {
      description: "/e2e:review-cumulative-setup",
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    },
  );
}

export async function loadSession(testPage: Page, taskId: string) {
  await testPage.goto(`/t/${taskId}`);
  const session = new SessionPage(testPage);
  await session.waitForLoad();
  await expect(
    session.chat.getByText("review-cumulative-setup complete", { exact: false }),
  ).toBeVisible({ timeout: 45_000 });
  return session;
}

export async function openDialogWithChanges(testPage: Page) {
  const changesTab = testPage.locator(".dv-default-tab", { hasText: "Changes" });
  const mobileChangesButton = testPage.getByRole("button", { name: /Changes/ }).last();
  const hasMobileChangesNav =
    (await mobileChangesButton.count()) > 0 &&
    (await mobileChangesButton.isVisible().catch(() => false));

  if (hasMobileChangesNav) {
    await mobileChangesButton.tap();
    await expect(testPage.getByTestId("mobile-changes-panel")).toBeVisible({ timeout: 15_000 });
  } else {
    await expect(changesTab).toBeVisible({ timeout: 10_000 });
    await changesTab.click();
  }

  await expect(testPage.getByTestId(`file-row-${DIFF_FILE}`)).toBeVisible({ timeout: 15_000 });
  await testPage.evaluate(() => window.dispatchEvent(new CustomEvent("open-review-dialog")));
  const dialog = testPage.getByRole("dialog", { name: "Review Changes" });
  await expect(dialog).toBeVisible({ timeout: 10_000 });
  return dialog;
}

/**
 * Open the hover popover by moving the real cursor onto the button and also
 * dispatching the hover events, then assert the overview is visible. Mirrors
 * SessionPage.hoverPRTopbar — the popover is driven by useHoverPopover, not a
 * native :hover, and needs both the cursor move and the synthetic events to
 * open reliably across browsers.
 */
export async function hoverFixComments(testPage: Page, button: Locator, overview: Locator) {
  await expect(async () => {
    const box = await button.boundingBox();
    expect(box).not.toBeNull();
    await testPage.mouse.move(0, 0);
    await testPage.mouse.move(box!.x + box!.width / 2, box!.y + box!.height / 2);
    await button.dispatchEvent("mouseover", { bubbles: true });
    await button.dispatchEvent("mouseenter", { bubbles: false });
    await button.dispatchEvent("mousemove", { bubbles: true });
    await expect(overview).toBeVisible({ timeout: 1_500 });
  }).toPass({ timeout: 10_000 });
}
