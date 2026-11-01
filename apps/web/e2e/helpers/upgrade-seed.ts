import { execFileSync } from "node:child_process";
import fs from "node:fs";
import path from "node:path";
import { expect } from "@playwright/test";
import type { UpgradeBackend } from "../fixtures/upgrade-backend";
import { captureUpgradeState, saveUpgradeSnapshot } from "./upgrade-oracle";
import { waitForUpgradeTurn } from "./upgrade-api";

export type UpgradeSeed = Awaited<ReturnType<typeof seedUpgrade>>;
export async function seedUpgrade(backend: UpgradeBackend) {
  const api = backend.api;
  const workspace = await api.createWorkspace("Upgrade preserved workspace");
  const workflow = await api.createWorkflow(workspace.id, "Upgrade preserved workflow", "simple");
  const { steps } = await api.listWorkflowSteps(workflow.id);
  steps.sort((a, b) => a.position - b.position);
  expect(steps.length).toBeGreaterThanOrEqual(3);
  for (const step of steps)
    await api.updateWorkflowStep(step.id, {
      events: { on_enter: [], on_turn_complete: [], on_exit: [] },
      complete_task_on_enter: step.id === steps.at(-1)!.id,
    });
  const agents = (await api.listAgents()).agents;
  const mock = agents.find((a) => a.name === "mock-agent");
  if (!mock?.profiles?.[0])
    throw new Error(`baseline capability: no mock profile: ${JSON.stringify(agents)}`);
  const profile = await api.createAgentProfile(mock.id, "Upgrade preserved profile", {
    model: "mock-fast",
    auto_approve: true,
  });
  const reviewProfile = await api.createAgentProfile(mock.id, "Upgrade review profile", {
    model: "mock-fast",
    auto_approve: true,
  });
  await api.updateWorkflowStep(steps[0].id, {
    agent_profile_id: profile.id,
    profile_session_end_policy: "complete",
  });
  await api.updateWorkflowStep(steps[1].id, {
    agent_profile_id: reviewProfile.id,
    prompt: "/e2e:simple-message",
    profile_session_start_policy: "new",
    profile_session_end_policy: "complete",
    events: { on_enter: [{ type: "auto_start_agent" }] },
  });
  const executor = (await api.listExecutors()).executors.find((e) => e.type === "worktree");
  if (!executor) throw new Error("baseline capability: no worktree executor");
  const executorProfile = await api.createExecutorProfile(
    executor.id,
    "Upgrade preserved executor profile",
  );
  const repositoryPath = path.join(backend.root, "repository");
  const origin = path.join(backend.root, "origin.git");
  fs.mkdirSync(repositoryPath);
  const git = (args: string[], cwd = repositoryPath) =>
    execFileSync("git", args, { cwd, env: backend.env, stdio: "pipe" });
  git(["init", "--bare", "-b", "main", origin]);
  git(["init", "-b", "main"]);
  fs.writeFileSync(path.join(repositoryPath, "upgrade-preserved.txt"), "UPGRADE_PRESERVED_FILE\n");
  git(["add", "."]);
  git(["commit", "-m", "Seed upgrade repository"]);
  git(["remote", "add", "origin", `file://${origin}`]);
  git(["push", "-u", "origin", "main"]);
  const repository = await api.createRepository(workspace.id, repositoryPath);
  await api.saveUserSettings({
    workspace_id: workspace.id,
    workflow_filter_id: workflow.id,
    enable_preview_on_click: false,
    confirm_task_archive: false,
  });
  const options = {
    workflow_id: workflow.id,
    workflow_step_id: steps[0].id,
    repository_ids: [repository.id],
    executor_profile_id: executorProfile.id,
  };
  const tasks: Record<string, { id: string; session_id?: string }> = {};
  for (const kind of ["pending", "waiting", "completed", "archived", "interrupted"]) {
    const task = await api.createTaskWithAgent(workspace.id, `Upgrade ${kind}`, profile.id, {
      ...options,
      description: "/e2e:simple-message",
      start_agent: kind !== "pending",
    });
    tasks[kind] = task;
    if (kind !== "pending") {
      if (!task.session_id) throw new Error(`baseline capability: ${kind} has no session`);
      await waitForUpgradeTurn(api, task.id, task.session_id);
    }
    await api.wsRequest("task.plan.create", {
      task_id: task.id,
      title: "Upgrade plan",
      content: `Preserved ${kind} plan`,
    });
    if (kind === "completed") {
      await api.moveTask(task.id, workflow.id, steps[1].id);
      await expect
        .poll(
          async () =>
            (await api.listTaskSessions(task.id)).sessions.find((s) => s.id === task.session_id)
              ?.state,
          { timeout: 30_000 },
        )
        .toBe("COMPLETED");
      const current = (await api.listTaskSessions(task.id)).sessions;
      const next = current.find((s) => s.id !== task.session_id);
      if (!next) throw new Error("workflow profile switch did not create a destination session");
      await waitForUpgradeTurn(api, task.id, next.id);
      await api.moveTask(task.id, workflow.id, steps.at(-1)!.id);
      await api.updateTaskState(task.id, "COMPLETED");
    }
    if (kind === "archived") await api.archiveTask(task.id);
  }
  const sibling = await api.launchSession({
    task_id: tasks.waiting.id,
    agent_profile_id: profile.id,
    executor_profile_id: executorProfile.id,
    prompt: "/e2e:simple-message",
  });
  await waitForUpgradeTurn(api, tasks.waiting.id, sibling.session_id);
  await api.setPrimarySession(tasks.waiting.session_id!);
  const seed = {
    version: 1,
    workspaceId: workspace.id,
    workflowId: workflow.id,
    steps: steps.map((s) => ({ id: s.id, name: s.name })),
    profileId: profile.id,
    reviewProfileId: reviewProfile.id,
    executorId: executor.id,
    executorProfileId: executorProfile.id,
    repositoryId: repository.id,
    repositoryPath,
    tasks,
  };
  fs.writeFileSync(path.join(backend.root, "seed.json"), JSON.stringify(seed, null, 2));
  return seed;
}
export async function interruptUpgrade(backend: UpgradeBackend, seed: UpgradeSeed) {
  const task = seed.tasks.interrupted;
  await backend.api.addUserMessage(task.id, task.session_id!, "/sleep 300");
  await expect
    .poll(
      async () =>
        (await backend.api.listTaskSessions(task.id)).sessions.find((s) => s.id === task.session_id)
          ?.state,
      { timeout: 30_000 },
    )
    .toBe("RUNNING");
  const before = await captureUpgradeState(backend, seed);
  saveUpgradeSnapshot(backend, "baseline-state.json", before);
  await backend.stop(true);
  backend.sealBaseline();
  return before;
}
