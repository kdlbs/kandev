import { test, expect } from "../../fixtures/test-base";
import { GITLAB_PROJECT, gitLabMR } from "../../helpers/gitlab";
import { SessionPage } from "../../pages/session-page";

// This spec covers push-detection auto-link — the feature that discovers a
// merge request opened OUTSIDE Kandev's own "Create MR" action (e.g. `glab mr
// create` run as a raw shell command, or an MR opened manually via the
// GitLab web UI) purely by observing a `git push`, and links it to the task
// without any Kandev UI action creating or naming the MR. This is distinct
// from gitlab-mr-creation.spec.ts, which exercises Kandev's own Create-MR
// button (a different code path — AssociateExistingMRByURL — that already
// worked before push-detection auto-link existed).
test.describe("GitLab MR push-detection auto-link", () => {
  test("links an externally-opened MR after a raw git push, without using Create MR", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    // Sequential assertion budget: 45s + 45s + 150s + 30s = 270s. Give it
    // headroom above that so the documented ~90s push-detection retry window
    // can actually run to completion instead of the test timing out first.
    test.setTimeout(330_000);
    const remoteURL = `${backend.baseUrl}/${GITLAB_PROJECT}.git`;
    await apiClient.configureGitLab(seedData.workspaceId, backend.baseUrl);
    await apiClient.configureGitLabRepositoryRemote(seedData.repositoryId, remoteURL);
    await apiClient.updateRepository(seedData.repositoryId, {
      provider: "gitlab",
      provider_host: backend.baseUrl,
      provider_owner: "platform",
      provider_name: "kandev",
      pull_before_worktree: false,
    });

    // First turn: commit a change but do NOT push yet, so the branch has no
    // upstream and no MR could exist for it — mirrors the ordinary "agent
    // made a change" state before any push happens.
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Push-detection GitLab auto-link",
      seedData.agentProfileId,
      {
        description: "/e2e:diff-update-setup",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
        executor_profile_id: seedData.worktreeExecutorProfileId,
      },
    );
    if (!task.session_id) throw new Error("Push-detection auto-link task did not return a session");

    await testPage.goto(`/t/${task.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await expect(
      session.chat.getByText("diff-update-setup complete", { exact: false }),
    ).toBeVisible({ timeout: 45_000 });

    // Learn the worktree's actual branch, then seed an MR for that branch as
    // already open on GitLab — simulating someone opening it outside Kandev
    // (`glab mr create`, or the GitLab web UI) before the push lands. Seeded
    // ahead of the push so the very first FindMRByBranch search after push
    // detection fires can find it, the common real-world case.
    const sessions = await apiClient.listTaskSessions(task.id);
    const branch = sessions.sessions.find((s) => s.id === task.session_id)?.worktree_branch;
    if (!branch) throw new Error("Could not resolve the session's worktree branch");

    const mrIID = 55;
    await apiClient.mockGitLabAddMRs(seedData.workspaceId, GITLAB_PROJECT, [
      gitLabMR(mrIID, "Externally opened GitLab MR", {
        head_branch: branch,
        // Seeded MRs are stored verbatim by the mock (no GitLab "opened" ->
        // "open" normalization the real client applies), and FindMRByBranch
        // only matches the normalized "open" value.
        state: "open",
        url: `${backend.baseUrl}/${GITLAB_PROJECT}/-/merge_requests/${mrIID}`,
        web_url: `${backend.baseUrl}/${GITLAB_PROJECT}/-/merge_requests/${mrIID}`,
      }),
    ]);

    // Second turn: a raw `git push`, with no Kandev "Create MR" dialog or
    // button ever used. This is the exact trigger push-detection auto-link
    // depends on.
    await session.clickSessionChatTab();
    await session.sendMessage("/e2e:push-current-branch");
    await expect(
      session.chat.getByText("push-current-branch complete", { exact: false }),
    ).toBeVisible({ timeout: 45_000 });

    // Push detection has to observe the push (agentctl git-status polling),
    // then search for and link the MR — give it the full retry window
    // (up to ~90s) plus polling latency.
    const mrButton = testPage.getByTestId("mr-topbar-button");
    await expect(mrButton).toHaveAttribute("data-mr-iid", String(mrIID), { timeout: 150_000 });

    await testPage.reload();
    await expect(testPage.getByTestId("mr-topbar-button")).toHaveAttribute(
      "data-mr-iid",
      String(mrIID),
      { timeout: 30_000 },
    );
  });
});
