import fs from "node:fs";
import path from "node:path";
import { execSync } from "node:child_process";
import { test, expect } from "../../fixtures/test-base";
import { makeGitEnv } from "../../helpers/git-helper";
import { waitForLatestSessionDone } from "../../helpers/session";
import { SessionPage } from "../../pages/session-page";

const GLOBAL_VALUE = "e2e-global-secret-value";
const WORKSPACE_VALUE = "e2e-workspace-secret-value";

async function createSecretFromSettings(
  page: import("@playwright/test").Page,
  route: string,
  name: string,
  value: string,
) {
  await page.goto(route);
  await expect(page.getByRole("button", { name: "Add secret", exact: true })).toBeVisible();
  await page.getByRole("button", { name: "Add secret", exact: true }).click();
  await page.getByPlaceholder("Name (e.g. OpenAI Production Key)").fill(name);
  await page.getByPlaceholder("Secret value").fill(value);

  const save = page
    .getByTestId("settings-floating-save")
    .getByRole("button", { name: "Save changes" });
  await expect(save).toBeEnabled();
  await save.click();
  await expect(page.getByText(name, { exact: true })).toBeVisible();
  await expect(page.locator("body")).not.toContainText(value);
}

async function createLocalRepository(
  apiClient: import("../../helpers/api-client").ApiClient,
  backendTmpDir: string,
  workspaceId: string,
  name: string,
) {
  const repoDir = path.join(backendTmpDir, "repos", name.toLowerCase().replaceAll(" ", "-"));
  fs.mkdirSync(repoDir, { recursive: true });
  const env = makeGitEnv(backendTmpDir);
  execSync("git init -b main", { cwd: repoDir, env });
  execSync('git commit --allow-empty -m "init"', { cwd: repoDir, env });
  return apiClient.createRepository(workspaceId, repoDir, "main", { name });
}

async function launchError(
  apiClient: import("../../helpers/api-client").ApiClient,
  taskId: string,
  agentProfileId: string,
) {
  try {
    await apiClient.launchSession({
      task_id: taskId,
      agent_profile_id: agentProfileId,
      prompt: "/e2e:simple-message",
    });
  } catch (error) {
    return error instanceof Error ? error.message : String(error);
  }
  return "launch unexpectedly succeeded";
}

test.describe("Repository secrets", () => {
  test("creates scoped secrets, filters them, and persists repository bindings", async ({
    testPage,
    apiClient,
    seedData,
    prCapture,
  }) => {
    const globalName = "E2E Global Repository Secret";
    const workspaceName = "E2E Workspace Repository Secret";
    await createSecretFromSettings(testPage, "/settings/general/secrets", globalName, GLOBAL_VALUE);
    const global = (await apiClient.listSecrets()).find((secret) => secret.name === globalName);
    expect(global?.id).toBeTruthy();

    await createSecretFromSettings(
      testPage,
      `/settings/workspace/${seedData.workspaceId}/secrets`,
      workspaceName,
      WORKSPACE_VALUE,
    );
    const workspace = (
      await apiClient.listSecrets({
        scope: "workspace",
        workspaceId: seedData.workspaceId,
      })
    ).find((secret) => secret.name === workspaceName);
    expect(workspace?.id).toBeTruthy();
    if (!global || !workspace) throw new Error("E2E secret creation did not return metadata");

    try {
      await testPage.goto("/settings/general/secrets");
      await expect(testPage.getByText(globalName, { exact: true })).toBeVisible();
      await expect(testPage.getByText(workspaceName, { exact: true })).toHaveCount(0);

      await testPage.goto(`/settings/workspace/${seedData.workspaceId}/secrets`);
      await expect(testPage.getByText(workspaceName, { exact: true })).toBeVisible();
      await expect(testPage.getByText(globalName, { exact: true })).toHaveCount(0);
      await expect(testPage.locator("body")).not.toContainText(GLOBAL_VALUE);
      await expect(testPage.locator("body")).not.toContainText(WORKSPACE_VALUE);

      await apiClient.updateRepository(seedData.repositoryId, {
        secret_bindings: [
          { key: "E2E_GLOBAL_TOKEN", secret_id: global.id },
          { key: "E2E_WORKSPACE_TOKEN", secret_id: workspace.id },
        ],
      });

      await testPage.goto(`/settings/workspace/${seedData.workspaceId}/repositories`);
      await testPage.getByRole("heading", { name: "E2E Repo", exact: true }).click();
      const editor = testPage.getByTestId("repository-secret-bindings");
      await expect(editor.getByTestId("repository-secret-key-0")).toHaveValue("E2E_GLOBAL_TOKEN");
      await expect(editor.getByTestId("repository-secret-key-1")).toHaveValue(
        "E2E_WORKSPACE_TOKEN",
      );
      await expect(editor.getByTestId("repository-secret-select-0")).toContainText(globalName);
      await expect(editor.getByTestId("repository-secret-select-1")).toContainText(workspaceName);
      await expect(testPage.locator("body")).not.toContainText(GLOBAL_VALUE);
      await expect(testPage.locator("body")).not.toContainText(WORKSPACE_VALUE);
      await testPage.getByText("Environment secrets", { exact: true }).scrollIntoViewIfNeeded();
      await prCapture.screenshot("desktop-repository-secret-bindings", {
        caption: "Desktop repository settings with scoped secret bindings",
      });

      await testPage.reload();
      await testPage.getByRole("heading", { name: "E2E Repo", exact: true }).click();
      const reloadedEditor = testPage.getByTestId("repository-secret-bindings");
      await expect(reloadedEditor.getByTestId("repository-secret-key-0")).toHaveValue(
        "E2E_GLOBAL_TOKEN",
      );
      await expect(reloadedEditor.getByTestId("repository-secret-key-1")).toHaveValue(
        "E2E_WORKSPACE_TOKEN",
      );
    } finally {
      await apiClient.updateRepository(seedData.repositoryId, { secret_bindings: [] });
      await apiClient.deleteSecret(workspace.id, seedData.workspaceId);
      await apiClient.deleteSecret(global.id);
    }
  });

  test("passes the resolved binding to setup and a new local terminal", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const secret = await apiClient.createSecret("E2E Local Runtime Secret", GLOBAL_VALUE);
    const marker = ".kandev-e2e-secret-setup-check";
    await apiClient.updateRepository(seedData.repositoryId, {
      secret_bindings: [{ key: "E2E_REPOSITORY_SECRET", secret_id: secret.id }],
      setup_script: `if [ -n "$E2E_REPOSITORY_SECRET" ]; then printf setup-binding-present > ${marker}; fi`,
    });

    try {
      const task = await apiClient.createTaskWithAgent(
        seedData.workspaceId,
        "Repository secret local runtime",
        seedData.agentProfileId,
        {
          description: "/e2e:simple-message",
          workflow_id: seedData.workflowId,
          workflow_step_id: seedData.startStepId,
          repository_ids: [seedData.repositoryId],
        },
      );
      await waitForLatestSessionDone(apiClient, task.id, 1, "Waiting for local secret session");

      await testPage.goto(`/t/${task.id}`);
      const session = new SessionPage(testPage);
      await session.waitForLoad();
      await session.clickTab("Terminal");
      await session.expectTerminalConnected(30_000);
      await session.typeInTerminal(`cat ${marker}`);
      await session.expectTerminalHasText("setup-binding-present");
      await session.typeInTerminal(
        'if [ -n "$E2E_REPOSITORY_SECRET" ]; then printf terminal-binding-present; fi',
      );
      await session.expectTerminalHasText("terminal-binding-present");
    } finally {
      await apiClient.updateRepository(seedData.repositoryId, {
        secret_bindings: [],
        setup_script: "",
      });
      await apiClient.deleteSecret(secret.id);
    }
  });

  test("fails before launch for conflicting and deleted repository references", async ({
    apiClient,
    backend,
    seedData,
  }) => {
    const first = await apiClient.createSecret("E2E Conflict Secret A", GLOBAL_VALUE);
    const second = await apiClient.createSecret("E2E Conflict Secret B", WORKSPACE_VALUE);
    const extraRepo = await createLocalRepository(
      apiClient,
      backend.tmpDir,
      seedData.workspaceId,
      "E2E Conflict Repository",
    );

    try {
      await apiClient.updateRepository(seedData.repositoryId, {
        secret_bindings: [{ key: "E2E_CONFLICT_TOKEN", secret_id: first.id }],
      });
      await apiClient.updateRepository(extraRepo.id, {
        secret_bindings: [{ key: "E2E_CONFLICT_TOKEN", secret_id: second.id }],
      });
      const conflictTask = await apiClient.createTask(
        seedData.workspaceId,
        "Repository secret conflict",
        {
          workflow_id: seedData.workflowId,
          workflow_step_id: seedData.startStepId,
          repository_ids: [seedData.repositoryId, extraRepo.id],
        },
      );
      const conflictMessage = await launchError(
        apiClient,
        conflictTask.id,
        seedData.agentProfileId,
      );
      expect(conflictMessage).toContain("E2E_CONFLICT_TOKEN");
      expect(conflictMessage).not.toContain(first.id);
      expect(conflictMessage).not.toContain(second.id);

      await apiClient.updateRepository(seedData.repositoryId, {
        secret_bindings: [{ key: "E2E_DELETED_TOKEN", secret_id: first.id }],
      });
      await apiClient.deleteSecret(first.id);
      const deletedTask = await apiClient.createTask(
        seedData.workspaceId,
        "Deleted repository secret",
        {
          workflow_id: seedData.workflowId,
          workflow_step_id: seedData.startStepId,
          repository_ids: [seedData.repositoryId],
        },
      );
      const deletedMessage = await launchError(apiClient, deletedTask.id, seedData.agentProfileId);
      expect(deletedMessage).toContain("E2E_DELETED_TOKEN");
      expect(deletedMessage).not.toContain(first.id);

      const repositoriesResponse = await apiClient.rawRequest(
        "GET",
        `/api/v1/workspaces/${seedData.workspaceId}/repositories`,
      );
      expect(repositoriesResponse.ok).toBe(true);
      const repositories = (await repositoriesResponse.json()) as {
        repositories: Array<{
          id: string;
          secret_bindings?: Array<{ key: string; secret_id: string }>;
        }>;
      };
      expect(
        repositories.repositories.find((repo) => repo.id === seedData.repositoryId)
          ?.secret_bindings,
      ).toEqual([{ key: "E2E_DELETED_TOKEN", secret_id: first.id }]);
    } finally {
      await apiClient.updateRepository(seedData.repositoryId, { secret_bindings: [] });
      await apiClient.updateRepository(extraRepo.id, { secret_bindings: [] });
      await apiClient.deleteSecret(second.id).catch(() => undefined);
      await apiClient.deleteSecret(first.id).catch(() => undefined);
    }
  });
});
