import { test, expect } from "../../fixtures/test-base";
import { openChangesTab } from "../git/diff-update-helpers";
import { GITLAB_PROJECT } from "../../helpers/gitlab";
import {
  assertLocatorWithinViewportX,
  assertNoDocumentHorizontalOverflow,
} from "../../helpers/layout-assertions";
import { GitLabPage } from "../../pages/gitlab-page";
import { SessionPage } from "../../pages/session-page";

test.describe("GitLab merge request creation", () => {
  test("creates an MR through the runtime and automatically persists the task link", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    test.setTimeout(180_000);
    const remoteURL = `${backend.baseUrl}/${GITLAB_PROJECT}.git`;
    await apiClient.configureGitLab(seedData.workspaceId, backend.baseUrl);
    await apiClient.configureGitLabRepositoryRemote(seedData.repositoryId, remoteURL);
    await apiClient.updateRepository(seedData.repositoryId, {
      provider: "gitlab",
      provider_host: backend.baseUrl,
      provider_owner: "platform",
      provider_name: "kandev",
    });

    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Create GitLab merge request",
      seedData.agentProfileId,
      {
        description: "/e2e:diff-update-setup",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
        executor_profile_id: seedData.worktreeExecutorProfileId,
      },
    );
    if (!task.session_id) throw new Error("GitLab creation task did not return a session");

    await testPage.goto(`/t/${task.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await expect(
      session.chat.getByText("diff-update-setup complete", { exact: false }),
    ).toBeVisible({
      timeout: 45_000,
    });

    await openChangesTab(testPage);
    const create = testPage.getByTestId("commits-repo-create-pr").first();
    await expect(create).toBeVisible();
    await create.click();
    const dialog = testPage.getByRole("dialog", { name: "Create merge request" });
    await expect(dialog).toBeVisible();
    await assertLocatorWithinViewportX(dialog, "desktop create MR dialog");
    await dialog
      .getByRole("textbox", { name: "Merge Request title", exact: true })
      .fill("Runtime-created GitLab MR");
    await dialog
      .getByRole("textbox", { name: "Description", exact: true })
      .fill("Created through worktree.create_pr.");
    const draft = dialog.getByLabel("Create as draft");
    await expect(draft).toBeChecked();
    await dialog.getByRole("button", { name: "Create MR", exact: true }).click();

    const gitlab = new GitLabPage(testPage);
    await expect
      .poll(async () => {
        try {
          return (await apiClient.getGitLabPushRecord(seedData.repositoryId)).args;
        } catch {
          return "";
        }
      })
      .toBe("push --set-upstream origin HEAD");
    await expect(testPage.getByTestId("mr-topbar-button")).toHaveAttribute("data-mr-iid", "100", {
      timeout: 120_000,
    });
    await gitlab.openLinkedMR(100);
    await expect(
      testPage.getByTestId("mr-detail-panel").last().getByText("Runtime-created GitLab MR"),
    ).toBeVisible();

    await testPage.reload();
    await expect(testPage.getByTestId("mr-topbar-button")).toHaveAttribute("data-mr-iid", "100", {
      timeout: 30_000,
    });
    await assertNoDocumentHorizontalOverflow(testPage, "desktop created MR task");
  });
});
