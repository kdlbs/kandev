// Regression guard for multi-file agent read links. OMP can read several
// comma-joined files in one read call ("a.ts:1-40,b.ts:1-40"); the backend keeps
// that path raw and the chat read card must split it into one openable link per
// file (selector excluded) instead of a single combined, un-openable link that
// fails with "file not found". Companion to the single-file open-and-scroll
// coverage in mobile-file-viewer.spec.ts.
import path from "node:path";
import { test, expect } from "../../fixtures/test-base";
import { GitHelper, makeGitEnv, createStandardProfile } from "../../helpers/git-helper";
import { SessionPage } from "../../pages/session-page";

test.describe("Chat multi-file read links", () => {
  test("splits a comma-joined read into one openable link per file", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    test.setTimeout(90_000);

    const suffix = `${Date.now()}-${Math.random().toString(36).slice(2, 8)}`;
    const fileA = `alpha-${suffix}.ts`;
    const fileB = `beta-${suffix}.ts`;
    const git = new GitHelper(
      path.join(backend.tmpDir, "repos", "e2e-repo"),
      makeGitEnv(backend.tmpDir),
    );
    git.createFile(fileA, 'export const alpha = "alpha";');
    git.createFile(fileB, 'export const beta = "beta";');
    git.stageAll();
    git.commit(`add ${fileA} and ${fileB}`);

    const profile = await createStandardProfile(apiClient, `multi-read-${Date.now()}`);
    // `e2e:message(...)` emits a single non-activity text message, so the seeded
    // read card is the only activity message and renders as a standalone card.
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Chat Multi-File Read",
      profile.id,
      {
        description: 'e2e:message("ready")',
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );
    if (!task.session_id) throw new Error("createTaskWithAgent did not return a session_id");

    // Seed a read card whose file_path bundles two files with line selectors,
    // exactly as the backend stores a multi-file omp read.
    await apiClient.seedSessionMessage(task.session_id, {
      type: "tool_read",
      content: "Read files",
      metadata: {
        status: "complete",
        tool_call_id: "tc-multi-file-read",
        normalized: {
          read_file: {
            file_path: `${fileA}:1-40,${fileB}:1-40`,
            output: { content: 'export const alpha = "alpha";', line_count: 40 },
          },
        },
      },
    });

    await testPage.goto(`/t/${task.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await session.waitForChatIdle({ timeout: 45_000 });

    // Each file is its own openable link (FilePathButton renders the bare path as
    // both the title and the button text) — never a single combined link.
    const chat = session.activeChat();
    const linkA = chat.locator(`button[title="${fileA}"]`);
    const linkB = chat.locator(`button[title="${fileB}"]`);
    await expect(linkA).toBeVisible({ timeout: 15_000 });
    await expect(linkB).toBeVisible({ timeout: 15_000 });
    // The combined path with embedded line numbers must NOT be a link.
    await expect(chat.locator(`button[title="${fileA}:1-40,${fileB}:1-40"]`)).toHaveCount(0);

    // Clicking the second link opens its bare path in the editor (the original
    // bug failed here with "file not found" because the line numbers stayed on
    // the path).
    await linkB.click();
    await expect(testPage.getByTestId("preview-tab-file-editor")).toBeVisible({ timeout: 15_000 });
    await expect(testPage.locator(".dv-tab", { hasText: fileB })).toBeVisible({ timeout: 15_000 });
  });
});
