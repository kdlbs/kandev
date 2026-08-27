import { execFileSync } from "node:child_process";
import path from "node:path";
import type { BackendContext } from "../../fixtures/backend";
import type { SeedData } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";
import { GitHelper, makeGitEnv } from "../../helpers/git-helper";

const targetURL = "https://github.com/upstream/widget.git";

type ForkPRComparisonOptions = {
  comparisonTargetAvailable?: boolean;
};

export async function seedForkPRComparisonTask(
  apiClient: ApiClient,
  seedData: SeedData,
  backend: BackendContext,
  options: ForkPRComparisonOptions = {},
) {
  const gitEnv = makeGitEnv(backend.tmpDir);
  const suffix = `${process.pid}-${Date.now()}`;
  const targetRemoteDir = path.join(backend.tmpDir, "repos", `upstream-${suffix}.git`);
  const targetWorktreeDir = path.join(backend.tmpDir, "repos", `upstream-${suffix}`);
  const localGit = new GitHelper(seedData.repositoryPath, gitEnv);

  execFileSync("git", ["clone", "--bare", seedData.repositoryRemoteURL, targetRemoteDir], {
    env: gitEnv,
  });
  execFileSync("git", ["clone", `file://${targetRemoteDir}`, targetWorktreeDir], { env: gitEnv });
  const upstreamGit = new GitHelper(targetWorktreeDir, gitEnv);
  upstreamGit.createFile("upstream-only.txt", "upstream target commit\n");
  upstreamGit.stageAll();
  upstreamGit.commit("Advance upstream target");
  upstreamGit.exec("git push origin main");

  execFileSync("git", ["fetch", `file://${targetRemoteDir}`, "main"], {
    cwd: seedData.repositoryPath,
    env: gitEnv,
  });
  localGit.exec("git checkout -B feature/fork FETCH_HEAD");
  for (const [name, content] of [
    ["fork-one.txt", "fork change one\n"],
    ["fork-two.txt", "fork change two\n"],
    ["fork-three.txt", "fork change three\n"],
  ]) {
    localGit.createFile(name, content);
  }
  localGit.stageAll();
  const headSHA = localGit.commit("Add three fork contribution files");

  // agentctl fetches the credential-free provider URL. The test maps that URL
  // to its disposable upstream bare repository without changing origin. The
  // unavailable mode maps to a missing local path so it exercises the same
  // fail-closed path without depending on external network state.
  const comparisonTargetPath =
    options.comparisonTargetAvailable === false
      ? path.join(backend.tmpDir, "repos", `missing-upstream-${suffix}.git`)
      : targetRemoteDir;
  execFileSync(
    "git",
    ["config", "--global", `url.file://${comparisonTargetPath}.insteadOf`, targetURL],
    { cwd: backend.tmpDir, env: gitEnv },
  );

  await apiClient.updateRepository(seedData.repositoryId, {
    provider: "github",
    provider_repo_id: "42",
    provider_host: "https://github.com",
    provider_owner: "contributor",
    provider_name: "widget-fork",
  });

  await apiClient.mockGitHubReset();
  await apiClient.mockGitHubSetUser("contributor");
  await apiClient.mockGitHubAddPRs([
    {
      number: 1701,
      title: "Fork contribution",
      state: "open",
      head_branch: "feature/fork",
      head_sha: headSHA,
      base_branch: "main",
      author_login: "contributor",
      repo_owner: "upstream",
      repo_name: "widget",
      head_repo_id: 42,
      head_repo_owner: "contributor",
      head_repo_name: "widget-fork",
      head_repo_clone_url: "https://github.com/contributor/widget-fork.git",
      base_repo_id: 99,
      base_repo_owner: "upstream",
      base_repo_name: "widget",
      base_default_branch: "main",
      maintainer_can_modify: true,
      html_url: "https://github.com/upstream/widget/pull/1701",
    },
  ]);
  await apiClient.mockGitHubAddPRFiles("upstream", "widget", 1701, [
    { filename: "fork-one.txt", status: "added", additions: 1, deletions: 0 },
    { filename: "fork-two.txt", status: "added", additions: 1, deletions: 0 },
    { filename: "fork-three.txt", status: "added", additions: 1, deletions: 0 },
  ]);
  await apiClient.mockGitHubAddPRCommits("upstream", "widget", 1701, [
    {
      sha: headSHA,
      message: "Add three fork contribution files",
      author_login: "contributor",
      author_date: "2026-08-19T12:00:00Z",
      stats_available: true,
    },
  ]);

  const task = await apiClient.createTask(seedData.workspaceId, "Fork PR comparison target", {
    description: "/e2e:simple-message",
    agent_profile_id: seedData.agentProfileId,
    workflow_id: seedData.workflowId,
    workflow_step_id: seedData.startStepId,
    repositories: [
      {
        repository_id: seedData.repositoryId,
        base_branch: "main",
        checkout_branch: "feature/fork",
      },
    ],
  });

  await apiClient.associateGitHubTaskPR({
    workspace_id: seedData.workspaceId,
    task_id: task.id,
    repository_id: seedData.repositoryId,
    pr_url: "https://github.com/upstream/widget/pull/1701",
  });
  await apiClient.launchSession({
    task_id: task.id,
    agent_profile_id: seedData.agentProfileId,
    workflow_step_id: seedData.startStepId,
    prompt: "/e2e:simple-message",
  });

  return { task, headSHA };
}
