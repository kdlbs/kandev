// One-off manual seed script for the "Scope GitLab MR automation switches
// per MR" task's isolated environment (STEP 3 of the Work phase). Not part
// of the automated test suite — run with `npx tsx` against a manually
// launched backend. Seeds a workspace/workflow/repo and a task with two
// linked GitLab MRs so the multi-MR dropdown independence UI can be
// exercised by hand.
import { execFileSync } from "node:child_process";
import fs from "node:fs";
import path from "node:path";
import { ApiClient } from "./helpers/api-client";
import { seedGitLabMRData, GITLAB_HOST, GITLAB_PROJECT } from "./helpers/gitlab";
import { dwell } from "./helpers/causal-waits";

const BASE_URL = process.env.KANDEV_BASE_URL || "http://localhost:18500";
// Resolved from this file's own location so the script works in any checkout;
// `__dirname` matches how global-setup.ts and the e2e helpers locate paths.
const REPO_ROOT =
  process.env.KANDEV_SEED_REPO_ROOT || path.resolve(__dirname, "../../backend/.manual-env/repos");

async function main() {
  const apiClient = new ApiClient(BASE_URL);

  const workspace = await apiClient.createWorkspace("GitLab MR Automation Demo");
  const workflow = await apiClient.createWorkflow(workspace.id, "Demo Workflow", "simple");
  const { steps } = await apiClient.listWorkflowSteps(workflow.id);
  const sorted = steps.sort((a, b) => a.position - b.position);
  const startStep = sorted.find((s) => s.is_start_step) ?? sorted[0];

  const remoteDir = path.join(REPO_ROOT, "e2e-remote.git");
  const repoDir = path.join(REPO_ROOT, "e2e-repo");
  fs.mkdirSync(REPO_ROOT, { recursive: true });
  if (!fs.existsSync(remoteDir)) {
    execFileSync("git", ["init", "--bare", "-b", "main", remoteDir]);
    fs.mkdirSync(repoDir, { recursive: true });
    execFileSync("git", ["init", "-b", "main"], { cwd: repoDir });
    execFileSync(
      "git",
      [
        "-c",
        "user.name=Demo",
        "-c",
        "user.email=demo@test.local",
        "commit",
        "--allow-empty",
        "-m",
        "init",
      ],
      { cwd: repoDir },
    );
    execFileSync("git", ["remote", "add", "origin", `file://${remoteDir}`], { cwd: repoDir });
    execFileSync("git", ["push", "origin", "main"], { cwd: repoDir });
  }
  const repo = await apiClient.createRepository(workspace.id, repoDir);

  let agentProfileId: string | undefined;
  const deadline = Date.now() + 30_000;
  while (Date.now() < deadline) {
    const { agents } = await apiClient.listAgents();
    agentProfileId = agents[0]?.profiles[0]?.id;
    if (agentProfileId) break;
    await dwell(
      250,
      "poll-interval",
      "polling listAgents() until the backend's initial agent setup completes",
    );
  }
  if (!agentProfileId) throw new Error("no agent profile available after 30s");

  const iidA = 220;
  const iidB = 221;
  await apiClient.configureGitLab(workspace.id, GITLAB_HOST);
  await seedGitLabMRData(apiClient, workspace.id, iidA, "Fix pagination bug");
  await seedGitLabMRData(apiClient, workspace.id, iidB, "Add dark mode toggle");
  await apiClient.updateRepository(repo.id, {
    provider: "gitlab",
    provider_host: GITLAB_HOST,
    provider_owner: "platform",
    provider_name: "kandev",
  });

  const task = await apiClient.createTaskWithAgent(
    workspace.id,
    "Demo: two linked GitLab MRs",
    agentProfileId,
    {
      description: "Demo task seeded for manual verification of per-MR automation scoping.",
      workflow_id: workflow.id,
      workflow_step_id: startStep.id,
      repository_ids: [repo.id],
    },
  );
  await apiClient.linkTaskGitLabMR(workspace.id, {
    task_id: task.id,
    repository_id: repo.id,
    mr_url: `${GITLAB_HOST}/${GITLAB_PROJECT}/-/merge_requests/${iidA}`,
  });
  await apiClient.linkTaskGitLabMR(workspace.id, {
    task_id: task.id,
    repository_id: repo.id,
    mr_url: `${GITLAB_HOST}/${GITLAB_PROJECT}/-/merge_requests/${iidB}`,
  });

  console.log("Seed complete.");
  console.log(`Workspace: ${workspace.id}`);
  console.log(`Task: ${task.id}`);
  console.log(`Open: ${BASE_URL}/t/${task.id}`);
}

main().catch((err) => {
  console.error(err);
  process.exit(1);
});
