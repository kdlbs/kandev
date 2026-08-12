import { test, expect } from "../../fixtures/test-base";
import { SessionPage } from "../../pages/session-page";

test.describe("Mobile archive task redirect", () => {
  test("keeps navigation responsive after skipping cascaded descendants", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);

    const parentTitle = "Mobile cascade archive parent";
    const childTitle = "Mobile cascade archive recent child";
    const secondChildTitle = "Mobile cascade archive second child";
    const unrelatedTitle = "Mobile cascade archive unrelated task";
    const laterTitle = "Mobile cascade archive later task";
    const createOptions = {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    };
    const parent = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      parentTitle,
      seedData.agentProfileId,
      { ...createOptions, description: 'e2e:message("mobile cascade parent response")' },
    );
    const child = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      childTitle,
      seedData.agentProfileId,
      {
        ...createOptions,
        parent_id: parent.id,
        description: 'e2e:message("mobile cascade child response")',
      },
    );
    const secondChild = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      secondChildTitle,
      seedData.agentProfileId,
      {
        ...createOptions,
        parent_id: parent.id,
        description: 'e2e:message("mobile cascade second child response")',
      },
    );
    const unrelated = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      unrelatedTitle,
      seedData.agentProfileId,
      { ...createOptions, description: 'e2e:message("mobile cascade unrelated response")' },
    );
    const later = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      laterTitle,
      seedData.agentProfileId,
      { ...createOptions, description: 'e2e:message("mobile cascade later response")' },
    );
    for (const task of [parent, child, secondChild, unrelated, later]) {
      if (!task.session_id) throw new Error(`missing session for ${task.title}`);
    }

    // Establish the same recent-child ordering as desktop, then open the
    // parent through a normal SPA document navigation before measuring reloads.
    await testPage.goto(`/t/${child.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await expect(testPage.getByTestId("mobile-task-layout")).toBeVisible();
    await expect(session.chat.getByText("mobile cascade child response").last()).toBeVisible({
      timeout: 30_000,
    });
    await testPage.goto(`/t/${parent.id}`);
    await session.waitForLoad();
    await expect(testPage.getByTestId("mobile-task-layout")).toBeVisible();
    await expect(session.chat.getByText("mobile cascade parent response").last()).toBeVisible({
      timeout: 30_000,
    });

    const documentRequests: string[] = [];
    testPage.on("request", (request) => {
      if (
        request.isNavigationRequest() &&
        request.frame() === testPage.mainFrame() &&
        request.resourceType() === "document"
      ) {
        documentRequests.push(request.url());
      }
    });
    const documentRequestsBeforeArchive = documentRequests.length;

    await testPage.getByTestId("mobile-session-menu").tap();
    const drawer = testPage.getByRole("dialog", { name: "Tasks" });
    const parentRow = drawer.getByTestId("sidebar-task-item").filter({ hasText: parentTitle });
    await expect(parentRow).toBeVisible({ timeout: 15_000 });
    await parentRow.getByRole("button", { name: "Task actions" }).tap();
    await testPage.getByRole("menuitem", { name: "Archive", exact: true }).tap();

    const archiveDialog = testPage.getByRole("alertdialog");
    await expect(archiveDialog).toBeVisible();
    await archiveDialog.getByTestId("archive-cascade-checkbox").tap();
    await archiveDialog.getByRole("button", { name: "Archive", exact: true }).tap();

    await expect(testPage).toHaveURL(new RegExp(`/t/${unrelated.id}$`), { timeout: 20_000 });
    await expect(testPage.getByTestId("mobile-task-layout")).toBeVisible();
    await expect(session.chat.getByText("mobile cascade unrelated response").last()).toBeVisible({
      timeout: 30_000,
    });
    expect(documentRequests).toHaveLength(documentRequestsBeforeArchive);

    await testPage.getByTestId("mobile-session-menu").tap();
    const postArchiveDrawer = testPage.getByRole("dialog", { name: "Tasks" });
    await expect(
      postArchiveDrawer.getByTestId("sidebar-task-item").filter({ hasText: parentTitle }),
    ).toHaveCount(0, { timeout: 15_000 });
    await expect(
      postArchiveDrawer.getByTestId("sidebar-task-item").filter({ hasText: childTitle }),
    ).toHaveCount(0, { timeout: 15_000 });
    await expect(
      postArchiveDrawer.getByTestId("sidebar-task-item").filter({ hasText: secondChildTitle }),
    ).toHaveCount(0, { timeout: 15_000 });

    await postArchiveDrawer.getByTestId("sidebar-task-item").filter({ hasText: laterTitle }).tap();
    await expect(postArchiveDrawer).toBeHidden();
    await expect(testPage).toHaveURL(new RegExp(`/t/${later.id}$`), { timeout: 20_000 });
    await expect(testPage.getByTestId("mobile-task-layout")).toBeVisible();
    await expect(session.chat.getByText("mobile cascade later response").last()).toBeVisible({
      timeout: 30_000,
    });
    expect(documentRequests).toHaveLength(documentRequestsBeforeArchive);
  });
});
