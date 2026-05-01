import { type Page } from "@playwright/test";
import { test, expect } from "../../fixtures/test-base";
import type { SeedData } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";
import { SessionPage } from "../../pages/session-page";

// ---------------------------------------------------------------------------
// Overview
// ---------------------------------------------------------------------------
//
// Covers the plan checkpointing feature (task plan revisions + rewind/revert).
//
// Coalesce window is set to 2000ms by the backend fixture
// (KANDEV_PLAN_COALESCE_WINDOW_MS=2000). Tests that cross the boundary use
// e2e:delay(2500) inside the mock-agent script; tests that stay within use
// e2e:delay(200).
//
// Agent-authored revisions are seeded through the MCP tool
// (create_task_plan_kandev). User-authored revisions are produced by typing
// into the plan editor and waiting for the 1500ms autosave debounce.

// ---------------------------------------------------------------------------
// Shared helpers
// ---------------------------------------------------------------------------

/** Create a task with agent, navigate to session, wait for agent idle. */
async function seedTaskAndWaitForIdle(
  testPage: Page,
  apiClient: ApiClient,
  seedData: SeedData,
  title: string,
  description = "/e2e:simple-message",
) {
  const task = await apiClient.createTaskWithAgent(
    seedData.workspaceId,
    title,
    seedData.agentProfileId,
    {
      description,
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    },
  );
  await testPage.goto(`/t/${task.id}`);

  const session = new SessionPage(testPage);
  await session.waitForLoad();
  // Agent writes may auto-switch the active panel to Plan, hiding the chat
  // input. Accept either the chat idle placeholder or the Plan panel as a
  // signal that the agent has produced output.
  await expect
    .poll(
      async () => (await session.idleInput().isVisible()) || (await session.planPanel.isVisible()),
      { timeout: 30_000, message: "agent output not visible" },
    )
    .toBe(true);

  return { session, taskId: task.id, sessionId: task.session_id! };
}

/** Ensure the plan panel is visible. When an agent write auto-opens it,
 * the toggle is unnecessary (and would close it). After a page reload the
 * panel may not auto-open, in which case togglePlanMode adds it; we poll
 * a few times because hydration can race the click. */
async function openPlanPanel(session: SessionPage) {
  if (await session.planPanel.isVisible()) return;
  for (let attempt = 0; attempt < 3; attempt++) {
    try {
      await session.togglePlanMode();
    } catch {
      // toggle button not yet ready — retry after a short wait
    }
    try {
      await session.planPanel.waitFor({ state: "visible", timeout: 5_000 });
      return;
    } catch {
      // Not visible yet; loop and try toggling again.
    }
  }
  await expect(session.planPanel).toBeVisible({ timeout: 10_000 });
}

/** Poll for the number of revision rows in the popover. */
async function expectRevisionCount(session: SessionPage, count: number, timeout = 10_000) {
  await expect(session.revisionRows()).toHaveCount(count, { timeout });
}

// MCP script building blocks — each emits one agent-authored plan revision.
function mcpWrite(content: string): string {
  // JSON-string-escape content: handles backslashes, quotes, and control chars.
  const escaped = JSON.stringify(content).slice(1, -1);
  return `e2e:mcp:kandev:create_task_plan_kandev({"task_id":"{task_id}","content":"${escaped}"})`;
}

/** Build a mock-agent script that chains plan writes with inter-write delays. */
function planWriteScript(writes: Array<{ content: string; delayMsAfter?: number }>): string {
  const lines: string[] = ['e2e:thinking("Seeding plan revisions...")'];
  for (const w of writes) {
    lines.push(mcpWrite(w.content));
    if (w.delayMsAfter) lines.push(`e2e:delay(${w.delayMsAfter})`);
  }
  lines.push('e2e:message("Revisions seeded.")');
  return lines.join("\n");
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

test.describe("Plan checkpointing — rewind UI", () => {
  test.describe.configure({ retries: 1 });

  test("empty plan: rewind button is disabled when no revisions exist", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(60_000);

    const { session } = await seedTaskAndWaitForIdle(
      testPage,
      apiClient,
      seedData,
      "Checkpoint empty",
    );
    await openPlanPanel(session);

    await expect(session.rewindButton()).toBeVisible({ timeout: 5_000 });
    // No plan yet → no revisions → button disabled.
    await expect(session.rewindButton()).toBeDisabled();
  });

  test("single agent write: rewind exposes one revision with current badge", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(90_000);

    const script = planWriteScript([{ content: "Initial plan" }]);
    const { session } = await seedTaskAndWaitForIdle(
      testPage,
      apiClient,
      seedData,
      "Checkpoint single",
      script,
    );
    await openPlanPanel(session);
    await expect(session.planPanel.getByText("Initial plan", { exact: false })).toBeVisible({
      timeout: 10_000,
    });

    await session.openRewind();
    await expectRevisionCount(session, 1);
    await expect(session.revisionRow(1).getByTestId("plan-revision-current-badge")).toBeVisible();
  });

  test("two agent writes within coalesce window: remain one revision", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(90_000);

    const script = planWriteScript([
      { content: "Draft v1", delayMsAfter: 200 },
      { content: "Draft v2 (merged)" },
    ]);
    const { session } = await seedTaskAndWaitForIdle(
      testPage,
      apiClient,
      seedData,
      "Checkpoint coalesce",
      script,
    );
    await openPlanPanel(session);
    await expect(session.planPanel.getByText("Draft v2", { exact: false })).toBeVisible({
      timeout: 10_000,
    });

    await session.openRewind();
    await expectRevisionCount(session, 1);
  });

  test("two agent writes across coalesce window: produce two revisions", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(90_000);

    const script = planWriteScript([
      { content: "First pass", delayMsAfter: 2500 },
      { content: "Second pass" },
    ]);
    const { session } = await seedTaskAndWaitForIdle(
      testPage,
      apiClient,
      seedData,
      "Checkpoint cross-window",
      script,
    );
    await openPlanPanel(session);
    await expect(session.planPanel.getByText("Second pass", { exact: false })).toBeVisible({
      timeout: 10_000,
    });

    await session.openRewind();
    await expectRevisionCount(session, 2);
    // Newest first: v2 has current badge; v1 has revert button instead.
    await expect(session.revisionRow(2).getByTestId("plan-revision-current-badge")).toBeVisible();
    await expect(session.revisionRow(1).getByTestId("plan-revision-current-badge")).toHaveCount(0);
    await expect(session.revisionRow(1).getByTestId("plan-revision-revert-button")).toBeVisible();
  });

  test("revert to earlier revision: HEAD updates and new revision appears", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);

    const script = planWriteScript([
      { content: "Agent draft A", delayMsAfter: 2500 },
      { content: "Agent draft B" },
    ]);
    const { session } = await seedTaskAndWaitForIdle(
      testPage,
      apiClient,
      seedData,
      "Checkpoint revert",
      script,
    );
    await openPlanPanel(session);
    await expect(session.planPanel.getByText("Agent draft B", { exact: false })).toBeVisible({
      timeout: 10_000,
    });

    await session.openRewind();
    await expectRevisionCount(session, 2);

    // Revert to v1.
    await session.revertToRevision(1);

    // HEAD content must now be the old v1 content.
    await expect(session.planPanel.getByText("Agent draft A", { exact: false })).toBeVisible({
      timeout: 10_000,
    });

    // Reopen popover; expect 3 revisions with restore marker on v3.
    await session.openRewind();
    await expectRevisionCount(session, 3);
    await expect(session.revisionRow(3).getByTestId("plan-revision-current-badge")).toBeVisible();
    await expect(session.revisionRow(3).getByTestId("plan-revision-revert-marker")).toBeVisible();
  });

  test("revert confirm dialog: cancel leaves revisions untouched", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(90_000);

    const script = planWriteScript([
      { content: "First", delayMsAfter: 2500 },
      { content: "Second" },
    ]);
    const { session } = await seedTaskAndWaitForIdle(
      testPage,
      apiClient,
      seedData,
      "Checkpoint cancel",
      script,
    );
    await openPlanPanel(session);

    await session.openRewind();
    await expectRevisionCount(session, 2);

    // Click revert on v1 then cancel the dialog.
    await session.revertButton(session.revisionRow(1)).click();
    await expect(session.revertConfirmDialog()).toBeVisible({ timeout: 5_000 });
    await session.revertConfirmCancel().click();
    await expect(session.revertConfirmDialog()).toBeHidden({ timeout: 5_000 });

    // HEAD is unchanged and the two revisions are still there.
    await expect(session.planPanel.getByText("Second", { exact: false })).toBeVisible();
    await session.openRewind();
    await expectRevisionCount(session, 2);
  });

  test("author switch breaks coalesce: user edit creates a second revision", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);

    const script = planWriteScript([{ content: "Agent wrote this" }]);
    const { session } = await seedTaskAndWaitForIdle(
      testPage,
      apiClient,
      seedData,
      "Checkpoint author switch",
      script,
    );
    await openPlanPanel(session);
    await expect(session.planPanel.getByText("Agent wrote this", { exact: false })).toBeVisible({
      timeout: 10_000,
    });

    // User types into the editor; autosave fires after 1500ms.
    const editor = session.planEditor();
    await editor.click();
    const modifier = process.platform === "darwin" ? "Meta" : "Control";
    await testPage.keyboard.press(`${modifier}+a`);
    await testPage.keyboard.type("User overwrote it");
    // Wait past autosave debounce + short settle time.
    await testPage.waitForTimeout(2_000);

    await session.openRewind();
    await expectRevisionCount(session, 2);
    // v2 (newest) is the user write — UI displays "You" for any user-authored
    // revision regardless of stored author_name. v1 stays as the agent label,
    // resolved from the active session's profile snapshot — exact value varies
    // per E2E setup ("Claude", "mock-default", …) so we just ensure it's a
    // non-empty agent name (i.e., not the user sentinel).
    const v2Author = session.revisionRow(2).getByTestId("plan-revision-author");
    const v1Author = session.revisionRow(1).getByTestId("plan-revision-author");
    await expect(v1Author).not.toHaveText("You");
    await expect(v1Author).not.toBeEmpty();
    await expect(v2Author).toHaveText("You");
  });

  test("preview dialog: row click opens content, restore goes through confirm", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);

    const script = planWriteScript([
      { content: "Older draft", delayMsAfter: 2500 },
      { content: "Newer draft" },
    ]);
    const { session } = await seedTaskAndWaitForIdle(
      testPage,
      apiClient,
      seedData,
      "Checkpoint preview",
      script,
    );
    await openPlanPanel(session);
    await session.openRewind();
    await expectRevisionCount(session, 2);

    // Open the older revision via row click.
    await session.openRevisionPreview(1);
    await expect(session.previewBody()).toContainText("Older draft", { timeout: 5_000 });
    await expect(session.previewRestoreButton()).toBeVisible();

    // Restore from preview routes through the confirm dialog.
    await session.previewRestoreButton().click();
    await expect(session.revertConfirmDialog()).toBeVisible({ timeout: 5_000 });
    await session.revertConfirmOk().click();
    await expect(session.revertConfirmDialog()).toBeHidden({ timeout: 5_000 });
    // HEAD is now the older content again.
    await expect(session.planPanel.getByText("Older draft", { exact: false })).toBeVisible({
      timeout: 10_000,
    });

    await session.openRewind();
    await expectRevisionCount(session, 3);
  });

  test("compare diff: open from preview, view unified + split, restore older", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);

    const script = planWriteScript([
      { content: "alpha\nbeta\ngamma", delayMsAfter: 2500 },
      { content: "alpha\nBETA\ngamma\ndelta" },
    ]);
    const { session } = await seedTaskAndWaitForIdle(
      testPage,
      apiClient,
      seedData,
      "Checkpoint compare",
      script,
    );
    await openPlanPanel(session);
    await session.openRewind();
    await expectRevisionCount(session, 2);

    // Open the older revision in preview, then "Compare with current" — this
    // is the only entry point now; there is no per-row compare toggle.
    await session.openRevisionPreview(1);
    await session.previewCompareWithCurrentButton().click();
    await expect(session.diffDialog()).toBeVisible({ timeout: 5_000 });
    await expect(session.diffDialog()).toContainText("Compare v1 → v2");

    // Summary + unified diff lines.
    await expect(session.diffSummary()).toContainText("added", { timeout: 5_000 });
    await expect(session.diffSummary()).toContainText("removed");
    await expect(session.diffLines("add").first()).toBeVisible();
    await expect(session.diffLines("remove").first()).toBeVisible();

    // Switch to split mode: side-by-side cells render with kinds.
    await session.diffModeToggle("split").click();
    await expect(session.diffSplitCells("remove").first()).toBeVisible({ timeout: 5_000 });
    await expect(session.diffSplitCells("add").first()).toBeVisible();

    // Back to unified.
    await session.diffModeToggle("unified").click();
    await expect(session.diffLines("add").first()).toBeVisible();

    // Restore from diff targets the older revision (v1).
    await session.diffRestoreButton().click();
    await expect(session.revertConfirmDialog()).toBeVisible({ timeout: 5_000 });
    await expect(session.revertConfirmDialog()).toContainText("version 1");
    await session.revertConfirmCancel().click();
  });

  test("persistence across reload: revisions survive page refresh", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);

    const script = planWriteScript([{ content: "One", delayMsAfter: 2500 }, { content: "Two" }]);
    const { session, taskId } = await seedTaskAndWaitForIdle(
      testPage,
      apiClient,
      seedData,
      "Checkpoint reload",
      script,
    );
    await openPlanPanel(session);
    await session.openRewind();
    await expectRevisionCount(session, 2);

    // Reload and reopen the panel.
    await testPage.goto(`/t/${taskId}`);
    const freshSession = new SessionPage(testPage);
    await freshSession.waitForLoad();
    await openPlanPanel(freshSession);
    await freshSession.openRewind();
    await expectRevisionCount(freshSession, 2);
  });
});
