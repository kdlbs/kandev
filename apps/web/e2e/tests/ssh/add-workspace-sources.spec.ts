import { expect, test } from "../../fixtures/ssh-test-base";
import { startHTTPGitFixture } from "../../helpers/http-git-server";
import { execInContainer, readRemoteFile, remotePathExists } from "../../helpers/ssh";
import { waitForLatestSessionDone } from "../../helpers/session";
import { SessionPage } from "../../pages/session-page";
import fs from "node:fs";

async function cleanupFixture(
  fixture: { close: () => Promise<void> },
  releaseBackendEnv?: () => Promise<void>,
): Promise<void> {
  let releaseError: unknown;
  try {
    await releaseBackendEnv?.();
  } catch (error) {
    releaseError = error;
  } finally {
    try {
      await fixture.close();
    } catch (closeError) {
      if (releaseError) {
        throw new AggregateError(
          [releaseError, closeError],
          "failed to release the backend environment and close the HTTP Git fixture",
        );
      }
      throw closeError;
    }
  }
  if (releaseError) throw releaseError;
}

test.describe("SSH executor — attach workspace sources", () => {
  test("materializes a cloneable repository across backend reconnect without leaking credentials", async ({
    apiClient,
    backend,
    seedData,
    testPage,
  }) => {
    test.setTimeout(240_000);
    const fixture = await startHTTPGitFixture(backend.tmpDir, "ssh-second-source");
    let releaseBackendEnv: (() => Promise<void>) | undefined;
    let releaseRemoteGitRewrite: (() => void) | undefined;
    try {
      releaseBackendEnv = await backend.useEnv(fixture.backendEnv);
      const rewriteKey = fixture.gitConfigEnvVars.find(
        ({ key }) => key === "GIT_CONFIG_KEY_0",
      )?.value;
      const rewriteValue = fixture.gitConfigEnvVars.find(
        ({ key }) => key === "GIT_CONFIG_VALUE_0",
      )?.value;
      if (!rewriteKey || !rewriteValue) {
        throw new Error("HTTP Git fixture did not provide its URL rewrite");
      }
      execInContainer(seedData.sshTarget, ["git", "config", "--system", rewriteKey, rewriteValue]);
      releaseRemoteGitRewrite = () => {
        execInContainer(seedData.sshTarget, [
          "git",
          "config",
          "--system",
          "--unset-all",
          rewriteKey,
        ]);
      };
      const fixtureRepository = await apiClient.createRepository(
        seedData.workspaceId,
        fixture.checkoutPath,
        "main",
        {
          name: "fixture/ssh-second-source",
          provider: "gitlab",
          provider_host: "https://gitlab.com",
          provider_owner: "fixture",
          provider_name: "ssh-second-source",
        },
      );
      const task = await apiClient.createTaskWithAgent(
        seedData.workspaceId,
        "SSH remote workspace source",
        seedData.agentProfileId,
        {
          description: "/e2e:simple-message",
          workflow_id: seedData.workflowId,
          workflow_step_id: seedData.startStepId,
          repository_ids: [seedData.repositoryId],
          executor_profile_id: seedData.sshExecutorProfileId,
        },
      );
      await waitForLatestSessionDone(apiClient, task.id, 1, "Waiting for SSH task");
      await testPage.goto(`/t/${task.id}`);
      const session = new SessionPage(testPage);
      await session.waitForLoad();
      await session.clickTab("Files");
      await testPage.getByTestId("files-workspace-actions").click();
      await testPage.getByRole("menuitem", { name: "Add Repositories to workspace" }).click();
      const dialog = testPage.getByTestId("add-workspace-sources-dialog");
      await expect(dialog).toBeVisible();
      await expect(dialog.getByTestId("source-mode-local")).toHaveCount(0);
      await expect(dialog.getByRole("button", { name: "Add folder" })).toHaveCount(0);
      await dialog.getByRole("button", { name: "Add repository" }).click();
      await expect(testPage.getByRole("menuitem", { name: "Workspace repository" })).toBeVisible();
      await expect(testPage.getByRole("menuitem", { name: "Local Git repository" })).toBeVisible();
      await expect(testPage.getByRole("menuitem", { name: "Remote repository" })).toBeVisible();
      await testPage.keyboard.press("Escape");
      await dialog.getByRole("button", { name: "Cancel" }).click();
      const response = await apiClient.rawRequest(
        "POST",
        `/api/v1/tasks/${task.id}/workspace-sources`,
        {
          sources: [
            {
              kind: "repository",
              repository_id: fixtureRepository.id,
              base_branch: "main",
            },
          ],
        },
      );
      const responseText = await response.text();
      expect(response.status, responseText).toBe(200);
      expect(responseText).not.toContain(seedData.sshTarget.identityFile);
      expect(responseText).not.toContain("BEGIN OPENSSH PRIVATE KEY");

      const rows = await apiClient.listSSHSessions(seedData.sshExecutorId);
      const row = rows.find((candidate) => candidate.task_id === task.id);
      expect(row?.remote_task_dir).toBeTruthy();
      expect(row?.local_forward_port).toBeGreaterThan(0);
      const initialForwardPort = row!.local_forward_port;
      const sibling = `${row!.remote_task_dir}/fixture-ssh-second-source-main/remote-source.txt`;
      expect(remotePathExists(seedData.sshTarget, sibling)).toBe(true);
      expect(readRemoteFile(seedData.sshTarget, sibling)).toBe("ssh-second-source fixture\n");
      const agentctlLog = readRemoteFile(
        seedData.sshTarget,
        `${row!.remote_task_dir}/.kandev/sessions/${row!.session_id}/agentctl.log`,
      );
      expect(agentctlLog).not.toContain(fs.readFileSync(seedData.sshTarget.identityFile, "utf8"));

      await session.clickTab("Files");
      await expect(
        session.files
          .getByTestId("file-tree-node")
          .filter({ hasText: "fixture-ssh-second-source-main" }),
      ).toBeVisible({ timeout: 30_000 });

      await backend.restart();
      await testPage.reload();
      await session.waitForLoad();
      await session.waitForChatIdle({ timeout: 60_000 });
      await expect
        .poll(
          async () => {
            const forwardPort = (await apiClient.listSSHSessions(seedData.sshExecutorId)).find(
              (item) => item.task_id === task.id,
            )?.local_forward_port;
            return Boolean(forwardPort && forwardPort !== initialForwardPort);
          },
          {
            timeout: 60_000,
            message: "Waiting for SSH backend reconnect",
          },
        )
        .toBe(true);
      expect(remotePathExists(seedData.sshTarget, sibling)).toBe(true);
      expect(readRemoteFile(seedData.sshTarget, sibling)).toBe("ssh-second-source fixture\n");
      await session.clickTab("Files");
      await expect(
        session.files
          .getByTestId("file-tree-node")
          .filter({ hasText: "fixture-ssh-second-source-main" }),
      ).toBeVisible({ timeout: 30_000 });
    } finally {
      try {
        releaseRemoteGitRewrite?.();
      } finally {
        await cleanupFixture(fixture, releaseBackendEnv);
      }
    }
  });
});
