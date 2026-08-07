import { execSync } from "node:child_process";
import fs from "node:fs";
import type { Page } from "@playwright/test";

import { test, expect } from "../../fixtures/test-base";
import { SessionPage } from "../../pages/session-page";
import { makeGitEnv } from "../../helpers/git-helper";
import type { ApiClient } from "../../helpers/api-client";
import {
  routeMainWebSocketWithDelayedActionResponse,
  routeMainWebSocketWithFailedActionResponse,
} from "../../helpers/ws-drop";

type GitSnapshotResponse = {
  // The backend marshals a nil Go slice as JSON null, not [], when no
  // snapshot rows exist yet — never assume this is always an array.
  snapshots: Array<{ branch?: string; ahead?: number; files?: Record<string, unknown> }> | null;
};

const DONE_STATES = ["COMPLETED", "WAITING_FOR_INPUT", "IDLE"];

async function waitForSessionDone(apiClient: ApiClient, taskId: string) {
  await expect
    .poll(
      async () => {
        const { sessions } = await apiClient.listTaskSessions(taskId);
        return DONE_STATES.includes(sessions[0]?.state ?? "");
      },
      { timeout: 60_000, message: "Waiting for the agent session to finish" },
    )
    .toBe(true);
}

/**
 * Opens the delete confirmation dialog via the tab's right-click context
 * menu. The tab's close (X) button (`showDeleteOnClose` in session-tab.tsx)
 * only renders once a task has more than one session — the context menu's
 * "Delete" item has no such gate, so it is the reliable way to reach the
 * shared `DeleteSessionDialog` for a single-session task.
 */
async function openDeleteConfirmDialog(session: SessionPage, sessionId: string) {
  await session.sessionTabBySessionId(sessionId).click({ button: "right" });
  await session.contextMenuItem("Delete").click();
}

/**
 * Whether `branch` is currently registered as a live git worktree in
 * `repoPath` (via `git worktree list --porcelain`). Deleting a task's sole
 * session can auto-respawn a replacement (the task's workflow step allows
 * auto-start — see EnsureSession), and that new session's worktree can
 * legitimately land at the exact same directory the reclaimed one occupied
 * (the path is derived only from task dir + repo name, not worktree ID or
 * session ID). Checking raw path existence can't tell "leaked" apart from
 * "reclaimed, then legitimately reoccupied by an unrelated worktree" — git's
 * own worktree/branch registration can.
 */
function isBranchCheckedOutAsWorktree(
  repoPath: string,
  env: NodeJS.ProcessEnv,
  branch: string,
): boolean {
  const output = execSync("git worktree list --porcelain", {
    cwd: repoPath,
    env,
    encoding: "utf8",
  });
  return output.split("\n").includes(`branch refs/heads/${branch}`);
}

/**
 * Collects incoming WS response frames for `session.git.snapshots`. Used to
 * prove the delete dialog's warning fetch actually round-tripped before
 * asserting the warning lines are absent — an absence check with no such
 * proof would pass vacuously if it ran before the async fetch resolved.
 */
function captureGitSnapshotResponses(page: Page): { count: number } {
  const state = { count: 0 };
  page.on("websocket", (ws) => {
    ws.on("framereceived", (event) => {
      const payload = event.payload;
      if (typeof payload !== "string" || !payload.includes('"action":"session.git.snapshots"')) {
        return;
      }
      state.count += 1;
    });
  });
  return state;
}

test.describe("Session delete reclaims its worktree", () => {
  test("deleting a session that exclusively holds a worktree removes it from disk and preserves the branch", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Session delete reclaim",
      seedData.agentProfileId,
      {
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
        executor_profile_id: seedData.worktreeExecutorProfileId,
      },
    );
    if (!task.session_id) throw new Error("expected task creation to auto-start a session");
    const sessionId = task.session_id;
    await waitForSessionDone(apiClient, task.id);

    const { sessions } = await apiClient.listTaskSessions(task.id);
    const seeded = sessions.find((s) => s.id === sessionId);
    const worktreePath = seeded?.worktree_path;
    const worktreeBranch = seeded?.worktree_branch;
    if (!worktreePath || !worktreeBranch) {
      throw new Error("expected a worktree-backed session with a path and branch");
    }
    expect(fs.existsSync(worktreePath)).toBe(true);

    await testPage.goto(`/t/${task.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();

    await openDeleteConfirmDialog(session, sessionId);
    await session.alertDialog().getByRole("button", { name: "Delete" }).click();

    // Session and its tab are gone from the UI.
    await expect(session.sessionTabBySessionId(sessionId)).toHaveCount(0, {
      timeout: 15_000,
    });
    await expect
      .poll(
        async () => {
          const { sessions: after } = await apiClient.listTaskSessions(task.id);
          return after.some((s) => s.id === sessionId);
        },
        { timeout: 15_000, message: "Waiting for the session row to be deleted" },
      )
      .toBe(false);

    // The durable cleanup job runs asynchronously after {"success":true} —
    // poll for the ORIGINAL worktree to disappear from git's own worktree
    // registration. This is the exact bug being fixed: before the fix this
    // worktree would never be removed. Raw path existence isn't a reliable
    // signal here — the task's workflow step allows auto-start, so deleting
    // its sole session can auto-respawn a replacement session whose new
    // worktree legitimately reoccupies the same directory (see
    // isBranchCheckedOutAsWorktree's doc comment).
    const gitEnv = makeGitEnv(backend.tmpDir);
    await expect
      .poll(() => isBranchCheckedOutAsWorktree(seedData.repositoryPath, gitEnv, worktreeBranch), {
        timeout: 45_000,
        message: `Waiting for worktree directory ${worktreePath} (branch ${worktreeBranch}) to be reclaimed`,
      })
      .toBe(false);

    // session.delete must never run `git branch -D`: the branch stays
    // resolvable in the source repository after the worktree is gone.
    const branchSha = execSync(`git rev-parse --verify ${JSON.stringify(worktreeBranch)}`, {
      cwd: seedData.repositoryPath,
      env: gitEnv,
    })
      .toString()
      .trim();
    expect(branchSha).toMatch(/^[0-9a-f]{7,40}$/);
  });

  test("shows uncommitted-file and unpushed-commit counts, and no longer claims only conversation history is removed", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    // Environment prep + agent turn + snapshot persistence + UI interaction
    // is more steps than the default 60s budget comfortably covers under
    // load — matches the 120s budget session-tab-management.spec.ts uses
    // for similarly multi-step session flows.
    test.setTimeout(120_000);
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Session delete warning counts",
      seedData.agentProfileId,
      {
        description: "/e2e:diff-update-setup",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
        executor_profile_id: seedData.worktreeExecutorProfileId,
      },
    );
    if (!task.session_id) throw new Error("expected task creation to auto-start a session");
    const sessionId = task.session_id;

    await testPage.goto(`/t/${task.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await expect(
      session.chat.getByText("diff-update-setup complete", { exact: false }),
    ).toBeVisible({ timeout: 60_000 });

    // Turn completion and snapshot persistence are separate ordered writes
    // (mirrors sidebar-diff-stats.spec.ts) — wait for the durable snapshot
    // the dialog reads before opening it.
    await expect
      .poll(
        async () => {
          const response = await apiClient.wsRequest<GitSnapshotResponse>("session.git.snapshots", {
            session_id: sessionId,
            limit: 1,
          });
          const snapshot = (response.snapshots ?? [])[0];
          return (snapshot?.ahead ?? 0) > 0 && Object.keys(snapshot?.files ?? {}).length > 0;
        },
        { timeout: 60_000, message: "Waiting for a dirty git snapshot to persist" },
      )
      .toBe(true);

    // Settle on the backend's terminal session state too — mirrors test 1/3,
    // which wait for this before interacting with the tab, rather than
    // racing the tab's right-click against any trailing frontend state
    // update still in flight right after the completion message renders.
    await waitForSessionDone(apiClient, task.id);

    // A dirty worktree auto-inserts the Changes panel during the initial
    // mount, which can race the effect that swaps the generic "chat"
    // placeholder tab for the real session-scoped tab (session-tab.tsx's
    // data-testid depends on that swap having completed). Reloading forces a
    // fresh mount once the session/task state is already fully hydrated,
    // side-stepping the race rather than depending on its timing.
    await testPage.reload();
    await session.waitForLoad();

    await openDeleteConfirmDialog(session, sessionId);
    const dialog = session.alertDialog();
    await expect(dialog).toBeVisible();

    const uncommittedWarning = dialog.getByTestId("session-delete-uncommitted-warning");
    const unpushedWarning = dialog.getByTestId("session-delete-unpushed-warning");
    // Both counts must be visible before the confirm control is ever used.
    await expect(uncommittedWarning).toBeVisible({ timeout: 10_000 });
    await expect(unpushedWarning).toBeVisible();
    await expect(uncommittedWarning).toHaveText(/[1-9]\d*/);
    await expect(unpushedWarning).toHaveText(/[1-9]\d*/);
    const confirmButton = dialog.getByRole("button", { name: "Delete" });
    await expect(confirmButton).toBeEnabled();

    // The revised copy — distinguishing phrase from the new EN string;
    // this fails if the old "only ... conversation history" copy regresses.
    await expect(dialog).toContainText("If no other session is using its workspace");
    await expect(dialog).not.toContainText(
      "This will permanently delete the conversation history with this session.",
    );

    // Cancelling must not delete the session.
    await dialog.getByRole("button", { name: "Cancel" }).click();
    await expect(dialog).toBeHidden();
    await expect(session.sessionTabBySessionId(sessionId)).toBeVisible();
    const { sessions: stillThere } = await apiClient.listTaskSessions(task.id);
    expect(stillThere.some((s) => s.id === sessionId)).toBe(true);
  });

  test("shows no warning for a session that is clean and level with its remote", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Session delete clean session",
      seedData.agentProfileId,
      {
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
        executor_profile_id: seedData.worktreeExecutorProfileId,
      },
    );
    if (!task.session_id) throw new Error("expected task creation to auto-start a session");
    const sessionId = task.session_id;
    await waitForSessionDone(apiClient, task.id);

    // Must attach before navigation: page.on("websocket") only fires for
    // connections opened after the listener is registered.
    const snapshotResponses = captureGitSnapshotResponses(testPage);
    await testPage.goto(`/t/${task.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();

    await openDeleteConfirmDialog(session, sessionId);
    const dialog = session.alertDialog();
    await expect(dialog).toBeVisible();

    // Prove the dialog's warning fetch actually round-tripped before
    // asserting absence — otherwise this check could pass vacuously before
    // the async response ever arrives.
    await expect
      .poll(() => snapshotResponses.count, {
        timeout: 10_000,
        message: "Waiting for the session.git.snapshots response the dialog issued",
      })
      .toBeGreaterThan(0);

    await expect(dialog.getByTestId("session-delete-uncommitted-warning")).toHaveCount(0);
    await expect(dialog.getByTestId("session-delete-unpushed-warning")).toHaveCount(0);
  });

  test("disables the confirm control until the warning fetch actually resolves, then enables it", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Session delete confirm gating",
      seedData.agentProfileId,
      {
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
        executor_profile_id: seedData.worktreeExecutorProfileId,
      },
    );
    if (!task.session_id) throw new Error("expected task creation to auto-start a session");
    const sessionId = task.session_id;
    await waitForSessionDone(apiClient, task.id);

    // Must attach before navigation: routeWebSocket only affects connections
    // opened after it is registered.
    const delayed = await routeMainWebSocketWithDelayedActionResponse(
      testPage,
      "session.git.snapshots",
    );
    await testPage.goto(`/t/${task.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();

    await openDeleteConfirmDialog(session, sessionId);
    const dialog = session.alertDialog();
    await expect(dialog).toBeVisible();

    // The dialog issues the fetch immediately on open, but the response is
    // held — this is the real WS round trip, not a mocked hook return value,
    // so it also proves the request actually went out before asserting the
    // gated state rather than racing an assertion against nothing in flight.
    const confirmButton = dialog.getByRole("button", { name: "Delete" });
    await expect(confirmButton).toBeDisabled();

    delayed.release();

    await expect(confirmButton).toBeEnabled({ timeout: 10_000 });

    // The gate does not block the request itself, and once armed correctly,
    // the button really is clickable — not merely enabled in markup.
    await confirmButton.click();
    await expect(session.sessionTabBySessionId(sessionId)).toHaveCount(0, { timeout: 15_000 });
  });

  test("still enables the confirm control after the warning fetch fails, rather than stranding it disabled", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Session delete gating survives fetch failure",
      seedData.agentProfileId,
      {
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
        executor_profile_id: seedData.worktreeExecutorProfileId,
      },
    );
    if (!task.session_id) throw new Error("expected task creation to auto-start a session");
    const sessionId = task.session_id;
    await waitForSessionDone(apiClient, task.id);

    // A failed warning fetch must not permanently lock the user out of
    // deleting the session — the gate exists to make sure the counts are
    // seen before confirming, not to hold the control hostage to a flaky
    // network call.
    await routeMainWebSocketWithFailedActionResponse(testPage, "session.git.snapshots");
    await testPage.goto(`/t/${task.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();

    await openDeleteConfirmDialog(session, sessionId);
    const dialog = session.alertDialog();
    await expect(dialog).toBeVisible();

    const confirmButton = dialog.getByRole("button", { name: "Delete" });
    await expect(confirmButton).toBeEnabled({ timeout: 10_000 });
  });
});
