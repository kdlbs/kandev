import { execFileSync } from "node:child_process";
import fs from "node:fs";
import path from "node:path";
import { expect, test } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";
import { waitForHttp, watchWs } from "../../helpers/causal-waits";
import { makeGitEnv } from "../../helpers/git-helper";
import { waitForSessionDone } from "../../helpers/session";
import { SessionPage } from "../../pages/session-page";

const WORKSPACE_SOURCES_PATH = /^\/api\/v1\/tasks\/[^/]+\/workspace-sources$/;
const SETUP_MARKER = "issue5151-setup-ok";
const REPOSITORY_NAME = "script-source-repository";
const WORKTREE_NAME = `${REPOSITORY_NAME}-main`;

/**
 * A local Git repository with a real offline origin, mirroring the fixture in
 * add-workspace-sources.spec.ts: workspace-source materialization refreshes the
 * required branch before it creates the attached worktree.
 */
function createScriptedRepository(root: string) {
  const gitEnv = makeGitEnv(root);
  const base = path.join(root, "scripted-sources");
  const repositoryPath = path.join(base, REPOSITORY_NAME);
  const originPath = path.join(base, `${REPOSITORY_NAME}-origin.git`);
  fs.mkdirSync(repositoryPath, { recursive: true });
  fs.writeFileSync(path.join(repositoryPath, "README.md"), "scripted source\n");
  execFileSync("git", ["init", "-b", "main"], { cwd: repositoryPath, env: gitEnv });
  execFileSync("git", ["add", "."], { cwd: repositoryPath, env: gitEnv });
  execFileSync("git", ["commit", "-m", "initial source"], { cwd: repositoryPath, env: gitEnv });
  execFileSync("git", ["init", "--bare", "-b", "main", originPath], { env: gitEnv });
  execFileSync("git", ["remote", "add", "origin", originPath], {
    cwd: repositoryPath,
    env: gitEnv,
  });
  execFileSync("git", ["push", "--set-upstream", "origin", "main"], {
    cwd: repositoryPath,
    env: gitEnv,
  });
  return { repositoryPath };
}

async function waitForWorkspaceReady(
  apiClient: ApiClient,
  taskId: string,
  sessionId: string,
): Promise<void> {
  await expect
    .poll(
      async () => {
        const { sessions } = await apiClient.listTaskSessions(taskId);
        return sessions.find((session) => session.id === sessionId)?.workspace_path ?? "";
      },
      { timeout: 60_000, message: "task workspace should be ready before attaching a source" },
    )
    .toMatch(/\S/);
}

async function waitForCompletedTurns(
  apiClient: ApiClient,
  sessionId: string,
  message: string,
): Promise<void> {
  await expect
    .poll(
      async () => {
        const { turns } = await apiClient.listSessionTurns(sessionId);
        return turns.length > 0 && turns.every((turn) => Boolean(turn.completed_at));
      },
      { timeout: 60_000, message },
    )
    .toBe(true);
}

function setupMessages(
  messages: Awaited<ReturnType<ApiClient["listSessionMessages"]>>["messages"],
) {
  return messages.filter(
    (message) =>
      message.type === "script_execution" &&
      (message.metadata as { script_type?: string } | undefined)?.script_type === "setup",
  );
}

function setupStatus(message: { metadata?: Record<string, unknown> }): string | undefined {
  return (message.metadata as { status?: string } | undefined)?.status;
}

async function attachmentWorktreePath(
  apiClient: ApiClient,
  taskId: string,
  sessionId: string,
): Promise<string> {
  await expect
    .poll(
      async () => {
        const { sessions } = await apiClient.listTaskSessions(taskId);
        const session = sessions.find((candidate) => candidate.id === sessionId);
        return (
          session?.worktrees?.find((worktree) => worktree.worktree_path?.endsWith(WORKTREE_NAME))
            ?.worktree_path ?? ""
        );
      },
      { timeout: 60_000, message: "attached repository worktree should be projected" },
    )
    .toMatch(/\S/);
  const { sessions } = await apiClient.listTaskSessions(taskId);
  const session = sessions.find((candidate) => candidate.id === sessionId);
  return (
    session?.worktrees?.find((worktree) => worktree.worktree_path?.endsWith(WORKTREE_NAME))
      ?.worktree_path ?? ""
  );
}

// @covers AC-TASKS-ATTACH-WORKSPACE-SOURCES-001.2, AC-TASKS-ATTACH-WORKSPACE-SOURCES-001.5
test.describe("Workspace source setup script", () => {
  test("attaches a script-enabled repository and keeps the session conversational", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    test.setTimeout(180_000);
    const { repositoryPath } = createScriptedRepository(backend.tmpDir);
    const repository = await apiClient.createRepository(
      seedData.workspaceId,
      repositoryPath,
      "main",
      { name: REPOSITORY_NAME },
    );
    await apiClient.updateRepository(repository.id, {
      setup_script: `printf '%s\\n' ${SETUP_MARKER} > setup-marker.txt && printf '%s\\n' ${SETUP_MARKER}`,
    });

    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Scripted workspace source",
      seedData.agentProfileId,
      {
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
        executor_profile_id: seedData.worktreeExecutorProfileId,
      },
    );
    if (!task.session_id) throw new Error("task creation did not return a session id");
    await waitForWorkspaceReady(apiClient, task.id, task.session_id);
    await waitForSessionDone(
      apiClient,
      task.id,
      task.session_id,
      "the initial task turn should finish before attaching a scripted source",
      60_000,
    );
    await waitForCompletedTurns(
      apiClient,
      task.session_id,
      "the initial task turn should be persisted and complete before attaching a source",
    );

    const ws = watchWs(testPage);
    await testPage.goto(`/t/${task.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await session.waitForChatIdle({ timeout: 60_000 });
    await session.clickTab("Files");

    const createMenu = testPage.getByTestId("files-create-menu");
    await expect(createMenu).toBeEnabled();
    await createMenu.click();
    await testPage.getByRole("menuitem", { name: "Add repositories or folders" }).click();
    const dialog = testPage.getByTestId("add-workspace-sources-dialog");
    await expect(dialog).toBeVisible();
    await dialog.getByRole("button", { name: "Add repository", exact: true }).click();
    await testPage.getByRole("menuitem", { name: "Workspace repository" }).click();
    const row = dialog.getByTestId("workspace-source-row").first();
    await row.getByTestId("repo-chip-trigger").click();
    await testPage
      .getByPlaceholder("Search repositories...", { exact: true })
      .fill(REPOSITORY_NAME);
    await testPage.keyboard.press("Enter");
    await expect(row.getByTestId("branch-chip-trigger")).toContainText("main");
    await dialog.getByRole("radio", { name: /Inside the current/ }).click();
    const submit = dialog.getByTestId("add-workspace-sources-submit");
    await expect(submit).toBeEnabled();

    const attachmentResponse = waitForHttp(testPage, "POST", WORKSPACE_SOURCES_PATH, {
      timeout: 90_000,
    });
    const adopted = ws.waitForEvent("session.workspace_sources.updated", {
      where: (payload) => payload.session_id === task.session_id,
    });
    await submit.click();
    const response = await attachmentResponse;
    const failureBody = response.ok() ? "" : await response.text();
    expect(
      response.ok(),
      `workspace source request returned ${response.status()}: ${failureBody}`,
    ).toBe(true);
    await adopted;
    await expect(dialog).not.toBeVisible();

    await expect
      .poll(
        async () => {
          const { messages } = await apiClient.listSessionMessages(task.session_id!);
          const setup = setupMessages(messages);
          return setup.length === 1 &&
            setupStatus(setup[0]!) === "exited" &&
            setup[0]!.content.includes(SETUP_MARKER)
            ? setup[0]!.turn_id
            : "";
        },
        { timeout: 30_000, message: "setup script output should be persisted" },
      )
      .toMatch(/\S/);
    const { messages } = await apiClient.listSessionMessages(task.session_id);
    const setupMessage = setupMessages(messages)[0]!;
    const turnsAfterAttach = await apiClient.listSessionTurns(task.session_id);
    expect(turnsAfterAttach.turns.every((turn) => Boolean(turn.completed_at))).toBe(true);
    expect(
      turnsAfterAttach.turns.find((turn) => turn.id === setupMessage.turn_id)?.completed_at,
    ).toBeTruthy();

    const worktreePath = await attachmentWorktreePath(apiClient, task.id, task.session_id);
    expect(fs.readFileSync(path.join(worktreePath, "setup-marker.txt"), "utf8")).toContain(
      SETUP_MARKER,
    );

    await session.clickTab("Files");
    const repositoryNode = await session.fileTree.waitForFileTreeNode(WORKTREE_NAME);
    await repositoryNode.click();
    const markerRow = await session.fileTree.waitForFileTreeNode(
      `${WORKTREE_NAME}/setup-marker.txt`,
    );
    await markerRow.click();
    const editor = testPage.locator(".monaco-editor:visible").first();
    await expect(editor).toBeVisible({ timeout: 15_000 });
    await expect(editor.locator(".view-lines")).toContainText(SETUP_MARKER);

    await session.clickSessionChatTab();
    await testPage.reload();
    await session.waitForLoad();
    await session.clickTab("Files");
    await expect(await session.fileTree.waitForFileTreeNode(WORKTREE_NAME)).toBeVisible();
    const persisted = await apiClient.listSessionMessages(task.session_id);
    expect(setupStatus(setupMessages(persisted.messages)[0]!)).toBe("exited");

    const turnsBeforePrompt = new Set(
      (await apiClient.listSessionTurns(task.session_id)).turns.map((turn) => turn.id),
    );
    await session.waitForChatIdle({ timeout: 30_000 });
    await session.sendMessage("/e2e:simple-message");
    await expect
      .poll(
        async () => {
          const { turns } = await apiClient.listSessionTurns(task.session_id!);
          const next = turns.find((turn) => !turnsBeforePrompt.has(turn.id) && turn.completed_at);
          if (!next) return "";
          const { messages: after } = await apiClient.listSessionMessages(task.session_id!);
          return (
            after.find(
              (message) =>
                message.turn_id === next.id &&
                message.author_type === "agent" &&
                message.content.includes("simple mock response"),
            )?.id ?? ""
          );
        },
        {
          timeout: 90_000,
          message: "the next conversational turn should complete with a response",
        },
      )
      .toMatch(/\S/);

    await session.clickTab("Files");
    await createMenu.click();
    const reopened = testPage.getByRole("menuitem", { name: "Add repositories or folders" });
    await expect(reopened).toBeEnabled();
    await testPage.keyboard.press("Escape");
    await expect(reopened).not.toBeVisible();
  });
});
