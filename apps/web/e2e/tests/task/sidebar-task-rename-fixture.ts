import { execFileSync } from "node:child_process";
import fs from "node:fs";
import path from "node:path";
import type { Page } from "@playwright/test";
import { expect, type SeedData } from "../../fixtures/test-base";
import type { BackendContext } from "../../fixtures/backend";
import type { ApiClient } from "../../helpers/api-client";
import { makeGitEnv } from "../../helpers/git-helper";

type Context = {
  testPage: Page;
  apiClient: ApiClient;
  backend: BackendContext;
  seedData: SeedData;
};

function repositorySnapshot(repositoryPath: string) {
  return ["rev-parse HEAD", "status --porcelain"].map((command) =>
    execFileSync("git", ["-C", repositoryPath, ...command.split(" ")], { encoding: "utf8" }),
  );
}

function initializeRepository(root: string, homeDirectory: string) {
  const repositoryPath = path.join(root, "source");
  const remotePath = path.join(root, "origin.git");
  const env = makeGitEnv(homeDirectory);
  const git = (args: string[]) => execFileSync("git", args, { env, stdio: "pipe" });
  git(["init", "--bare", "-b", "main", remotePath]);
  git(["init", "-b", "main", repositoryPath]);
  fs.writeFileSync(path.join(repositoryPath, "rename-base.txt"), "Owned Rename fixture\n");
  git(["-C", repositoryPath, "add", "rename-base.txt"]);
  git(["-C", repositoryPath, "commit", "-m", "initialize rename fixture"]);
  const remoteURL = `file://${remotePath}`;
  git(["-C", repositoryPath, "remote", "add", "origin", remoteURL]);
  git(["-C", repositoryPath, "push", "origin", "main"]);
  return { repositoryPath, remoteURL };
}

// Every runtime producer in this flow receives an owned workspace and repository.
export async function withIsolatedRenameFixture(
  context: Context,
  run: (owned: SeedData) => Promise<void>,
) {
  const { apiClient, backend, seedData, testPage } = context;
  const workerBefore = repositorySnapshot(seedData.repositoryPath);
  const root = fs.mkdtempSync(path.join(backend.tmpDir, "row-rename-"));
  let workspace: { id: string; name: string } | undefined;
  let source: ReturnType<typeof initializeRepository> | undefined;
  let sourceBefore: string[] | undefined;
  const errors: unknown[] = [];
  try {
    source = initializeRepository(root, backend.tmpDir);
    sourceBefore = repositorySnapshot(source.repositoryPath);
    workspace = await apiClient.createWorkspace(`Rename ${path.basename(root)}`);
    const workflow = await apiClient.createWorkflow(workspace.id, "Rename workflow", "simple");
    const { steps } = await apiClient.listWorkflowSteps(workflow.id);
    const start = steps.find((step) => step.is_start_step);
    expect(start).toBeDefined();
    const repository = await apiClient.createRepository(workspace.id, source.repositoryPath);
    await run({
      ...seedData,
      workspaceId: workspace.id,
      workflowId: workflow.id,
      startStepId: start!.id,
      steps,
      repositoryId: repository.id,
      repositoryPath: source.repositoryPath,
      repositoryRemoteURL: source.remoteURL,
      repositoryBaselineOID: sourceBefore[0].trim(),
    });
  } catch (error) {
    errors.push(error);
  } finally {
    try {
      await testPage.goto("about:blank");
    } catch (error) {
      errors.push(error);
    }
    if (source && sourceBefore) {
      try {
        expect(repositorySnapshot(source.repositoryPath)).toEqual(sourceBefore);
      } catch (error) {
        errors.push(error);
      }
    }
    if (workspace) {
      try {
        await apiClient.deleteWorkspace(workspace.id, workspace.name);
      } catch (error) {
        errors.push(error);
      }
    }
    try {
      fs.rmSync(root, { recursive: true, force: true });
    } catch (error) {
      errors.push(error);
    }
    try {
      expect(repositorySnapshot(seedData.repositoryPath)).toEqual(workerBefore);
    } catch (error) {
      errors.push(error);
    }
  }
  if (errors.length === 1) throw errors[0];
  if (errors.length > 1) throw new AggregateError(errors, "Rename flow and cleanup failures");
}
