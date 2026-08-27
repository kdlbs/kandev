import { test, expect } from "../../fixtures/test-base";
import { SessionPage } from "../../pages/session-page";

// GitHub twin of gitlab-mr-push-autolink.spec.ts. Covers push-detection
// auto-link — the feature that discovers a PR opened OUTSIDE Kandev's own
// "Create PR" action (e.g. `gh pr create` run as a raw shell command, or a
// PR opened manually via the GitHub web UI) purely by observing a `git
// push`, and links it to the task without any Kandev UI action creating or
// naming the PR. Existing coverage in pr-detection.spec.ts only exercises
// the background poller and direct mock association (mockGitHubAssociateTaskPR)
// — neither goes through detectPushAndAssociatePR, the code path this
// covers. trackPushAndAssociatePR/shouldFirePushDetection/checkGitChanges
// are shared between GitHub and GitLab, so this pins the same fix the
// GitLab spec does, from the other provider's entry point.
test.describe("GitHub PR push-detection auto-link", () => {
  test("links an externally-opened PR after a raw git push, without using Create PR", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    // Sequential assertion budget: 45s + 45s + 150s + 30s = 270s. Give it
    // headroom above that so the documented ~90s push-detection retry window
    // can actually run to completion instead of the test timing out first.
    test.setTimeout(330_000);
    // The default seeded repository has no git "origin" configured — nothing
    // in the existing PR-detection coverage ever pushes for real. The path
    // segment here isn't provider-specific: the test-harness endpoint checks
    // it against KANDEV_E2E_GITLAB_REMOTE_URL verbatim regardless of which
    // provider's test uses it, so this reuses the same fixed E2E remote path
    // gitlab-mr-push-autolink.spec.ts configures.
    await apiClient.configureGitLabRepositoryRemote(
      seedData.repositoryId,
      `${backend.baseUrl}/platform/kandev.git`,
    );
    await apiClient.updateRepository(seedData.repositoryId, {
      provider: "github",
      provider_host: "https://github.com",
      provider_owner: "testorg",
      provider_name: "testrepo",
      pull_before_worktree: false,
    });
    await apiClient.mockGitHubReset();
    await apiClient.mockGitHubSetUser("test-user");

    // First turn: commit a change but do NOT push yet, so the branch has no
    // upstream and no PR could exist for it — mirrors the ordinary "agent
    // made a change" state before any push happens.
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Push-detection GitHub auto-link",
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

    // Learn the worktree's actual branch, then seed a PR for that branch as
    // already open on GitHub — simulating someone opening it outside Kandev
    // (`gh pr create`, or the GitHub web UI) before the push lands. Seeded
    // ahead of the push so the very first FindPRByBranch search after push
    // detection fires can find it, the common real-world case.
    const sessions = await apiClient.listTaskSessions(task.id);
    const branch = sessions.sessions.find((s) => s.id === task.session_id)?.worktree_branch;
    if (!branch) throw new Error("Could not resolve the session's worktree branch");

    const prNumber = 77;
    await apiClient.mockGitHubAddPRs([
      {
        number: prNumber,
        title: "Externally opened GitHub PR",
        state: "open",
        head_branch: branch,
        base_branch: "main",
        author_login: "test-user",
        repo_owner: "testorg",
        repo_name: "testrepo",
        additions: 5,
        deletions: 1,
      },
    ]);

    // Second turn: a raw `git push`, with no Kandev "Create PR" dialog or
    // button ever used. This is the exact trigger push-detection auto-link
    // depends on.
    await session.clickSessionChatTab();
    await session.sendMessage("/e2e:push-current-branch");
    await expect(
      session.chat.getByText("push-current-branch complete", { exact: false }),
    ).toBeVisible({ timeout: 45_000 });

    // Push detection has to observe the push (agentctl git-status polling),
    // then search for and link the PR — give it the full retry window
    // (up to ~90s) plus polling latency.
    const prButton = session.prTopbarButton();
    await expect(prButton).toHaveAttribute("data-pr-number", String(prNumber), {
      timeout: 150_000,
    });

    await testPage.reload();
    await expect(session.prTopbarButton()).toHaveAttribute("data-pr-number", String(prNumber), {
      timeout: 30_000,
    });
  });
});
