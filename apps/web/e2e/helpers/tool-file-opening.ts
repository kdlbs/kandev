import path from "node:path";
import type { Page } from "@playwright/test";
import type { BackendContext } from "../fixtures/backend";
import type { SeedData } from "../fixtures/test-base";
import type { ApiClient } from "./api-client";
import { createEmptyRemoteRepository } from "./empty-remote-repository";
import { GitHelper, createStandardProfile, publishSeedCommit } from "./git-helper";
import { waitForWorkspaceFile, waitForWorkspacePath } from "./session";
import { waitForActiveTaskSession } from "./session-store";
import { SessionPage } from "../pages/session-page";

export async function withToolFileTask(
  context: { testPage: Page; apiClient: ApiClient; seedData: SeedData; backend: BackendContext },
  kind: "read" | "search",
  run: (session: SessionPage, title: string) => Promise<void>,
) {
  const { testPage, apiClient, seedData, backend } = context;
  const fixture = createEmptyRemoteRepository(backend.tmpDir, `tool-file-${kind}`);
  const git = new GitHelper(fixture.localPath, fixture.gitEnv);
  const fileName = kind === "read" ? "Dockerfile" : "LICENSE";
  const content =
    kind === "read"
      ? Array.from({ length: 500 }, (_, index) => `read fixture line ${index + 1}`).join("\n")
      : "search fixture content";
  let repositoryId: string | undefined;
  let profileId: string | undefined;
  let taskId: string | undefined;
  try {
    git.createFile(fileName, content);
    git.stageAll();
    git.commit(`add ${fileName}`);
    publishSeedCommit(git, fixture.remoteURL);
    repositoryId = (await apiClient.createRepository(seedData.workspaceId, fixture.localPath)).id;
    profileId = (await createStandardProfile(apiClient, `tool-file-${kind}-${Date.now()}`)).id;
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      `Tool file ${kind}`,
      profileId,
      {
        description: 'e2e:message("ready")',
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [repositoryId],
      },
    );
    taskId = task.id;
    if (!task.session_id) throw new Error("Missing task session");
    await waitForWorkspaceFile(apiClient, task.session_id, fileName);
    const workspace = await waitForWorkspacePath(apiClient, task.id, task.session_id);
    const title = kind === "read" ? path.join(workspace, fileName) : fileName;
    await apiClient.seedSessionMessage(task.session_id, {
      type: kind === "read" ? "tool_read" : "tool_search",
      content: `Tool ${kind}`,
      metadata: {
        status: "complete",
        tool_call_id: `tool-file-${kind}`,
        normalized:
          kind === "read"
            ? {
                read_file: {
                  file_path: title,
                  offset: 300,
                  limit: 1,
                  output: { content: "read fixture line 300", line_count: 1 },
                },
              }
            : { code_search: { path: ".", output: { files: [title], file_count: 1 } } },
      },
    });
    await testPage.goto(`/t/${task.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await waitForActiveTaskSession(testPage, task.id, task.session_id);
    await run(session, title);
  } finally {
    if (taskId) await apiClient.deleteTask(taskId, { discardWorktreeChanges: true });
    if (repositoryId) await apiClient.deleteRepository(repositoryId);
    if (profileId) await apiClient.deleteAgentProfile(profileId, true);
    fixture.cleanup();
  }
}
