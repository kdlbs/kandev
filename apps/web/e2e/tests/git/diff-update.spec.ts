import { test, expect } from "../../fixtures/test-base";
import {
  fileDiffTab,
  getDiffsContainer,
  openChangesTab,
  openFileDiff,
  seedDiffUpdateTask,
  seedMultiFileTask,
  seedUntrackedFileTask,
  waitForDiffText,
  waitForDiffTextAbsent,
  waitForStoreFileDiffText,
} from "./diff-update-helpers";

test.describe("Diff update on file change", () => {
  test.describe.configure({ retries: 2, timeout: 120_000 });

  test("shows initial diff with FIRST_MODIFICATION", async ({ testPage, apiClient, seedData }) => {
    await seedDiffUpdateTask(testPage, apiClient, seedData);
    await openChangesTab(testPage);
    await openFileDiff(testPage, "diff_update_test.txt");

    // The Pierre Diffs viewer should show the initial modification.
    // Playwright's getByText auto-pierces shadow DOM and auto-retries, so we use it
    // directly with a generous timeout to handle async web worker initialization.
    // On cold CI runners (first test in shard, no V8 code cache), Pierre Diffs'
    // createJavaScriptRegexEngine() can take 30-40s to JIT-compile.
    const diffsContainer = getDiffsContainer(testPage);
    await expect(diffsContainer).toBeVisible({ timeout: 15_000 });
    await waitForDiffText(testPage, "FIRST_MODIFICATION");
  });

  test("diff updates when agent modifies file again", async ({ testPage, apiClient, seedData }) => {
    const { session } = await seedDiffUpdateTask(testPage, apiClient, seedData);
    await openChangesTab(testPage);
    await session.closeFileDiffPreview();
    await openFileDiff(testPage, "diff_update_test.txt");

    // Verify initial diff content (scoped to diffs-container to avoid matching chat text).
    // Allow up to 60s for Pierre Diffs engine JIT on cold CI runners.
    const diffsContainer = getDiffsContainer(testPage);
    await expect(diffsContainer).toBeVisible({ timeout: 15_000 });
    await waitForDiffText(testPage, "FIRST_MODIFICATION");

    // Click on the session tab to make the chat input visible again
    await session.clickSessionChatTab();

    // Send another message to trigger the second modification
    await session.sendMessage("/e2e:diff-update-modify");

    // Wait for the second turn to complete
    await expect(
      session.chat.getByText("diff-update-modify complete", { exact: false }),
    ).toBeVisible({ timeout: 45_000 });

    // Switch back to Changes tab and click on the diff file again to see the updated diff.
    // The git status (with diff data) should have been updated via polling when
    // the file changed - this is the bug we're testing for.
    await openChangesTab(testPage);
    await openFileDiff(testPage, "diff_update_test.txt");

    // Re-query the diffs container since the DOM may have changed after tab switch
    const updatedDiffsContainer = getDiffsContainer(testPage);
    await expect(updatedDiffsContainer).toBeVisible({ timeout: 15_000 });

    // The diff should now show SECOND_MODIFICATION instead of FIRST_MODIFICATION.
    // Allow extra time for git polling to detect the change and re-render the diff.
    await waitForDiffText(testPage, "SECOND_MODIFICATION", 30_000);

    // Verify FIRST_MODIFICATION is no longer shown (replaced, not merged)
    await waitForDiffTextAbsent(testPage, "FIRST_MODIFICATION");

    // Also verify the additional change on line 3
    await waitForDiffText(testPage, "ALSO_CHANGED", 15_000);
  });

  test("diff panel shows updated content after file changes", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    // This test verifies the Diff [file] preview shows the latest git-status
    // snapshot after the underlying file changes.
    const { session, sessionId } = await seedDiffUpdateTask(testPage, apiClient, seedData);
    await openChangesTab(testPage);
    await openFileDiff(testPage, "diff_update_test.txt");

    // Verify initial diff content
    const diffsContainer = getDiffsContainer(testPage);
    await expect(diffsContainer).toBeVisible({ timeout: 15_000 });
    await waitForDiffText(testPage, "FIRST_MODIFICATION", 15_000);

    // Switch to chat, trigger the second modification
    await session.clickSessionChatTab();
    await session.sendMessage("/e2e:diff-update-modify");
    await expect(
      session.chat.getByText("diff-update-modify complete", { exact: false }),
    ).toBeVisible({ timeout: 45_000 });

    // Wait for the Changes panel's git status source to observe the second
    // modification. Do not re-open the file diff; just verify the status source
    // the already-open diff panel depends on has moved past the initial state.
    await openChangesTab(testPage);
    const changedFileRow = testPage.getByTestId("file-row-diff_update_test.txt");
    await expect(changedFileRow.getByText("+2", { exact: true })).toBeVisible({
      timeout: 45_000,
    });
    await expect(changedFileRow.getByText("-2", { exact: true })).toBeVisible({
      timeout: 5_000,
    });
    await waitForStoreFileDiffText(
      testPage,
      sessionId,
      "diff_update_test.txt",
      "SECOND_MODIFICATION",
      30_000,
    );

    await session.closeFileDiffPreview();
    await openFileDiff(testPage, "diff_update_test.txt");

    await waitForDiffText(testPage, "SECOND_MODIFICATION", 15_000);
    await waitForDiffText(testPage, "ALSO_CHANGED", 15_000);
  });

  test("diff panel closes when uncommitted change is undone via hunk Undo", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    // Regression: Undo must close the diff tab even when PR/cumulative diffs keep the file visible.
    await seedDiffUpdateTask(testPage, apiClient, seedData);
    await openChangesTab(testPage);
    await openFileDiff(testPage, "diff_update_test.txt");

    const diffTab = fileDiffTab(testPage, "diff_update_test.txt");
    await expect(diffTab).toBeVisible({ timeout: 10_000 });

    const diffsContainer = getDiffsContainer(testPage);
    await expect(diffsContainer).toBeVisible({ timeout: 15_000 });
    await waitForDiffText(testPage, "FIRST_MODIFICATION", 15_000);

    // Button is CSS-hidden until hover; dispatchEvent bypasses pointer-events:none.
    const undoBtn = diffsContainer.locator("[data-undo-btn] button").first();
    await expect(undoBtn).toHaveCount(1, { timeout: 10_000 });
    await undoBtn.dispatchEvent("click");

    // The Diff tab should close automatically.
    await expect(diffTab).toHaveCount(0, { timeout: 15_000 });
  });
});

test.describe("File editor auto-update on file change", () => {
  test.describe.configure({ retries: 2, timeout: 120_000 });

  test("editor panel auto-updates without re-opening when file changes", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    // Regression: opening a file in the FileEditorPanel and then having the
    // agent modify it should reactively update the editor content WITHOUT the
    // user re-clicking the file.
    const { session } = await seedDiffUpdateTask(testPage, apiClient, seedData);

    // Open the file in the file editor via the Files tree.
    await session.clickTab("Files");
    await expect(session.files).toBeVisible({ timeout: 10_000 });
    const fileRow = session.files.getByText("diff_update_test.txt");
    await expect(fileRow).toBeVisible({ timeout: 10_000 });
    await fileRow.click();

    // The editor tab should appear in dockview.
    const editorTab = testPage.locator(".dv-default-tab", { hasText: "diff_update_test.txt" });
    await expect(editorTab).toBeVisible({ timeout: 10_000 });

    // Verify initial content shows FIRST_MODIFICATION (default Monaco editor).
    const editorContent = testPage.locator(".view-lines").first();
    await expect(editorContent).toContainText("FIRST_MODIFICATION", { timeout: 15_000 });

    // Switch to chat and trigger the second modification.
    await session.clickSessionChatTab();
    await session.sendMessage("/e2e:diff-update-modify");
    await expect(
      session.chat.getByText("diff-update-modify complete", { exact: false }),
    ).toBeVisible({ timeout: 45_000 });

    // Click the editor tab back to view it (do NOT re-open from file tree).
    // The panel is still mounted, just not the active tab.
    await editorTab.click();

    // The editor should auto-update with SECOND_MODIFICATION content.
    // Re-query .view-lines since DOM may have re-rendered after tab switch.
    const updatedEditorContent = testPage.locator(".view-lines").first();
    await expect(updatedEditorContent).toContainText("SECOND_MODIFICATION", { timeout: 30_000 });
    await expect(updatedEditorContent).toContainText("ALSO_CHANGED", { timeout: 15_000 });

    // FIRST_MODIFICATION should be gone.
    await expect(updatedEditorContent).not.toContainText("FIRST_MODIFICATION", { timeout: 5_000 });
  });

  test("editor + diff panels auto-update while agent is streaming (mid-turn)", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    // Reproduces the user-reported bug: open both editor and diff from the
    // Changes tab, then have the agent modify the file MID-TURN (still
    // streaming). Both panels must auto-update within a few seconds while
    // the agent's turn is still active — without re-opening the file.
    const { session } = await seedDiffUpdateTask(testPage, apiClient, seedData);

    // Open the diff panel from the Changes tab.
    await openChangesTab(testPage);
    await openFileDiff(testPage, "diff_update_test.txt");
    const diffsContainer = getDiffsContainer(testPage);
    await expect(diffsContainer).toBeVisible({ timeout: 15_000 });
    await waitForDiffText(testPage, "FIRST_MODIFICATION", 15_000);

    // Also open the file editor for the same file via the Files tree.
    await session.clickTab("Files");
    await expect(session.files).toBeVisible({ timeout: 5_000 });
    const fileRow = session.files.getByText("diff_update_test.txt");
    await expect(fileRow).toBeVisible({ timeout: 10_000 });
    await fileRow.click();
    const editorTab = testPage.locator(".dv-default-tab[type='file-editor']", {
      hasText: "diff_update_test.txt",
    });
    await expect(editorTab).toBeVisible({ timeout: 10_000 });
    const editorContent = testPage.locator(".view-lines").first();
    await expect(editorContent).toContainText("FIRST_MODIFICATION", { timeout: 15_000 });

    // Trigger the streaming scenario: agent will write the file mid-turn and
    // keep emitting text for ~6s afterwards. Do NOT wait for completion —
    // we want to assert updates while the turn is still streaming.
    await session.clickSessionChatTab();
    await session.sendMessage("/e2e:diff-update-streaming");

    // Wait for the agent to confirm it has started — turn is now live.
    await expect(session.chat.getByText("starting work", { exact: false })).toBeVisible({
      timeout: 30_000,
    });

    // While the agent is still mid-turn (it has ~6s of trailing delay), both
    // panels must reflect the new content. This is the user's bug scenario.
    // Click back to the editor tab to make assertions.
    await editorTab.click();
    const liveEditorContent = testPage.locator(".view-lines").first();
    await expect(liveEditorContent).toContainText("SECOND_MODIFICATION", { timeout: 8_000 });
    await expect(liveEditorContent).toContainText("ALSO_CHANGED", { timeout: 5_000 });

    // Switch to the diff tab and assert it also updated.
    const diffTab = fileDiffTab(testPage, "diff_update_test.txt");
    await expect(diffTab).toBeVisible({ timeout: 10_000 });
    await diffTab.click();
    const liveDiffsContainer = getDiffsContainer(testPage);
    await expect(liveDiffsContainer).toBeVisible({ timeout: 5_000 });
    await waitForDiffText(testPage, "SECOND_MODIFICATION", 8_000);
    await waitForDiffText(testPage, "ALSO_CHANGED", 5_000);
  });
});

test.describe("Multi-file editor + diff auto-update", () => {
  test.describe.configure({ retries: 2, timeout: 180_000 });

  test("diff panel auto-updates across all 3 files during a single multi-file streaming turn", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    // Reproduces the user-reported "multiple files / diffs opened" case.
    // The Changes panel mounts useFileEditors. Opening a diff for file_a
    // mounts another instance. A single multi-file streaming modify turn
    // must propagate to gitStatus so:
    //   - The currently open diff (file_a) auto-updates mid-turn.
    //   - Switching the preview to file_b shows file_b's NEW diff immediately
    //     (not the pre-turn FIRST_MODIFICATION).
    //   - Same for file_c.
    // This exercises the gitStatus-driven re-render path without the user
    // re-triggering anything per-file.
    const { session } = await seedMultiFileTask(testPage, apiClient, seedData);
    const fileA = "multi_a.txt";
    const fileB = "multi_b.txt";
    const fileC = "multi_c.txt";

    // Open the diff preview for file_a from the Changes panel.
    await openChangesTab(testPage);
    await openFileDiff(testPage, fileA);
    const diffsContainer = getDiffsContainer(testPage);
    await expect(diffsContainer).toBeVisible({ timeout: 15_000 });
    await waitForDiffText(testPage, "FIRST_MODIFICATION", 15_000);

    // Trigger the multi-file streaming modification. Don't wait for completion.
    await session.clickSessionChatTab();
    await session.sendMessage("/e2e:multi-file-modify");
    await expect(session.chat.getByText("starting work", { exact: false })).toBeVisible({
      timeout: 30_000,
    });

    // Click back to the file_a diff tab so its panel becomes the visible one.
    const diffTabA = fileDiffTab(testPage, fileA);
    await expect(diffTabA).toBeVisible({ timeout: 10_000 });
    await diffTabA.click();

    // While the agent is still streaming, the file_a diff must reflect
    // SECOND_MODIFICATION + ALSO_CHANGED_0.
    await waitForDiffText(testPage, "SECOND_MODIFICATION", 15_000);
    await waitForDiffText(testPage, "ALSO_CHANGED_0", 5_000);

    // Swap the diff preview to file_b — the gitStatus-driven content must
    // already have SECOND_MODIFICATION available without re-running the agent.
    await openChangesTab(testPage);
    await openFileDiff(testPage, fileB);
    await waitForDiffText(testPage, "ALSO_CHANGED_1", 15_000);

    // And again for file_c.
    await openChangesTab(testPage);
    await openFileDiff(testPage, fileC);
    await waitForDiffText(testPage, "ALSO_CHANGED_2", 15_000);
  });
});

test.describe("User-save then diff view (colleague repro)", () => {
  // Intermittent bug: after agent modifies a file, user opens it in editor,
  // edits + saves, then switches to diff view \u2014 the diff panel shows the
  // pre-save content (agent's edit only, not the user's save). Workaround
  // reported: leave the task and re-enter. We try to repro by driving the
  // exact UI sequence and asserting the diff contains the user's marker.
  test.describe.configure({ retries: 2, timeout: 120_000 });

  test("diff shows user's edit after open-edit-save sequence", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const USER_MARKER = "USER_EDIT_MARKER_42";
    const { session } = await seedDiffUpdateTask(testPage, apiClient, seedData);

    // Step 1: agent has already modified diff_update_test.txt with FIRST_MODIFICATION.
    // Wait for the Changes panel auto-activate (fires once gitStatus 0\u2192N) to
    // settle before we click Files \u2014 otherwise the auto-activate races with
    // our click and Files ends up inactive. Match any "Changes (N)" count so we
    // don't fail when the badge briefly reads a different value.
    await expect(testPage.locator(".dv-default-tab", { hasText: /^Changes \(\d+\)/ })).toBeVisible({
      timeout: 30_000,
    });
    // Click Files and verify the file row becomes visible. If a late
    // auto-activate stole focus back to Changes, click Files again.
    const fileRow = session.fileTreeNode("diff_update_test.txt");
    await expect
      .poll(
        async () => {
          await session.clickTab("Files");
          return await fileRow.isVisible();
        },
        { timeout: 20_000, intervals: [500, 1000, 2000] },
      )
      .toBe(true);
    await fileRow.click();
    const editorTab = testPage.locator(".dv-default-tab[type='file-editor']", {
      hasText: "diff_update_test.txt",
    });
    await expect(editorTab).toBeVisible({ timeout: 10_000 });
    const editorContent = testPage.locator(".view-lines").first();
    await expect(editorContent).toContainText("FIRST_MODIFICATION", { timeout: 30_000 });

    // Step 2: type into Monaco \u2014 add a unique marker line at the end.
    // Click the view-lines area to focus the editor (Monaco's hidden textarea
    // captures input but isn't directly focusable across all browsers).
    await editorContent.click();
    const modifier = process.platform === "darwin" ? "Meta" : "Control";
    // Move caret to end of document, then add a new line with our marker.
    await testPage.keyboard.press(`${modifier}+End`);
    await testPage.keyboard.press("End");
    await testPage.keyboard.type(`\n${USER_MARKER}`);
    // Confirm the marker is present in the editor before saving.
    await expect(editorContent).toContainText(USER_MARKER, { timeout: 5_000 });

    // Step 3: save with Cmd/Ctrl+S.
    await testPage.keyboard.press(`${modifier}+s`);

    // Step 4: open the diff for the same file.
    await openChangesTab(testPage);
    await openFileDiff(testPage, "diff_update_test.txt");

    // Step 5: the diff must reflect the user's marker.
    const diffsContainer = getDiffsContainer(testPage);
    await expect(diffsContainer).toBeVisible({ timeout: 15_000 });
    await waitForDiffText(testPage, USER_MARKER, 15_000);
    // FIRST_MODIFICATION should still be present (agent's earlier edit).
    await waitForDiffText(testPage, "FIRST_MODIFICATION", 5_000);
  });
});

test.describe("Untracked file diff update", () => {
  test.describe.configure({ retries: 2, timeout: 120_000 });

  test("untracked file diff updates when modified", async ({ testPage, apiClient, seedData }) => {
    // This test verifies that modifying an untracked file triggers a git status update
    // and the diff viewer shows the updated content. This was a bug where the polling
    // mechanism didn't detect untracked file changes (git diff-files only shows tracked files).
    const { session } = await seedUntrackedFileTask(testPage, apiClient, seedData);
    await openChangesTab(testPage);
    await openFileDiff(testPage, "untracked_test.txt");

    // Verify initial diff content shows INITIAL_CONTENT
    // Note: exact match is false because the line shows "line 1: INITIAL_CONTENT"
    const diffsContainer = getDiffsContainer(testPage);
    await expect(diffsContainer).toBeVisible({ timeout: 15_000 });
    await waitForDiffText(testPage, "INITIAL_CONTENT", 15_000);

    // Click on the session tab to make the chat input visible again
    await session.clickSessionChatTab();

    // Send another message to trigger the modification
    await session.sendMessage("/e2e:untracked-file-modify");

    // Wait for the second turn to complete
    await expect(
      session.chat.getByText("untracked-file-modify complete", { exact: false }),
    ).toBeVisible({ timeout: 45_000 });

    // Wait for git polling to detect the file change (polling interval is ~1-2s)
    await testPage.waitForTimeout(3_000);

    // Switch back to Changes tab and click on the diff file again
    await openChangesTab(testPage);
    await openFileDiff(testPage, "untracked_test.txt");

    // Re-query the diffs container
    const updatedDiffsContainer = getDiffsContainer(testPage);
    await expect(updatedDiffsContainer).toBeVisible({ timeout: 15_000 });

    // The diff should now show MODIFIED_CONTENT instead of INITIAL_CONTENT
    // Note: exact match is false because the line includes prefix text
    // Give extra time for git polling to detect and refresh the diff view
    await waitForDiffText(testPage, "MODIFIED_CONTENT", 45_000);

    // Verify INITIAL_CONTENT is no longer shown
    await waitForDiffTextAbsent(testPage, "INITIAL_CONTENT");

    // Also verify the new line was added
    await waitForDiffText(testPage, "NEW_LINE", 15_000);
  });
});
