import { expect, test } from "../../fixtures/ssh-test-base";
import { startHTTPGitFixture } from "../../helpers/http-git-server";
import { readRemoteFile, remotePathExists } from "../../helpers/ssh";
import { waitForLatestSessionDone } from "../../helpers/session";
import { SessionPage } from "../../pages/session-page";
import fs from "node:fs";

function fixtureBackendEnv(fixture: { gitConfigEnvVars: Array<{ key: string; value: string }> }) {
  return Object.fromEntries(fixture.gitConfigEnvVars.map(({ key, value }) => [key, value]));
}

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
    try {
      releaseBackendEnv = await backend.useEnv(fixtureBackendEnv(fixture));
      const fixtureProfile = await apiClient.createExecutorProfile(seedData.sshExecutorId, {
        name: "E2E SSH HTTP Git fixture",
        config: {},
        prepare_script: "",
        cleanup_script: "",
        env_vars: fixture.gitConfigEnvVars,
      });
      const task = await apiClient.createTaskWithAgent(
        seedData.workspaceId,
        "SSH remote workspace source",
        seedData.agentProfileId,
        {
          description: "/e2e:simple-message",
          workflow_id: seedData.workflowId,
          workflow_step_id: seedData.startStepId,
          repository_ids: [seedData.repositoryId],
          executor_profile_id: fixtureProfile.id,
        },
      );
      await waitForLatestSessionDone(apiClient, task.id, 1, "Waiting for SSH task");
      await testPage.goto(`/t/${task.id}`);
      const session = new SessionPage(testPage);
      await session.waitForLoad();
      await session.clickTab("Files");
      await testPage.getByTestId("files-workspace-actions").click();
      await testPage.getByRole("menuitem", { name: "Add sources" }).click();
      const dialog = testPage.getByTestId("add-workspace-sources-dialog");
      await expect(dialog).toBeVisible();
      await expect(dialog.getByTestId("source-mode-local")).toBeVisible();
      await expect(dialog.getByTestId("source-mode-remote")).toBeVisible();
      await expect(dialog.getByRole("button", { name: "Local folder" })).toHaveCount(0);
      await dialog.getByRole("button", { name: "Cancel" }).click();
      const response = await apiClient.rawRequest(
        "POST",
        `/api/v1/tasks/${task.id}/workspace-sources`,
        {
          sources: [
            {
              kind: "repository",
              remote_url: fixture.remoteURL,
              provider: "gitlab",
              base_branch: "main",
            },
          ],
        },
      );
      expect(response.status).toBe(200);
      const responseText = await response.text();
      expect(responseText).not.toContain(seedData.sshTarget.identityFile);
      expect(responseText).not.toContain("BEGIN OPENSSH PRIVATE KEY");

      const rows = await apiClient.listSSHSessions(seedData.sshExecutorId);
      const row = rows.find((candidate) => candidate.task_id === task.id);
      expect(row?.remote_task_dir).toBeTruthy();
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
      await expect
        .poll(
          async () =>
            (await apiClient.listSSHSessions(seedData.sshExecutorId)).find(
              (item) => item.task_id === task.id,
            )?.local_forward_port ?? 0,
          {
            timeout: 60_000,
            message: "Waiting for SSH backend reconnect",
          },
        )
        .toBeGreaterThan(0);
      expect(remotePathExists(seedData.sshTarget, sibling)).toBe(true);
      expect(readRemoteFile(seedData.sshTarget, sibling)).toBe("ssh-second-source fixture\n");
      await testPage.reload();
      await session.waitForLoad();
      await session.clickTab("Files");
      await expect(
        session.files
          .getByTestId("file-tree-node")
          .filter({ hasText: "fixture-ssh-second-source-main" }),
      ).toBeVisible({ timeout: 30_000 });
    } finally {
      await cleanupFixture(fixture, releaseBackendEnv);
    }
  });
});
